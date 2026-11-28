package logic

import (
	"database/sql"
	"strconv"

	"bluebell/dao/mysql"
	"bluebell/models"

	"github.com/jmoiron/sqlx"
)

// 通知相关

// ListNotifications 用户通知列表（cursor 分页；typeFilter 非空时按类型过滤）
func ListNotifications(userID int64, cursor string, limit int, typeFilter string) (*models.PageResult[*models.ApiNotificationDetail], error) {
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
	ns, err := mysql.ListNotifications(userID, cursorTime, cursorID, limit, typeFilter)
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
	// 邀请/新回答类通知（8-11）：批量关联问题标题与跳转 question_id
	// type 8 target=question；type 9 target=invitation（需解析 question_id）；type 10/11 target=question
	qInfo := loadQuestionInfoForNotifications(ns)
	// 回答类通知的 target_id 是回答 ID，补查所属问题 ID 供前端跳转（main 第四阶段；批量避免 N+1）
	answerIDs := make([]int64, 0, len(ns))
	for _, n := range ns {
		if n.TargetType == models.TargetAnswer {
			answerIDs = append(answerIDs, n.TargetID)
		}
	}
	questionIDs, qerr := mysql.AnswerQuestionIDs(answerIDs)
	if qerr != nil {
		return nil, qerr
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
		excerpt := notifyText(n.Type)
		detail := &models.ApiNotificationDetail{
			Notification: n,
			Excerpt:      excerpt,
			AggCount:     1,
		}
		if info := qInfo[key64(int64(n.Type), n.TargetID)]; info != nil {
			if info.Title != "" {
				detail.Excerpt = excerpt + "：" + truncateRunes(info.Title, 50)
			}
			if info.QuestionID != 0 {
				detail.QuestionID = strconv.FormatInt(info.QuestionID, 10)
			}
			if n.Type == models.NotifyTypeNewAnswer && n.UserID == info.AuthorID {
				detail.Excerpt = "回答了你的问题"
				if info.Title != "" {
					detail.Excerpt = "回答了你的问题：" + truncateRunes(info.Title, 50)
				}
			}
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
	case models.NotifyTypeAccept:
		return "采纳了你的回答"
	case models.NotifyTypeUnaccept:
		return "撤销了对你回答的采纳"
	case models.NotifyTypeNewAnswer:
		return "回答了你的问题"
	case models.NotifyTypeAnswerInvite:
		return "邀请你回答"
	case models.NotifyTypeInviteAccepted:
		return "接受了你的回答邀请"
	case models.NotifyTypeInviteDeclined:
		return "婉拒了你的回答邀请"
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
// 被禁用账号统一按不存在处理（防枚举，与内容可见策略一致）
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
	if user.Status != 1 {
		return nil, mysql.ErrUserNotExist
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

// ListUserArticles 用户的公开文章列表（用户主页作品聚合）
func ListUserArticles(userID int64, limit int) (articles []*models.Article, err error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return mysql.ListArticlesByAuthor(userID, limit)
}

// ListUserColumns 用户的公开专栏列表（用户主页作品聚合）
func ListUserColumns(userID int64, limit int) (columns []*models.Column, err error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	columns, err = mysql.ListPublicColumnsByOwner(userID)
	if err != nil {
		return nil, err
	}
	if len(columns) > limit {
		columns = columns[:limit]
	}
	return columns, nil
}

// notifQuestionInfo 通知关联的问题信息
type notifQuestionInfo struct {
	Title     string
	AuthorID  int64
	QuestionID int64 // type 9 经邀请解析出的问题 id
}

func key64(a, b int64) string {
	return strconv.FormatInt(a, 10) + ":" + strconv.FormatInt(b, 10)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// loadQuestionInfoForNotifications 批量解析 8-11 类通知关联的问题（标题/作者/跳转 id）
func loadQuestionInfoForNotifications(ns []*models.Notification) map[string]*notifQuestionInfo {
	out := make(map[string]*notifQuestionInfo)
	var questionIDs, invitationIDs []int64
	seenQ, seenI := make(map[int64]struct{}), make(map[int64]struct{})
	for _, n := range ns {
		switch n.Type {
		case models.NotifyTypeNewAnswer, models.NotifyTypeInviteAccepted, models.NotifyTypeInviteDeclined:
			if _, ok := seenQ[n.TargetID]; !ok {
				seenQ[n.TargetID] = struct{}{}
				questionIDs = append(questionIDs, n.TargetID)
			}
		case models.NotifyTypeAnswerInvite:
			if _, ok := seenI[n.TargetID]; !ok {
				seenI[n.TargetID] = struct{}{}
				invitationIDs = append(invitationIDs, n.TargetID)
			}
		}
	}
	if len(questionIDs) > 0 {
		rows := make([]struct {
			QuestionID int64  `db:"question_id"`
			Title      string `db:"title"`
			AuthorID   int64  `db:"author_id"`
		}, 0)
		query, args, err := sqlx.In("SELECT question_id, title, author_id FROM question WHERE question_id IN (?)", questionIDs)
		if err == nil {
			if err = mysql.GetGlobalDB().Select(&rows, query, args...); err == nil {
				for _, r := range rows {
					out[key64(models.NotifyTypeNewAnswer, r.QuestionID)] = &notifQuestionInfo{Title: r.Title, AuthorID: r.AuthorID, QuestionID: r.QuestionID}
					out[key64(models.NotifyTypeInviteAccepted, r.QuestionID)] = &notifQuestionInfo{Title: r.Title, AuthorID: r.AuthorID, QuestionID: r.QuestionID}
					out[key64(models.NotifyTypeInviteDeclined, r.QuestionID)] = &notifQuestionInfo{Title: r.Title, AuthorID: r.AuthorID, QuestionID: r.QuestionID}
				}
			}
		}
	}
	if len(invitationIDs) > 0 {
		rows := make([]struct {
			InvitationID int64  `db:"invitation_id"`
			QuestionID   int64  `db:"question_id"`
			Title        string `db:"title"`
		}, 0)
		query, args, err := sqlx.In(`SELECT inv.invitation_id, inv.question_id, qu.title
			FROM question_answer_invitation inv JOIN question qu ON qu.question_id = inv.question_id
			WHERE inv.invitation_id IN (?)`, invitationIDs)
		if err == nil {
			if err = mysql.GetGlobalDB().Select(&rows, query, args...); err == nil {
				for _, r := range rows {
					out[key64(models.NotifyTypeAnswerInvite, r.InvitationID)] = &notifQuestionInfo{Title: r.Title, QuestionID: r.QuestionID}
				}
			}
		}
	}
	return out
}
