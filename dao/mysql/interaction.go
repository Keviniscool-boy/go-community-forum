package mysql

import (
	"database/sql"
	"errors"

	"bluebell/models"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

var (
	ErrCannotFollowSelf       = errors.New("不能关注自己")
	ErrFollowRelationNotExist = errors.New("关注关系不存在")
	ErrCommentNotExist        = errors.New("评论不存在")
	ErrInvalidParams          = errors.New("参数不合法")
)

// FollowUser 关注用户（幂等，重复关注无副作用）
func FollowUser(followerID, followingID int64) error {
	if followerID == followingID {
		return ErrCannotFollowSelf
	}
	_, err := db.Exec("INSERT IGNORE INTO user_follow (follower_id, following_id) VALUES (?, ?)", followerID, followingID)
	return err
}

// UnfollowUser 取消关注；关系不存在时返回错误供 logic 提示
func UnfollowUser(followerID, followingID int64) error {
	result, err := db.Exec("DELETE FROM user_follow WHERE follower_id = ? AND following_id = ?", followerID, followingID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrFollowRelationNotExist
	}
	return nil
}

// IsFollowing 查询关注关系是否存在
func IsFollowing(followerID, followingID int64) (bool, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(id) FROM user_follow WHERE follower_id = ? AND following_id = ?", followerID, followingID)
	return count > 0, err
}

// FollowedUserIDs 查询用户关注的全部用户 id（Feed 关注流用）
func FollowedUserIDs(userID int64) (ids []int64, err error) {
	ids = make([]int64, 0)
	err = db.Select(&ids, "SELECT following_id FROM user_follow WHERE follower_id = ?", userID)
	return
}

// CountFollowings / CountFollowers 关注数与粉丝数
func CountFollowings(userID int64) (int, error) {
	var n int
	err := db.Get(&n, "SELECT COUNT(id) FROM user_follow WHERE follower_id = ?", userID)
	return n, err
}

func CountFollowers(userID int64) (int, error) {
	var n int
	err := db.Get(&n, "SELECT COUNT(id) FROM user_follow WHERE following_id = ?", userID)
	return n, err
}

// ---------- 回答投票 ----------

// VoteAnswer 对回答投票：upsert 方向，幂等；返回票数增量供更新 vote_count
// 返回增量 = 新状态贡献 - 旧状态贡献（1 赞同 +1，反对 -1，取消 0）
func VoteAnswer(userID, answerID int64, direction int8) (delta int, err error) {
	if direction != 1 && direction != -1 && direction != 0 {
		return 0, ErrInvalidParams
	}
	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var old int8
	err = tx.Get(&old, "SELECT direction FROM answer_vote WHERE user_id = ? AND answer_id = ?", userID, answerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	hasOld := !errors.Is(err, sql.ErrNoRows)

	contrib := func(d int8) int {
		switch d {
		case 1:
			return 1
		case -1:
			return -1
		}
		return 0
	}

	if direction == 0 {
		// 取消投票：删除记录
		if hasOld {
			if _, err = tx.Exec("DELETE FROM answer_vote WHERE user_id = ? AND answer_id = ?", userID, answerID); err != nil {
				return 0, err
			}
		}
	} else if !hasOld {
		if _, err = tx.Exec("INSERT INTO answer_vote (user_id, answer_id, direction) VALUES (?, ?, ?)", userID, answerID, direction); err != nil {
			return 0, err
		}
	} else {
		if _, err = tx.Exec("UPDATE answer_vote SET direction = ? WHERE user_id = ? AND answer_id = ?", direction, userID, answerID); err != nil {
			return 0, err
		}
	}
	delta = contrib(direction) - contrib(old)
	if _, err = tx.Exec("UPDATE answer SET vote_count = vote_count + ? WHERE answer_id = ?", delta, answerID); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return delta, nil
}

// GetUserVoteDirection 查询用户对某回答的投票方向（0 表示未投票）
func GetUserVoteDirection(userID, answerID int64) (int8, error) {
	var d int8
	err := db.Get(&d, "SELECT direction FROM answer_vote WHERE user_id = ? AND answer_id = ?", userID, answerID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return d, err
}

// ---------- 收藏 ----------

// CollectAnswer 收藏回答（幂等）
func CollectAnswer(userID, answerID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("INSERT IGNORE INTO answer_collect (user_id, answer_id) VALUES (?, ?)", userID, answerID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		if _, err = tx.Exec("UPDATE answer SET collect_count = collect_count + 1 WHERE answer_id = ?", answerID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UncollectAnswer 取消收藏
func UncollectAnswer(userID, answerID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("DELETE FROM answer_collect WHERE user_id = ? AND answer_id = ?", userID, answerID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		if _, err = tx.Exec("UPDATE answer SET collect_count = collect_count - 1 WHERE answer_id = ?", answerID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// IsCollected 查询是否已收藏
func IsCollected(userID, answerID int64) (bool, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(id) FROM answer_collect WHERE user_id = ? AND answer_id = ?", userID, answerID)
	return count > 0, err
}

// ---------- 问题关注 ----------

// FollowQuestion 关注问题（幂等）
func FollowQuestion(userID, questionID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("INSERT IGNORE INTO question_follow (user_id, question_id) VALUES (?, ?)", userID, questionID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		if _, err = tx.Exec("UPDATE question SET follower_count = follower_count + 1 WHERE question_id = ?", questionID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UnfollowQuestion 取消关注问题
func UnfollowQuestion(userID, questionID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("DELETE FROM question_follow WHERE user_id = ? AND question_id = ?", userID, questionID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		if _, err = tx.Exec("UPDATE question SET follower_count = follower_count - 1 WHERE question_id = ?", questionID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// IsQuestionFollowed 查询是否已关注问题
func IsQuestionFollowed(userID, questionID int64) (bool, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(id) FROM question_follow WHERE user_id = ? AND question_id = ?", userID, questionID)
	return count > 0, err
}

// ---------- 评论 ----------

// CreateComment 创建评论并同步累加目标计数（同一事务）
func CreateComment(cm *models.Comment, targetAuthorID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(
		"INSERT INTO comment (comment_id, user_id, target_type, target_id, parent_id, content) VALUES (?, ?, ?, ?, ?, ?)",
		cm.CommentID, cm.UserID, cm.TargetType, cm.TargetID, cm.ParentID, cm.Content); err != nil {
		return err
	}
	switch cm.TargetType {
	case models.TargetAnswer:
		if _, err = tx.Exec("UPDATE answer SET comment_count = comment_count + 1 WHERE answer_id = ?", cm.TargetID); err != nil {
			return err
		}
	case models.TargetQuestion:
		if _, err = tx.Exec("UPDATE question SET comment_count = comment_count + 1 WHERE question_id = ?", cm.TargetID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CommentPageParams 评论分页查询参数（cursor）
type CommentPageParams struct {
	TargetType int32
	TargetID   int64
	CursorTime sql.NullTime
	CursorID   int64
	Limit      int
}

// ListComments 按 cursor 分页查询目标的评论
func ListComments(p *CommentPageParams) (comments []*models.Comment, err error) {
	comments = make([]*models.Comment, 0)
	limit := p.Limit + 1
	var query string
	var args []interface{}
	if p.CursorID == 0 {
		query = `SELECT comment_id, user_id, target_type, target_id, parent_id, content, status, create_time
			 FROM comment WHERE target_type = ? AND target_id = ? AND status = 1
			 ORDER BY create_time DESC, comment_id DESC LIMIT ?`
		args = []interface{}{p.TargetType, p.TargetID, limit}
	} else {
		query = `SELECT comment_id, user_id, target_type, target_id, parent_id, content, status, create_time
			 FROM comment WHERE target_type = ? AND target_id = ? AND status = 1
			 AND (create_time < ? OR (create_time = ? AND comment_id < ?))
			 ORDER BY create_time DESC, comment_id DESC LIMIT ?`
		args = []interface{}{p.TargetType, p.TargetID, p.CursorTime.Time, p.CursorTime.Time, p.CursorID, limit}
	}
	err = db.Select(&comments, query, args...)
	if err != nil {
		zap.L().Error("query comments failed", zap.Error(err))
		return nil, err
	}
	return comments, nil
}

// GetCommentByID 查询单条评论（回复时校验父评论）
func GetCommentByID(id int64) (*models.Comment, error) {
	cm := new(models.Comment)
	err := db.Get(cm, "SELECT comment_id, user_id, target_type, target_id, parent_id, content, status, create_time FROM comment WHERE comment_id = ?", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCommentNotExist
		}
		return nil, err
	}
	return cm, nil
}

// ---------- 通知 ----------

// CreateNotification 创建通知（自己给自己的动作不通知）
func CreateNotification(n *models.Notification) error {
	if n.UserID == n.ActorID {
		return nil
	}
	_, err := db.Exec(
		"INSERT INTO notification (notification_id, user_id, actor_id, type, target_type, target_id) VALUES (?, ?, ?, ?, ?, ?)",
		n.NotificationID, n.UserID, n.ActorID, n.Type, n.TargetType, n.TargetID)
	return err
}

// ListNotifications 按 cursor 分页查询用户通知
func ListNotifications(userID int64, cursorTime sql.NullTime, cursorID int64, limit int) (ns []*models.Notification, err error) {
	ns = make([]*models.Notification, 0)
	q := limit + 1
	var query string
	var args []interface{}
	if cursorID == 0 {
		query = `SELECT notification_id, user_id, actor_id, type, target_type, target_id, is_read, create_time
			 FROM notification WHERE user_id = ?
			 ORDER BY create_time DESC, notification_id DESC LIMIT ?`
		args = []interface{}{userID, q}
	} else {
		query = `SELECT notification_id, user_id, actor_id, type, target_type, target_id, is_read, create_time
			 FROM notification WHERE user_id = ?
			 AND (create_time < ? OR (create_time = ? AND notification_id < ?))
			 ORDER BY create_time DESC, notification_id DESC LIMIT ?`
		args = []interface{}{userID, cursorTime.Time, cursorTime.Time, cursorID, q}
	}
	err = db.Select(&ns, query, args...)
	return
}

// CountUnreadNotifications 查询未读通知数
func CountUnreadNotifications(userID int64) (int, error) {
	var n int
	err := db.Get(&n, "SELECT COUNT(notification_id) FROM notification WHERE user_id = ? AND is_read = 0", userID)
	return n, err
}

// MarkAllNotificationsRead 全部标记已读
func MarkAllNotificationsRead(userID int64) error {
	_, err := db.Exec("UPDATE notification SET is_read = 1 WHERE user_id = ? AND is_read = 0", userID)
	return err
}

// GetUsersByIDs 批量查询用户（Feed/评论组装用）
func GetUsersByIDs(ids []int64) (users []*models.User, err error) {
	users = make([]*models.User, 0, len(ids))
	if len(ids) == 0 {
		return users, nil
	}
	query, args, err := sqlx.In(`SELECT user_id, username, email, email_verified, avatar_url, bio, status
		FROM user WHERE user_id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	err = db.Select(&users, query, args...)
	return
}
