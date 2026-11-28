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
		item.Title = r.Title
		if t, ok := topicMap[r.QuestionID]; ok && len(t) > 0 {
			item.Topic = t[0]
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

// GetRecommendFeed 推荐流：热榜基础分 + 关注加分（未登录时与热榜一致）
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
	followed := map[int64]struct{}{}
	if userID > 0 {
		ids, err := mysql.FollowedUserIDs(userID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			followed[id] = struct{}{}
		}
	}
	items, hasMore := recommendPage(filterEntriesByBlocks(snap.Entries, userID), followed, offset, limit)
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

// GetLatestFeed 最新流（时间 + id 双字段游标）
func GetLatestFeed(viewerID int64, cursor string, limit int) (*models.PageResult[*models.FeedItem], error) {
	return timeOrderedFeed(viewerID, cursor, limit, nil)
}

// GetFollowingFeed 关注流（需登录）
func GetFollowingFeed(userID int64, cursor string, limit int) (*models.PageResult[*models.FeedItem], error) {
	ids, err := mysql.FollowedUserIDs(userID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &models.PageResult[*models.FeedItem]{
			Items: make([]*models.FeedItem, 0), NextCursor: "", HasMore: false,
		}, nil
	}
	return timeOrderedFeed(userID, cursor, limit, ids)
}

// timeOrderedFeed 时间序 Feed 的公共实现（authorIDs 为 nil 时不限作者）
func timeOrderedFeed(viewerID int64, cursor string, limit int, authorIDs []int64) (*models.PageResult[*models.FeedItem], error) {
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
	var rows []*mysql.FeedRow
	var err error
	if authorIDs == nil {
		rows, err = mysql.LatestFeedPage(cursorTime, cursorType, cursorID, limit+1, viewerID)
	} else {
		rows, err = mysql.FollowingFeedPage(authorIDs, cursorTime, cursorType, cursorID, limit+1, viewerID)
	}
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(rows) > limit {
		hasMore = true
		rows = rows[:limit]
	}
	items, err := buildFeedItems(rows)
	if err != nil {
		return nil, err
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
