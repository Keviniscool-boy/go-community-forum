package logic

import (
	"errors"

	"bluebell/dao/mysql"
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
