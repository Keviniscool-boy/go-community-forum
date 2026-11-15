package logic

import (
	"database/sql"
	"errors"
	"strconv"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"
	"go.uber.org/zap"
)

// 互动相关业务错误
var (
	ErrAnswerNotExist   = errors.New("回答不存在")
	ErrCommentInvalid   = errors.New("评论参数不合法")
)

// targetIDToAuthorID 返回目标内容的作者 id（用于通知）
func targetAuthorID(targetType int32, targetID int64) (int64, error) {
	switch targetType {
	case models.TargetAnswer:
		a, err := mysql.GetAnswerByID(targetID)
		if err != nil {
			return 0, err
		}
		return a.AuthorID, nil
	case models.TargetQuestion:
		q, err := mysql.GetQuestionByID(targetID)
		if err != nil {
			return 0, err
		}
		return q.AuthorID, nil
	}
	return 0, ErrCommentInvalid
}

// createNotification 写一条通知（忽略失败，不影响主流程）
func createNotification(userID, actorID int64, notifyType int32, targetType int32, targetID int64) {
	n := &models.Notification{
		NotificationID: snowflake.GenerateID(),
		UserID:         userID,
		ActorID:        actorID,
		Type:           notifyType,
		TargetType:     targetType,
		TargetID:       targetID,
	}
	if err := mysql.CreateNotification(n); err != nil {
		zap.L().Warn("create notification failed", zap.Error(err))
	}
}

// ---------- 关注用户 ----------

func FollowUser(followerID, followingID int64) error {
	if err := mysql.FollowUser(followerID, followingID); err != nil {
		return err
	}
	createNotification(followingID, followerID, models.NotifyTypeFollow, 0, followerID)
	return nil
}

func UnfollowUser(followerID, followingID int64) error {
	return mysql.UnfollowUser(followerID, followingID)
}

func IsFollowing(followerID, followingID int64) (bool, error) {
	if followerID <= 0 {
		return false, nil
	}
	return mysql.IsFollowing(followerID, followingID)
}

// ---------- 回答投票 ----------

// VoteAnswer 投票并记录通知
func VoteAnswer(userID, answerID int64, direction int8) error {
	// 校验回答存在
	a, err := mysql.GetAnswerByID(answerID)
	if err != nil {
		if errors.Is(err, mysql.ErrAnswerNotExist) {
			return ErrAnswerNotExist
		}
		return err
	}
	if _, err = mysql.VoteAnswer(userID, answerID, direction); err != nil {
		return err
	}
	// 只有新赞同才通知（取消/改反对不通知）
	if direction == 1 {
		createNotification(a.AuthorID, userID, models.NotifyTypeLike, models.TargetAnswer, answerID)
	}
	return nil
}

func GetUserVoteDirection(userID, answerID int64) (int8, error) {
	if userID <= 0 {
		return 0, nil
	}
	return mysql.GetUserVoteDirection(userID, answerID)
}

// ---------- 收藏 ----------

func CollectAnswer(userID, answerID int64) error {
	a, err := mysql.GetAnswerByID(answerID)
	if err != nil {
		if errors.Is(err, mysql.ErrAnswerNotExist) {
			return ErrAnswerNotExist
		}
		return err
	}
	if err = mysql.CollectAnswer(userID, answerID); err != nil {
		return err
	}
	createNotification(a.AuthorID, userID, models.NotifyTypeCollect, models.TargetAnswer, answerID)
	return nil
}

func UncollectAnswer(userID, answerID int64) error {
	return mysql.UncollectAnswer(userID, answerID)
}

func IsCollected(userID, answerID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	return mysql.IsCollected(userID, answerID)
}

// ---------- 关注问题 ----------

func FollowQuestion(userID, questionID int64) error {
	q, err := mysql.GetQuestionByID(questionID)
	if err != nil {
		if errors.Is(err, mysql.ErrQuestionNotExist) {
			return ErrQuestionNotExist
		}
		return err
	}
	if err = mysql.FollowQuestion(userID, questionID); err != nil {
		return err
	}
	createNotification(q.AuthorID, userID, models.NotifyTypeFollow, models.TargetQuestion, questionID)
	return nil
}

