package logic

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"time"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/snowflake"
	"go.uber.org/zap"
)

// 推荐算法权重（可解释规则，见开发文档 8.3）
const (
	interactionVoteWeight    = 2.0
	interactionCommentWeight = 3.0
	interactionCollectWeight = 4.0
	freshnessScale           = 100.0
	followBonus              = 20.0
	feedCandidateMax         = 100 // 热榜缓存的最大条目数
	feedCacheMaxAge          = 10 * time.Minute
)

// cachedFeedEntry 热榜缓存条目：装配好的 Feed 条目 + 基础分
type cachedFeedEntry struct {
	Item      *models.FeedItem `json:"item"`
	BaseScore float64          `json:"base_score"`
}

// cachedFeedSnapshot 缓存整体结构：快照版本绑定游标，版本变更后旧游标失效
type cachedFeedSnapshot struct {
	Version int64              `json:"version"` // 缓存写入时间（unix nano）
	Entries []cachedFeedEntry  `json:"entries"`
}

// buildFeedItems 把 Feed 查询行装配成 FeedItem（批量查作者与话题，避免 N+1）
func buildFeedItems(rows []*mysql.FeedRow) ([]*models.FeedItem, error) {
	if len(rows) == 0 {
		return make([]*models.FeedItem, 0), nil
	}
	// 批量查作者
	authorIDs := make([]int64, 0, len(rows))
	seen := make(map[int64]struct{})
	for _, r := range rows {
		if _, ok := seen[r.AuthorID]; !ok {
			seen[r.AuthorID] = struct{}{}
			authorIDs = append(authorIDs, r.AuthorID)
		}
	}
	users, err := mysql.GetUsersByIDs(authorIDs)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*models.User, len(users))
	for _, u := range users {
		userMap[u.UserID] = u
	}
	// 批量查问题的话题
	questionIDs := make([]int64, 0)
	for _, r := range rows {
		if r.ItemType == "question" {
			questionIDs = append(questionIDs, r.ItemID)
		}
	}
	topicMap, err := mysql.GetTopicsByQuestionIDs(questionIDs)
	if err != nil {
		return nil, err
	}

	items := make([]*models.FeedItem, 0, len(rows))
	for _, r := range rows {
		author := userMap[r.AuthorID]
		fa := models.FeedAuthor{}
		if author != nil {
			fa = models.FeedAuthor{
				UserID:    strconv.FormatInt(author.UserID, 10),
				Username:  author.Username,
				AvatarURL: author.AvatarURL,
			}
		}
		item := &models.FeedItem{
			ItemType: r.ItemType,
			ItemID:   strconv.FormatInt(r.ItemID, 10),
			Author:   fa,
			Excerpt:  truncateExcerpt(r.ExcerptSrc, 120),
			CreateTime: r.CreateTime,
			Stats: models.FeedStats{
				VoteCount:    r.VoteCount,
				CommentCount: r.CommentCount,
				CollectCount: r.CollectCount,
				AnswerCount:  r.AnswerCount,
			},
		}
		if r.ItemType == "answer" {
			item.QuestionID = strconv.FormatInt(r.QuestionID, 10)
		}
		if r.ItemType == "article" && r.ColumnID > 0 {
			if c, err := mysql.GetColumnByID(r.ColumnID); err == nil {
				item.Column = &models.ColumnBrief{ColumnID: strconv.FormatInt(c.ColumnID, 10), Name: c.Name}
			}
		}
		item.Title = r.Title
		if r.ItemType == "question" {
			if t, ok := topicMap[r.QuestionID]; ok && len(t) > 0 {
				item.Topic = t[0]
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// truncateExcerpt 摘要截断（按 rune 截，避免截断中文乱码）
func truncateExcerpt(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// feedBaseScore 可解释的基础分：interaction + freshness * 100
func feedBaseScore(item *models.FeedItem) float64 {
	interaction := float64(item.Stats.VoteCount)*interactionVoteWeight +
		float64(item.Stats.CommentCount)*interactionCommentWeight +
		float64(item.Stats.CollectCount)*interactionCollectWeight
	ageHours := time.Since(item.CreateTime).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	freshness := 1 / math.Pow(ageHours+2, 1.2)
	return interaction + freshness*freshnessScale
}

// feed refreshing 相关错误：调用方（controller）映射为可重试的业务码
var (
	ErrFeedRefreshing      = errors.New("热榜更新中，请稍后重试")
	ErrFeedSnapshotExpired = errors.New("列表已更新，请刷新重试")
)

// getHotFeedCache 读取热榜缓存（返回快照与版本号）
// 缓存不存在时：抢锁重建；抢不到锁的请求短暂等待缓存就绪，
// 超时仍无缓存则返回可重试错误——绝不把"正在刷新"伪装成空 Feed
func getHotFeedCache() (*cachedFeedSnapshot, error) {
	// 快路径：命中缓存
	if snap := readHotFeedCache(); snap != nil {
		return snap, nil
	}
	// 尝试抢锁重建（随机持有者标识）
	holder := strconv.FormatInt(snowflake.GenerateID(), 10)
	ok, err := redis.SetFeedRefreshLock(holder)
	if err != nil {
		return nil, err
	}
	if ok {
		defer redis.ReleaseFeedRefreshLock(holder)
		return rebuildHotFeedCache()
	}
	// 别的请求正在刷新：最多等 2 秒（200ms × 10 轮）轮询缓存
	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		if snap := readHotFeedCache(); snap != nil {
			return snap, nil
		}
	}
	return nil, ErrFeedRefreshing
}

// readHotFeedCache 读缓存并反序列化；损坏/缺失返回 nil
func readHotFeedCache() *cachedFeedSnapshot {
	data, _ := redis.GetFeedHotCache()
	if data == nil {
		return nil
	}
	var snap cachedFeedSnapshot
	if err := json.Unmarshal(data, &snap); err != nil || len(snap.Entries) == 0 {
		return nil
	}
	return &snap
}

// rebuildHotFeedCache 重建热榜缓存并返回新快照
func rebuildHotFeedCache() (*cachedFeedSnapshot, error) {
	// 重建：取近 30 天热门候选
	rows, err := mysql.HotFeedCandidates(feedCandidateMax * 3)
	if err != nil {
		return nil, err
	}
	items, err := buildFeedItems(rows)
	if err != nil {
		return nil, err
	}
	entries := make([]cachedFeedEntry, 0, len(items))
	for _, item := range items {
		entries = append(entries, cachedFeedEntry{
			Item:      item,
			BaseScore: feedBaseScore(item),
		})
	}
	// 按基础分排序取前 feedCandidateMax 条
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].BaseScore > entries[j].BaseScore
	})
	if len(entries) > feedCandidateMax {
		entries = entries[:feedCandidateMax]
	}
	snap := &cachedFeedSnapshot{
		Version: time.Now().UnixNano(),
		Entries: entries,
	}
	if data, err := json.Marshal(snap); err == nil {
		if err := redis.SetFeedHotCache(data); err != nil {
			zap.L().Warn("cache hot feed failed", zap.Error(err))
		}
	}
	return snap, nil
}

