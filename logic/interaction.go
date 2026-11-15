package logic

import (
	"database/sql"
	"errors"
	"strconv"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// 互动相关业务错误
var ErrAnswerNotExist = errors.New("回答不存在")
var ErrCommentInvalid = errors.New("评论参数不合法")

// ---------- 关注用户 ----------

// FollowUser 关注用户（通知在 dao 事务内写入）
func FollowUser(followerID, followingID int64) error {
	return mysql.FollowUser(followerID, followingID)
}

// UnfollowUser 取消关注
func UnfollowUser(followerID, followingID int64) error {
	return mysql.UnfollowUser(followerID, followingID)
}

// IsFollowing 查询关注关系
func IsFollowing(followerID, followingID int64) (bool, error) {
	if followerID <= 0 {
		return false, nil
	}
	return mysql.IsFollowing(followerID, followingID)
}

// ---------- 回答投票 ----------

// VoteAnswer 投票（通知在 dao 事务内写入）
func VoteAnswer(userID, answerID int64, direction int8) error {
	return mysql.VoteAnswer(userID, answerID, direction)
}

// GetUserVoteDirection 当前用户对回答的投票方向
func GetUserVoteDirection(userID, answerID int64) (int8, error) {
	if userID <= 0 {
		return 0, nil
	}
	return mysql.GetUserVoteDirection(userID, answerID)
}

// ---------- 收藏 ----------

// CollectAnswer 收藏回答（通知在 dao 事务内写入）
func CollectAnswer(userID, answerID int64) error {
	return mysql.CollectAnswer(userID, answerID)
}

// UncollectAnswer 取消收藏
func UncollectAnswer(userID, answerID int64) error {
	return mysql.UncollectAnswer(userID, answerID)
}

// IsCollected 是否已收藏
func IsCollected(userID, answerID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	return mysql.IsCollected(userID, answerID)
}

// ---------- 关注问题 ----------

// FollowQuestion 关注问题（通知在 dao 事务内写入）
func FollowQuestion(userID, questionID int64) error {
	return mysql.FollowQuestion(userID, questionID)
}

// UnfollowQuestion 取消关注问题
func UnfollowQuestion(userID, questionID int64) error {
	return mysql.UnfollowQuestion(userID, questionID)
}

// IsQuestionFollowed 是否已关注问题
func IsQuestionFollowed(userID, questionID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	return mysql.IsQuestionFollowed(userID, questionID)
}

// ---------- 评论 ----------

// CreateComment 发表评论（评论、计数、通知在同一事务）
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
	cm := &models.Comment{
		CommentID:  snowflake.GenerateID(),
		UserID:     userID,
		TargetType: targetType,
		TargetID:   targetID,
		ParentID:   parentID,
		Content:    p.Content,
	}
	// 目标不存在时 dao 层事务内返回 ErrAnswerNotExist / ErrQuestionNotExist
	if err = mysql.CreateComment(cm); err != nil {
		switch {
		case errors.Is(err, mysql.ErrAnswerNotExist):
			return 0, ErrAnswerNotExist
		case errors.Is(err, mysql.ErrQuestionNotExist):
			return 0, ErrQuestionNotExist
		default:
			return 0, err
		}
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
