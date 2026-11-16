package mysql

import (
	"database/sql"
	"errors"
	"time"

	"bluebell/models"
	"bluebell/pkg/snowflake"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// 死锁重试：并发互动同一内容时，不同事务的锁顺序可能交叉（MySQL 1213/1205）。
// 高并发场景统一在事务开头 FOR UPDATE 目标内容行，再辅以有限重试。
const deadlockRetryMax = 3

func isDeadlock(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && (mysqlErr.Number == 1213 || mysqlErr.Number == 1205)
}

// withDeadlockRetry 对死锁错误做有限次重试，其他错误原样返回
func withDeadlockRetry(fn func() error) error {
	var err error
	for i := 0; i < deadlockRetryMax; i++ {
		if err = fn(); err == nil || !isDeadlock(err) {
			return err
		}
		time.Sleep(time.Duration(10*(i+1)) * time.Millisecond)
	}
	return err
}

var (
	ErrCannotFollowSelf       = errors.New("不能关注自己")
	ErrFollowRelationNotExist = errors.New("关注关系不存在")
	ErrCommentNotExist        = errors.New("评论不存在")
	ErrInvalidParams          = errors.New("参数不合法")
)

// FollowUser 关注用户（幂等，重复关注无副作用；首次关注在同一事务写通知）
func FollowUser(followerID, followingID int64) error {
	if followerID == followingID {
		return ErrCannotFollowSelf
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("INSERT IGNORE INTO user_follow (follower_id, following_id) VALUES (?, ?)", followerID, followingID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		if err := txCreateNotification(tx, followingID, followerID, models.NotifyTypeFollow, 0, followerID); err != nil {
			return err
		}
	}
	return tx.Commit()
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

// VoteAnswer 对回答投票：upsert 方向，幂等；投票、计数与首次赞同的通知在同一事务
// 并发安全：INSERT ... ON DUPLICATE KEY UPDATE 保证同一 (user, answer) 只有一行；
// 计数不走增量（并发增量会漂移），而是在事务内以关系表为准原子重算
func VoteAnswer(userID, answerID int64, direction int8) error {
	if direction != 1 && direction != -1 && direction != 0 {
		return ErrInvalidParams
	}
	return withDeadlockRetry(func() error {
		return voteAnswerOnce(userID, answerID, direction)
	})
}

func voteAnswerOnce(userID, answerID int64, direction int8) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 事务内取回答作者并锁定该行（统一锁顺序：先内容行，后关系表）
	var answerAuthor int64
	if err = tx.Get(&answerAuthor, "SELECT author_id FROM answer WHERE answer_id = ? FOR UPDATE", answerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAnswerNotExist
		}
		return err
	}

	// 读取旧方向（仅用于"首次赞同"判定，计数不依赖它）
	var old int8
	err = tx.Get(&old, "SELECT direction FROM answer_vote WHERE user_id = ? AND answer_id = ?", userID, answerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	hasOld := !errors.Is(err, sql.ErrNoRows)

	if direction == 0 {
		// 取消投票：删除记录
		if _, err = tx.Exec("DELETE FROM answer_vote WHERE user_id = ? AND answer_id = ?", userID, answerID); err != nil {
			return err
		}
	} else {
		// 原子 upsert：并发提交同一 (user, answer) 不会撞唯一索引
		if _, err = tx.Exec(
			"INSERT INTO answer_vote (user_id, answer_id, direction) VALUES (?, ?, ?) "+
				"ON DUPLICATE KEY UPDATE direction = VALUES(direction)",
			userID, answerID, direction); err != nil {
			return err
		}
	}
	// 以关系表为准重算票数（原子，杜绝增量漂移）
	if _, err = tx.Exec(
		"UPDATE answer SET vote_count = COALESCE((SELECT SUM(CASE WHEN direction = 1 THEN 1 WHEN direction = -1 THEN -1 ELSE 0 END) "+
			"FROM answer_vote WHERE answer_id = ?), 0) WHERE answer_id = ?", answerID, answerID); err != nil {
		return err
	}
	// 首次赞同才通知（取消/重复赞同/改反对不通知）
	if direction == 1 && (!hasOld || old != 1) {
		if err := txCreateNotification(tx, answerAuthor, userID, models.NotifyTypeLike, models.TargetAnswer, answerID); err != nil {
			return err
		}
	}
	return tx.Commit()
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

// CollectAnswer 收藏回答（幂等；收藏、计数与通知在同一事务）
func CollectAnswer(userID, answerID int64) error {
	return withDeadlockRetry(func() error { return collectAnswerOnce(userID, answerID) })
}

func collectAnswerOnce(userID, answerID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var answerAuthor int64
	if err = tx.Get(&answerAuthor, "SELECT author_id FROM answer WHERE answer_id = ? FOR UPDATE", answerID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAnswerNotExist
		}
		return err
	}
	result, err := tx.Exec("INSERT IGNORE INTO answer_collect (user_id, answer_id) VALUES (?, ?)", userID, answerID)
	if err != nil {
		return err
	}
	// 计数与通知只在首次收藏时写入；计数以关系表为准重算
	if n, _ := result.RowsAffected(); n > 0 {
		if err := recalcCollectCount(tx, answerID); err != nil {
			return err
		}
		if err := txCreateNotification(tx, answerAuthor, userID, models.NotifyTypeCollect, models.TargetAnswer, answerID); err != nil {
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
		if err := recalcCollectCount(tx, answerID); err != nil {
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

// FollowQuestion 关注问题（幂等；关注、计数与通知在同一事务）
func FollowQuestion(userID, questionID int64) error {
	return withDeadlockRetry(func() error { return followQuestionOnce(userID, questionID) })
}

func followQuestionOnce(userID, questionID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var qAuthor int64
	if err = tx.Get(&qAuthor, "SELECT author_id FROM question WHERE question_id = ? FOR UPDATE", questionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrQuestionNotExist
		}
		return err
	}
	result, err := tx.Exec("INSERT IGNORE INTO question_follow (user_id, question_id) VALUES (?, ?)", userID, questionID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		if err := recalcFollowerCount(tx, questionID); err != nil {
			return err
		}
		if err := txCreateNotification(tx, qAuthor, userID, models.NotifyTypeFollow, models.TargetQuestion, questionID); err != nil {
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
		if err := recalcFollowerCount(tx, questionID); err != nil {
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

// CreateComment 创建评论：评论、目标计数与通知在同一事务
func CreateComment(cm *models.Comment) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 事务内取目标作者（通知用）
	var targetAuthor int64
	switch cm.TargetType {
	case models.TargetAnswer:
		if err = tx.Get(&targetAuthor, "SELECT author_id FROM answer WHERE answer_id = ? FOR UPDATE", cm.TargetID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAnswerNotExist
			}
			return err
		}
	case models.TargetQuestion:
		if err = tx.Get(&targetAuthor, "SELECT author_id FROM question WHERE question_id = ? FOR UPDATE", cm.TargetID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrQuestionNotExist
			}
			return err
		}
	case models.TargetArticle:
		if err = tx.Get(&targetAuthor, "SELECT author_id FROM article WHERE article_id = ? FOR UPDATE", cm.TargetID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrArticleNotExist
			}
			return err
		}
	default:
		return ErrInvalidParams
	}
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
	case models.TargetArticle:
		// 文章表无 comment_count 冗余列，计数按关系表查询；通知照发
	}
	if err := txCreateNotification(tx, targetAuthor, cm.UserID, models.NotifyTypeComment, cm.TargetType, cm.TargetID); err != nil {
		return err
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
			 AND moderation_status = 1 AND deleted_at IS NULL
			 ORDER BY create_time DESC, comment_id DESC LIMIT ?`
		args = []interface{}{p.TargetType, p.TargetID, limit}
	} else {
		query = `SELECT comment_id, user_id, target_type, target_id, parent_id, content, status, create_time
			 FROM comment WHERE target_type = ? AND target_id = ? AND status = 1
			 AND moderation_status = 1 AND deleted_at IS NULL
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

// txCreateNotification 在互动事务内写入通知（自己给自己的动作不通知）
// 通知与互动同事务：要么都成功要么都回滚，不允许先返回成功再静默丢通知
func txCreateNotification(tx *sqlx.Tx, userID, actorID int64, notifyType int32, targetType int32, targetID int64) error {
	if userID == actorID {
		return nil
	}
	_, err := tx.Exec(
		"INSERT INTO notification (notification_id, user_id, actor_id, type, target_type, target_id) VALUES (?, ?, ?, ?, ?, ?)",
		snowflake.GenerateID(), userID, actorID, notifyType, targetType, targetID)
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

// recalcCollectCount 以关系表为准重算回答收藏数（事务内，杜绝增量漂移/负数）
func recalcCollectCount(tx *sqlx.Tx, answerID int64) error {
	_, err := tx.Exec(
		"UPDATE answer SET collect_count = (SELECT COUNT(*) FROM answer_collect WHERE answer_id = ?) WHERE answer_id = ?",
		answerID, answerID)
	return err
}

// recalcFollowerCount 以关系表为准重算问题关注数（事务内）
func recalcFollowerCount(tx *sqlx.Tx, questionID int64) error {
	_, err := tx.Exec(
		"UPDATE question SET follower_count = (SELECT COUNT(*) FROM question_follow WHERE question_id = ?) WHERE question_id = ?",
		questionID, questionID)
	return err
}