func UnfollowQuestion(userID, questionID int64) error {
	return mysql.UnfollowQuestion(userID, questionID)
}

func IsQuestionFollowed(userID, questionID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	return mysql.IsQuestionFollowed(userID, questionID)
}

// ---------- 评论 ----------

// CreateComment 发表评论并通知目标作者
func CreateComment(p *models.ParamCreateComment, userID int64) (commentID int64, err error) {
	targetType := models.TargetTypeToCode(p.TargetType)
	if targetType == 0 {
		return 0, ErrCommentInvalid
	}
	targetID, err := strconv.ParseInt(p.TargetID, 10, 64)
	if err != nil || targetID <= 0 {
		return 0, ErrCommentInvalid
	}
	var parentID int64
	if p.ParentID != "" {
		parentID, err = strconv.ParseInt(p.ParentID, 10, 64)
		if err != nil || parentID <= 0 {
			return 0, ErrCommentInvalid
		}
		// 父评论必须存在且属于同一目标
		parent, err := mysql.GetCommentByID(parentID)
		if err != nil {
			return 0, ErrCommentInvalid
		}
		if parent.TargetType != targetType || parent.TargetID != targetID {
			return 0, ErrCommentInvalid
		}
	}
	// 目标必须存在
	if _, err = targetAuthorID(targetType, targetID); err != nil {
		if errors.Is(err, mysql.ErrAnswerNotExist) {
			return 0, ErrAnswerNotExist
		}
		if errors.Is(err, mysql.ErrQuestionNotExist) {
			return 0, ErrQuestionNotExist
		}
		return 0, err
	}
	cm := &models.Comment{
		CommentID:  snowflake.GenerateID(),
		UserID:     userID,
		TargetType: targetType,
		TargetID:   targetID,
		ParentID:   parentID,
		Content:    p.Content,
	}
	if err = mysql.CreateComment(cm, 0); err != nil {
		return 0, err
	}
	// 通知目标作者
	if authorID, err := targetAuthorID(targetType, targetID); err == nil {
		createNotification(authorID, userID, models.NotifyTypeComment, targetType, targetID)
	}
	return cm.CommentID, nil
}

// ListComments 评论列表（cursor 分页）
func ListComments(targetTypeStr, targetIDStr, cursor string, limit int) (*models.PageResult[*models.ApiCommentDetail], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	targetType := models.TargetTypeToCode(targetTypeStr)
	if targetType == 0 {
		return nil, ErrCommentInvalid
	}
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil || targetID <= 0 {
		return nil, ErrCommentInvalid
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
	comments, err := mysql.ListComments(&mysql.CommentPageParams{
		TargetType: targetType,
		TargetID:   targetID,
		CursorTime: cursorTime,
		CursorID:   cursorID,
		Limit:      limit,
	})
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(comments) > limit {
		hasMore = true
		comments = comments[:limit]
	}
	// 批量查评论者
	userIDs := make([]int64, 0, len(comments))
	for _, cm := range comments {
		userIDs = append(userIDs, cm.UserID)
	}
	users, err := mysql.GetUsersByIDs(userIDs)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*models.User, len(users))
	for _, u := range users {
		userMap[u.UserID] = u
	}
	items := make([]*models.ApiCommentDetail, 0, len(comments))
	var last *models.Comment
	for _, cm := range comments {
		last = cm
		detail := &models.ApiCommentDetail{Comment: cm}
		if u := userMap[cm.UserID]; u != nil {
			detail.Username = u.Username
			detail.AvatarURL = u.AvatarURL
		}
		items = append(items, detail)
	}
	next := ""
	if hasMore && last != nil {
		next = encodeTimeCursor(last.CreateTime, last.CommentID)
	}
	return &models.PageResult[*models.ApiCommentDetail]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}
