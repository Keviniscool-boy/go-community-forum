package logic

import (
	"database/sql"
	"strconv"

	"bluebell/dao/mysql"
	"bluebell/models"
)

// 通知相关

// ListNotifications 用户通知列表（cursor 分页）
func ListNotifications(userID int64, cursor string, limit int) (*models.PageResult[*models.ApiNotificationDetail], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var cursorTime sql.NullTime
	var cursorID int64
	if cursor != "" {
		c, err := parseTimeCursor(cursor)
		if err != nil {
			return nil, ErrCursorInvalid
		}
		cursorTime.Time = c.Time
		cursorID = c.ID
	}
	ns, err := mysql.ListNotifications(userID, cursorTime, cursorID, limit)
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(ns) > limit {
		hasMore = true
		ns = ns[:limit]
	}
	// 批量查触发者
	actorIDs := make([]int64, 0, len(ns))
	for _, n := range ns {
		actorIDs = append(actorIDs, n.ActorID)
	}
	users, err := mysql.GetUsersByIDs(actorIDs)
	if err != nil {
		return nil, err
	}
	actorMap := make(map[int64]*models.User, len(users))
	for _, u := range users {
		actorMap[u.UserID] = u
	}
	// 展示层聚合（规格 10.1）：同批次内 type=赞同 且目标相同的通知合并为"某某等 N 人"，
	// 不改变原始记录与审计事实；跨页不聚合
	items := make([]*models.ApiNotificationDetail, 0, len(ns))
	var last *models.Notification
	for _, n := range ns {
		if n.Type == models.NotifyTypeLike && len(items) > 0 {
			prev := items[len(items)-1]
			if prev.Type == models.NotifyTypeLike && prev.TargetType == n.TargetType && prev.TargetID == n.TargetID {
				prev.AggCount++
				continue
			}
		}
		last = n
		detail := &models.ApiNotificationDetail{
			Notification: n,
			Excerpt:      notifyText(n.Type),
			AggCount:     1,
		}
		if u := actorMap[n.ActorID]; u != nil {
			detail.ActorName = u.Username
		}
		items = append(items, detail)
	}
	next := ""
	if hasMore && last != nil {
		next = encodeTimeCursor(last.CreateTime, last.NotificationID)
	}
	return &models.PageResult[*models.ApiNotificationDetail]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// notifyText 通知文案
func notifyText(t int32) string {
	switch t {
	case models.NotifyTypeLike:
		return "赞同了你的内容"
	case models.NotifyTypeComment:
		return "评论了你的内容"
	case models.NotifyTypeFollow:
		return "关注了你"
	case models.NotifyTypeCollect:
		return "收藏了你的回答"
	}
	return "与你互动"
}

// CountUnreadNotifications 未读通知数
func CountUnreadNotifications(userID int64) (int, error) {
	return mysql.CountUnreadNotifications(userID)
}

// MarkAllNotificationsRead 全部已读
func MarkAllNotificationsRead(userID int64) error {
	return mysql.MarkAllNotificationsRead(userID)
}

// ---------- 用户主页 ----------

// ApiUserProfile 用户主页数据
type ApiUserProfile struct {
	UserID         string `json:"user_id,string"`
	Username       string `json:"username"`
	AvatarURL      string `json:"avatar_url"`
	Bio            string `json:"bio"`
	QuestionCount  int    `json:"question_count"`
	AnswerCount    int    `json:"answer_count"`
	FollowerCount  int    `json:"follower_count"`
	FollowingCount int    `json:"following_count"`
	IsFollowing    bool   `json:"is_following"`
}

// GetUserProfile 用户主页：基本信息 + 内容统计 + 关注关系
func GetUserProfile(targetID int64, viewerID int64) (*ApiUserProfile, error) {
	user, err := mysql.GetUserDetailByID(targetID)
	if err != nil {
		return nil, err
	}
	questionCount, err := mysql.CountUserQuestions(targetID)
	if err != nil {
		return nil, err
	}
	answerCount, err := mysql.CountUserAnswers(targetID)
	if err != nil {
		return nil, err
	}
	followerCount, err := mysql.CountFollowers(targetID)
	if err != nil {
		return nil, err
	}
	followingCount, err := mysql.CountFollowings(targetID)
	if err != nil {
		return nil, err
	}
	isFollowing := false
	if viewerID > 0 && viewerID != targetID {
		isFollowing, err = mysql.IsFollowing(viewerID, targetID)
		if err != nil {
			return nil, err
		}
	}
	return &ApiUserProfile{
		UserID:         strconv.FormatInt(user.UserID, 10),
		Username:       user.Username,
		AvatarURL:      user.AvatarURL,
		Bio:            user.Bio,
		QuestionCount:  questionCount,
		AnswerCount:    answerCount,
		FollowerCount:  followerCount,
		FollowingCount: followingCount,
		IsFollowing:    isFollowing,
	}, nil
}

// ListUserQuestions 用户的提问列表
func ListUserQuestions(userID int64, limit int) (questions []*models.Question, err error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return mysql.ListUserQuestions(userID, limit)
}

// ListUserAnswers 用户的回答列表（带所属问题标题）
func ListUserAnswers(userID int64, limit int) (answers []*models.ApiAnswerDetail, err error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return mysql.ListUserAnswers(userID, limit)
}