// recommendPage 应用关注加分后排序取页
func recommendPage(entries []cachedFeedEntry, followedIDs map[int64]struct{}, offset, limit int) (items []*models.FeedItem, hasMore bool) {
	adjusted := make([]cachedFeedEntry, len(entries))
	copy(adjusted, entries)
	for i := range adjusted {
		var authorID int64
		authorID, _ = strconv.ParseInt(adjusted[i].Item.Author.UserID, 10, 64)
		if _, ok := followedIDs[authorID]; ok {
			adjusted[i].BaseScore += followBonus
		}
	}
	sort.Slice(adjusted, func(i, j int) bool {
		return adjusted[i].BaseScore > adjusted[j].BaseScore
	})
	return sliceFeedPage(adjusted, offset, limit)
}

func sliceFeedPage(entries []cachedFeedEntry, offset, limit int) (items []*models.FeedItem, hasMore bool) {
	items = make([]*models.FeedItem, 0)
	if offset >= len(entries) {
		return items, false
	}
	end := offset + limit
	if end > len(entries) {
		end = len(entries)
	}
	for _, e := range entries[offset:end] {
		items = append(items, e.Item)
	}
	return items, end < len(entries)
}

// GetHotFeed 热榜（游标绑定缓存快照版本，快照刷新后旧游标返回可重试错误）
func GetHotFeed(viewerID int64, cursor string, limit int) (*models.PageResult[*models.FeedItem], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	offset, snap, err := parseOffsetAndSnapshot(cursor, 0)
	if err != nil {
		return nil, err
	}
	items, hasMore := sliceFeedPage(filterEntriesByBlocks(snap.Entries, viewerID), offset, limit)
	next := ""
	if hasMore {
		next = encodeOffsetCursor(offset+limit, snap.Version, 0)
	}
	return &models.PageResult[*models.FeedItem]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// GetRecommendFeed 推荐流：热榜基础分 + 关注加分（未登录或关闭个性化时与热榜一致）。
// 过滤优先于排序：屏蔽作者、负反馈（不感兴趣内容/减少话题）、治理状态在返回前过滤；
// 推荐理由只从服务端已知信号生成；每次响应条目签发 exposure_id 供点击关联。
func GetRecommendFeed(userID int64, cursor string, limit int) (*models.PageResult[*models.FeedItem], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	offset, snap, err := parseOffsetAndSnapshot(cursor, userID)
	if err != nil {
		return nil, err
	}

	// 个性化开关（关闭时：不加关注加分，不用任何个人信号，与热榜同序）
	personalized := userID > 0
	if personalized {
		enabled, _, err := mysql.GetRecommendationPreference(userID)
		if err != nil {
			return nil, err
		}
		personalized = enabled
	}

	// 过滤集合：屏蔽作者、不感兴趣内容、减少话题
	blockedSet := map[int64]struct{}{}
	hiddenContent := map[string]struct{}{}
	reducedTopics := map[int64]struct{}{}
	if userID > 0 {
		blocked, _ := mysql.BlockedAuthorIDs(userID)
		for _, b := range blocked {
			blockedSet[b] = struct{}{}
		}
		fb, _ := mysql.ListAllRecommendationFeedback(userID)
		for _, f := range fb {
			switch f.Kind {
			case "not_interested":
				hiddenContent[f.TargetType+":"+strconv.FormatInt(f.TargetID, 10)] = struct{}{}
			case "reduce_topic":
				reducedTopics[f.TargetID] = struct{}{}
			}
		}
	}

	// 过滤候选（返回前过滤；快照旧条目也不泄露）
	entries := make([]cachedFeedEntry, 0, len(snap.Entries))
	for _, e := range snap.Entries {
		var aid int64
		aid, _ = strconv.ParseInt(e.Item.Author.UserID, 10, 64)
		if _, bad := blockedSet[aid]; bad {
			continue
		}
		if _, bad := hiddenContent[e.Item.ItemType+":"+e.Item.ItemID]; bad {
			continue
		}
		if e.Item.Topic != nil {
			var tid int64
			tid, _ = strconv.ParseInt(e.Item.Topic.TopicID, 10, 64)
			if _, bad := reducedTopics[tid]; bad {
				continue
			}
		}
		entries = append(entries, e)
	}

	// 排序：个性化时加关注加分
	followedSet := map[int64]struct{}{}
	var followedTopicSetMap map[int64]struct{}
	if personalized {
		ids, err := mysql.FollowedUserIDs(userID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			followedSet[id] = struct{}{}
		}
		tids, err := mysql.FollowedTopicIDs(userID)
		if err != nil {
			return nil, err
		}
		followedTopicSetMap = make(map[int64]struct{}, len(tids))
		for _, id := range tids {
			followedTopicSetMap[id] = struct{}{}
		}
	}
	items, hasMore := recommendPage(entries, followedSet, offset, limit)

	// 装配推荐理由与 exposure_id
	for _, item := range items {
		var aid int64
		aid, _ = strconv.ParseInt(item.Author.UserID, 10, 64)
		if personalized {
			if _, ok := followedSet[aid]; ok {
				item.RecommendationReason = "你关注了该作者"
			} else if item.Topic != nil && followedTopicSetMap != nil {
				var tid int64
				tid, _ = strconv.ParseInt(item.Topic.TopicID, 10, 64)
				if _, ok := followedTopicSetMap[tid]; ok {
					item.RecommendationReason = "你关注了该话题"
				}
			}
		}
		if item.RecommendationReason == "" {
			if time.Since(item.CreateTime) < 48*time.Hour {
				item.RecommendationReason = "近期新内容"
			} else {
				item.RecommendationReason = "近期热门"
			}
		}
		item.ExposureID = strconv.FormatInt(snowflake.GenerateID(), 10)
	}

	next := ""
	if hasMore {
		next = encodeOffsetCursor(offset+limit, snap.Version, userID)
	}
	return &models.PageResult[*models.FeedItem]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// filterEntriesByBlocks 热榜/推荐缓存的读时拉黑过滤（PRD F2）：
// 快照跨用户共享，过滤只能按 viewer 在读取时做；offset 游标在过滤后的列表上保持一致
// （同一 viewer 同一快照版本下过滤结果确定）。
func filterEntriesByBlocks(entries []cachedFeedEntry, viewerID int64) []cachedFeedEntry {
	if viewerID <= 0 || len(entries) == 0 {
		return entries
	}
	blocked, err := mysql.BlockedIDsFor(viewerID)
	if err != nil || len(blocked) == 0 {
		return entries
	}
	set := make(map[string]struct{}, len(blocked))
	for _, id := range blocked {
		set[strconv.FormatInt(id, 10)] = struct{}{}
	}
	out := make([]cachedFeedEntry, 0, len(entries))
	for _, e := range entries {
		if _, hit := set[e.Item.Author.UserID]; !hit {
			out = append(out, e)
		}
	}
	return out
}

// parseOffsetAndSnapshot 解析 offset 游标并校验快照版本：
// 游标携带的版本与当前缓存版本不一致时返回 ErrFeedSnapshotExpired（客户端应重刷首页）
func parseOffsetAndSnapshot(cursor string, userID int64) (int, *cachedFeedSnapshot, error) {
	offset, cursorVersion, cursorUser, err := parseOffsetCursor(cursor)
	if err != nil {
		return 0, nil, ErrCursorInvalid
	}
	// 快照绑定用户上下文：跨账号复用游标一律判过期
	if cursorUser != userID {
		return 0, nil, ErrFeedSnapshotExpired
	}
	snap, err := getHotFeedCache()
	if err != nil {
		return 0, nil, err
	}
	if cursorVersion != 0 && cursorVersion != snap.Version {
		return 0, nil, ErrFeedSnapshotExpired
	}
	return offset, snap, nil
}

// GetLatestFeed 最新流（时间 + 类型 + id 三字段游标，三类内容统一）
func GetLatestFeed(viewerID int64, cursor string, limit int) (*models.PageResult[*models.FeedItem], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var cursorTime time.Time
	var cursorType string
	var cursorID int64
	if cursor != "" {
		c, err := parseTimeCursor(cursor)
		if err != nil {
			return nil, ErrCursorInvalid
		}
		cursorTime = c.Time
		cursorType = c.Type
		cursorID = c.ID
	}
	rows, err := mysql.LatestFeedPage(cursorTime, cursorType, cursorID, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(rows) > limit
	if len(rows) > limit {
		rows = rows[:limit]
	}
	items, err := buildFeedItems(rows)
	if err != nil {
		return nil, err
	}
	// 拉黑过滤（PRD F2）：viewer 拉黑的作者不出现
	if viewerID > 0 {
		blockedSet := map[int64]struct{}{}
		blocked, _ := mysql.BlockedAuthorIDs(viewerID)
		for _, b := range blocked {
			blockedSet[b] = struct{}{}
		}
		filtered := make([]*models.FeedItem, 0, len(items))
		for _, it := range items {
			aid, _ := strconv.ParseInt(it.Author.UserID, 10, 64)
			if _, bad := blockedSet[aid]; !bad {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}
	next := ""
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		next = encodeFeedTimeCursor(last.CreateTime, last.ItemType, last.ItemID)
	}
	return &models.PageResult[*models.FeedItem]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// GetFollowingFeed 关注流（需登录）：关注作者 + 关注话题 + 关注专栏 多来源，
// 屏蔽作者过滤优先于关注（规格 4.2：屏蔽和隐藏规则优先于关注）
func GetFollowingFeed(userID int64, cursor string, limit int) (*models.PageResult[*models.FeedItem], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	authorIDs, err := mysql.FollowedUserIDs(userID)
	if err != nil {
		return nil, err
	}
	topicIDs, err := mysql.FollowedTopicIDs(userID)
	if err != nil {
		return nil, err
	}
	columnIDs, err := mysql.FollowedColumnIDs(userID)
	if err != nil {
		return nil, err
	}
	blocked, err := mysql.BlockedAuthorIDs(userID)
	if err != nil {
		return nil, err
	}
	blockedSet := make(map[int64]struct{}, len(blocked))
	for _, b := range blocked {
		blockedSet[b] = struct{}{}
	}
	// 屏蔽作者从关注来源中剔除（屏蔽优先于关注）
	if len(blockedSet) > 0 {
		authorIDs = filterIDs(authorIDs, blockedSet)
	}
	if len(authorIDs) == 0 && len(topicIDs) == 0 && len(columnIDs) == 0 {
		return &models.PageResult[*models.FeedItem]{
			Items: make([]*models.FeedItem, 0), NextCursor: "", HasMore: false,
		}, nil
	}

	var cursorTime time.Time
	var cursorType string
	var cursorID int64
	if cursor != "" {
		c, err := parseTimeCursor(cursor)
		if err != nil {
			return nil, ErrCursorInvalid
		}
		cursorTime = c.Time
		cursorType = c.Type
		cursorID = c.ID
	}
	rows, err := mysql.FollowingFeedPage(&mysql.FollowingFeedPageParams{
		AuthorIDs:  authorIDs,
		TopicIDs:   topicIDs,
		ColumnIDs:  columnIDs,
		CursorTime: cursorTime,
		CursorType: cursorType,
		CursorID:   cursorID,
		Limit:      limit + 1,
	})
	if err != nil {
		return nil, err
	}
	// 屏蔽过滤（SQL 内已按来源过滤，作者屏蔽在此兜底）
	filtered := make([]*mysql.FeedRow, 0, len(rows))
	for _, r := range rows {
		if _, bad := blockedSet[r.AuthorID]; bad {
			continue
		}
		filtered = append(filtered, r)
	}
	rows = filtered
	hasMore := len(rows) > limit
	if len(rows) > limit {
		rows = rows[:limit]
	}
	items, err := buildFeedItems(rows)
	if err != nil {
		return nil, err
	}
	// 多来源标注（规格 4.2：同一内容多路命中合并并返回来源数组）
	markSources(items, userID)
	next := ""
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		next = encodeFeedTimeCursor(last.CreateTime, last.ItemType, last.ItemID)
	}
	return &models.PageResult[*models.FeedItem]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// filterIDs 移除在 excluded 集合中的 id
func filterIDs(ids []int64, excluded map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, bad := excluded[id]; !bad {
			out = append(out, id)
		}
	}
	return out
}

// markSources 为关注流条目标注命中来源（author/topic/column）
func markSources(items []*models.FeedItem, userID int64) {
	if len(items) == 0 {
		return
	}
	authorIDs, _ := mysql.FollowedUserIDs(userID)
	topicIDs, _ := mysql.FollowedTopicIDs(userID)
	columnIDs, _ := mysql.FollowedColumnIDs(userID)
	authorSet := make(map[int64]struct{}, len(authorIDs))
	for _, id := range authorIDs {
		authorSet[id] = struct{}{}
	}
	topicSet := make(map[int64]struct{}, len(topicIDs))
	for _, id := range topicIDs {
		topicSet[id] = struct{}{}
	}
	columnSet := make(map[int64]struct{}, len(columnIDs))
	for _, id := range columnIDs {
		columnSet[id] = struct{}{}
	}
	for _, item := range items {
		sources := make([]string, 0, 3)
		var aid int64
		aid, _ = strconv.ParseInt(item.Author.UserID, 10, 64)
		if _, ok := authorSet[aid]; ok {
			sources = append(sources, "author")
		}
		if item.Topic != nil {
			var tid int64
			tid, _ = strconv.ParseInt(item.Topic.TopicID, 10, 64)
			if _, ok := topicSet[tid]; ok {
				sources = append(sources, "topic")
			}
		}
		if item.Column != nil {
			var cid int64
			cid, _ = strconv.ParseInt(item.Column.ColumnID, 10, 64)
			if _, ok := columnSet[cid]; ok {
				sources = append(sources, "column")
			}
		}
		if len(sources) > 0 {
			item.Sources = sources
		}
	}
}

// ---------- 游标辅助 ----------

type timeCursor struct {
	Time time.Time `json:"t"`
	Type string    `json:"ty,omitempty"` // item_type，排序 tie-breaker
	ID   int64     `json:"i"`
}

func encodeTimeCursor(t time.Time, id int64) string {
	return base64Encode(marshalJSONMust(&timeCursor{Time: t, ID: id}))
}

func parseTimeCursor(cursor string) (*timeCursor, error) {
	raw, err := base64Decode(cursor)
	if err != nil {
		return nil, ErrCursorInvalid
	}
	var c timeCursor
	if err := unmarshalJSONMust(raw, &c); err != nil {
		return nil, ErrCursorInvalid
	}
	return &c, nil
}

// encodeFeedTimeCursor Feed 游标：时间 + 类型 + ID 三字段排序键
func encodeFeedTimeCursor(t time.Time, itemType string, id int64) string {
	return base64Encode(marshalJSONMust(&timeCursor{Time: t, Type: itemType, ID: id}))
}

type offsetCursor struct {
	Offset  int   `json:"o"`
	Version int64 `json:"v,omitempty"` // 绑定的热榜缓存快照版本
	UserID  int64 `json:"u,omitempty"` // 快照绑定用户（游客为 0），防止跨账号复用游标
}

func encodeOffsetCursor(offset int, version int64, userID int64) string {
	return base64Encode(marshalJSONMust(&offsetCursor{Offset: offset, Version: version, UserID: userID}))
}

func parseOffsetCursor(cursor string) (int, int64, int64, error) {
	if cursor == "" {
		return 0, 0, 0, nil
	}
	raw, err := base64Decode(cursor)
	if err != nil {
		return 0, 0, 0, ErrCursorInvalid
	}
	var c offsetCursor
	if err := unmarshalJSONMust(raw, &c); err != nil {
		return 0, 0, 0, ErrCursorInvalid
	}
	if c.Offset < 0 {
		return 0, 0, 0, ErrCursorInvalid
	}
	return c.Offset, c.Version, c.UserID, nil
}
