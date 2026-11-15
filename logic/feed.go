package logic

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"time"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
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

// getHotFeedCache 读取热榜缓存；缓存过期或为空时重建（加锁避免并发重复计算）
func getHotFeedCache() ([]cachedFeedEntry, error) {
	if data, _ := redis.GetFeedHotCache(); data != nil {
		var entries []cachedFeedEntry
		if err := json.Unmarshal(data, &entries); err == nil {
			return entries, nil
		}
	}
	// 尝试拿刷新锁
	ok, err := redis.SetFeedRefreshLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		// 别的请求正在刷新，稍后重试；本轮直接返回空列表
		return []cachedFeedEntry{}, nil
	}
	defer redis.ReleaseFeedRefreshLock()

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
	if data, err := json.Marshal(entries); err == nil {
		if err := redis.SetFeedHotCache(data); err != nil {
			zap.L().Warn("cache hot feed failed", zap.Error(err))
		}
	}
	return entries, nil
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

// GetHotFeed 热榜（offset 游标）
func GetHotFeed(cursor string, limit int) (*models.PageResult[*models.FeedItem], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	offset, err := parseOffsetCursor(cursor)
	if err != nil {
		return nil, ErrCursorInvalid
	}
	entries, err := getHotFeedCache()
	if err != nil {
		return nil, err
	}
	items, hasMore := sliceFeedPage(entries, offset, limit)
	next := ""
	if hasMore {
		next = encodeOffsetCursor(offset + limit)
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
	offset, err := parseOffsetCursor(cursor)
	if err != nil {
		return nil, ErrCursorInvalid
	}
	entries, err := getHotFeedCache()
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
	items, hasMore := recommendPage(entries, followed, offset, limit)
	next := ""
	if hasMore {
		next = encodeOffsetCursor(offset + limit)
	}
	return &models.PageResult[*models.FeedItem]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// GetLatestFeed 最新流（时间 + id 双字段游标）
func GetLatestFeed(cursor string, limit int) (*models.PageResult[*models.FeedItem], error) {
	return timeOrderedFeed(cursor, limit, nil)
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
	return timeOrderedFeed(cursor, limit, ids)
}

// timeOrderedFeed 时间序 Feed 的公共实现（authorIDs 为 nil 时不限作者）
func timeOrderedFeed(cursor string, limit int, authorIDs []int64) (*models.PageResult[*models.FeedItem], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var cursorTime time.Time
	var cursorID int64
	if cursor != "" {
		c, err := parseTimeCursor(cursor)
		if err != nil {
			return nil, ErrCursorInvalid
		}
		cursorTime = c.Time
		cursorID = c.ID
	}
	var rows []*mysql.FeedRow
	var err error
	if authorIDs == nil {
		rows, err = mysql.LatestFeedPage(cursorTime, cursorID, limit+1)
	} else {
		rows, err = mysql.FollowingFeedPage(authorIDs, cursorTime, cursorID, limit+1)
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
		next = encodeTimeCursor(last.CreateTime, last.ItemID)
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

type offsetCursor struct {
	Offset int `json:"o"`
}

func encodeOffsetCursor(offset int) string {
	return base64Encode(marshalJSONMust(&offsetCursor{Offset: offset}))
}

func parseOffsetCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64Decode(cursor)
	if err != nil {
		return 0, ErrCursorInvalid
	}
	var c offsetCursor
	if err := unmarshalJSONMust(raw, &c); err != nil {
		return 0, ErrCursorInvalid
	}
	if c.Offset < 0 {
		return 0, ErrCursorInvalid
	}
	return c.Offset, nil
}
