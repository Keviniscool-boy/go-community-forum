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
	// 回答类通知的 target_id 是回答 ID，补查所属问题 ID 供前端跳转（批量，避免 N+1）
	answerIDs := make([]int64, 0, len(ns))
	for _, n := range ns {
		if n.TargetType == models.TargetAnswer {
			answerIDs = append(answerIDs, n.TargetID)
		}
	}
	questionIDs, err := mysql.AnswerQuestionIDs(answerIDs)
	if err != nil {
		return nil, err
	}
	items := make([]*models.ApiNotificationDetail, 0, len(ns))
	var last *models.Notification
	for _, n := range ns {
		last = n
		detail := &models.ApiNotificationDetail{
			Notification: n,
			Excerpt:      notifyText(n.Type),
		}
		if u := actorMap[n.ActorID]; u != nil {
			detail.ActorName = u.Username
		}
		if qid, ok := questionIDs[n.TargetID]; ok {
			detail.QuestionID = strconv.FormatInt(qid, 10)
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
	case models.NotifyTypeCommentLike:
		return "赞同了你的评论"
	case models.NotifyTypeReply:
		return "回复了你的评论"
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
	// 双向屏蔽统一按不存在处理（PRD F2 内容可见性；防关系探测）
	if viewerID > 0 && viewerID != targetID {
		if blocked, berr := mysql.IsBlockedBetween(targetID, viewerID); berr == nil && blocked {
			return nil, mysql.ErrUserNotExist
		}
	}
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
