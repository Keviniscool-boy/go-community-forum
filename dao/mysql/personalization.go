package mysql

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"bluebell/models"

)

// ---------- 专栏关注 ----------

// FollowColumn 关注专栏（幂等；首次关注累加计数）
func FollowColumn(userID, columnID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec("INSERT IGNORE INTO column_follow (user_id, column_id) VALUES (?, ?)", userID, columnID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		if _, err = tx.Exec("UPDATE column_info SET follower_count = follower_count + 1 WHERE column_id = ?", columnID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UnfollowColumn 取消关注（计数以关系表为准重算，防止漂移/负数）
func UnfollowColumn(userID, columnID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM column_follow WHERE user_id = ? AND column_id = ?", userID, columnID); err != nil {
		return err
	}
	if _, err = tx.Exec(
		"UPDATE column_info SET follower_count = (SELECT COUNT(*) FROM column_follow WHERE column_id = ?) WHERE column_id = ?",
		columnID, columnID); err != nil {
		return err
	}
	return tx.Commit()
}

// IsColumnFollowed 查询关注状态
func IsColumnFollowed(userID, columnID int64) (bool, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(id) FROM column_follow WHERE user_id = ? AND column_id = ?", userID, columnID)
	return count > 0, err
}

// FollowedColumnIDs 用户关注的专栏 id 列表
func FollowedColumnIDs(userID int64) (ids []int64, err error) {
	ids = make([]int64, 0)
	err = db.Select(&ids, "SELECT column_id FROM column_follow WHERE user_id = ?", userID)
	return
}

// ---------- 兴趣 ----------

// GetUserInterests 用户显式兴趣话题
func GetUserInterests(userID int64) (topicIDs []int64, err error) {
	topicIDs = make([]int64, 0)
	err = db.Select(&topicIDs, "SELECT topic_id FROM user_topic_interest WHERE user_id = ?", userID)
	return
}

// ReplaceUserInterests 全量替换兴趣（事务；上限 20 由调用方校验）
func ReplaceUserInterests(userID int64, topicIDs []int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM user_topic_interest WHERE user_id = ?", userID); err != nil {
		return err
	}
	for _, id := range topicIDs {
		if _, err = tx.Exec("INSERT IGNORE INTO user_topic_interest (user_id, topic_id) VALUES (?, ?)", userID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- 推荐偏好 ----------

// GetRecommendationPreference 读取偏好（无记录返回默认开启）
func GetRecommendationPreference(userID int64) (enabled bool, version int64, err error) {
	var row struct {
		Enabled bool   `db:"personalization_enabled"`
		Version int64  `db:"version"`
	}
	err = db.Get(&row, "SELECT personalization_enabled, version FROM recommendation_preference WHERE user_id = ?", userID)
	if errors.Is(err, sql.ErrNoRows) {
		return true, 0, nil // 默认开启个性化；version 0 表示尚无记录
	}
	if err != nil {
		return false, 0, err
	}
	return row.Enabled, row.Version, nil
}

// SetRecommendationPreference 更新偏好（版本乐观锁；首条记录 version 从 1 开始）
func SetRecommendationPreference(userID int64, enabled bool, expectVersion int64) (newVersion int64, err error) {
	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var cur int64
	err = tx.Get(&cur, "SELECT version FROM recommendation_preference WHERE user_id = ?", userID)
	if errors.Is(err, sql.ErrNoRows) {
		if expectVersion != 0 {
			return 0, ErrPreferenceConflict
		}
		if _, err = tx.Exec(
			"INSERT INTO recommendation_preference (user_id, personalization_enabled, version) VALUES (?, ?, 1)",
			userID, enabled); err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	} else {
		// B05 真正的 CAS：条件更新带版本，受影响行 0 即并发冲突（避免读改写竞态）
		result, err := tx.Exec(
			"UPDATE recommendation_preference SET personalization_enabled = ?, version = version + 1 WHERE user_id = ? AND version = ?",
			enabled, userID, expectVersion)
		if err != nil {
			return 0, err
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return 0, ErrPreferenceConflict
		}
	}
	if err = tx.Get(&newVersion, "SELECT version FROM recommendation_preference WHERE user_id = ?", userID); err != nil {
		return 0, err
	}
	return newVersion, tx.Commit()
}

// ---------- 负反馈 ----------

// AddRecommendationFeedback 记录负反馈（幂等）
func AddRecommendationFeedback(userID int64, kind, targetType string, targetID int64) error {
	_, err := db.Exec(
		"INSERT IGNORE INTO recommendation_feedback (user_id, kind, target_type, target_id) VALUES (?, ?, ?, ?)",
		userID, kind, targetType, targetID)
	return err
}

// RemoveRecommendationFeedback 撤销负反馈
func RemoveRecommendationFeedback(userID int64, kind, targetType string, targetID int64) error {
	_, err := db.Exec(
		"DELETE FROM recommendation_feedback WHERE user_id = ? AND kind = ? AND target_type = ? AND target_id = ?",
		userID, kind, targetType, targetID)
	return err
}

// ListRecommendationFeedback 用户负反馈列表
func ListRecommendationFeedback(userID int64, limit int) (rows []map[string]interface{}, err error) {
	type fbRow struct {
		Kind       string    `db:"kind"`
		TargetType string    `db:"target_type"`
		TargetID   int64     `db:"target_id"`
		CreateTime time.Time `db:"create_time"`
	}
	rowsF := make([]fbRow, 0, limit)
	if err = db.Select(&rowsF, "SELECT kind, target_type, target_id, create_time FROM recommendation_feedback WHERE user_id = ? ORDER BY create_time DESC LIMIT ?",
		userID, limit); err != nil {
		return nil, err
	}
	for _, r := range rowsF {
		rows = append(rows, map[string]interface{}{
			"kind": r.Kind, "target_type": r.TargetType,
			"target_id": fmt.Sprintf("%d", r.TargetID), "create_time": r.CreateTime,
		})
	}
	return rows, nil
}

// ---------- 屏蔽 ----------

// BlockUser 屏蔽作者（幂等；禁止自屏蔽）
func BlockUser(blockerID, blockedID int64) error {
	if blockerID == blockedID {
		return ErrCannotBlockSelf
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT IGNORE INTO user_block (blocker_id, blocked_id) VALUES (?, ?)", blockerID, blockedID); err != nil {
		return err
	}
	// 双方建立屏蔽：两人之间的待处理回答邀请立即失效（PRD §9，同事务）
	if _, err = tx.Exec(
		"UPDATE question_answer_invitation SET status = 5 WHERE status = 0 AND "+
			"((inviter_id = ? AND invitee_id = ?) OR (inviter_id = ? AND invitee_id = ?))",
		blockerID, blockedID, blockedID, blockerID); err != nil {
		return err
	}
	return tx.Commit()
}

// UnblockUser 取消屏蔽
func UnblockUser(blockerID, blockedID int64) error {
	_, err := db.Exec("DELETE FROM user_block WHERE blocker_id = ? AND blocked_id = ?", blockerID, blockedID)
	return err
}

// IsBlocked 查询屏蔽关系（A 是否屏蔽了 B）
func IsBlocked(blockerID, blockedID int64) (bool, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(id) FROM user_block WHERE blocker_id = ? AND blocked_id = ?", blockerID, blockedID)
	return count > 0, err
}

// BlockedAuthorIDs 用户屏蔽的作者 id 列表（Feed/推荐过滤用）
func BlockedAuthorIDs(userID int64) (ids []int64, err error) {
	ids = make([]int64, 0)
	err = db.Select(&ids, "SELECT blocked_id FROM user_block WHERE blocker_id = ?", userID)
	return
}

// ListBlockedUsers 用户屏蔽列表
func ListBlockedUsers(userID int64, limit int) (users []*models.User, err error) {
	users = make([]*models.User, 0, limit)
	err = db.Select(&users, `SELECT u.user_id, u.username, u.avatar_url FROM user_block b
		JOIN user u ON u.user_id = b.blocked_id WHERE b.blocker_id = ? LIMIT ?`, userID, limit)
	return
}

var (
	ErrPreferenceConflict = errors.New("设置版本冲突，请刷新后重试")
	ErrCannotBlockSelf    = errors.New("不能屏蔽自己")
)

// FollowedTopicIDs 用户关注的话题 id 列表
func FollowedTopicIDs(userID int64) (ids []int64, err error) {
	ids = make([]int64, 0)
	err = db.Select(&ids, "SELECT topic_id FROM topic_follow WHERE user_id = ?", userID)
	return
}

// FeedbackItem 全量负反馈（过滤用）
type FeedbackItem struct {
	Kind       string
	TargetType string
	TargetID   int64
}

// ListAllRecommendationFeedback 用户全部负反馈（Feed 过滤用）
func ListAllRecommendationFeedback(userID int64) (items []FeedbackItem, err error) {
	items = make([]FeedbackItem, 0)
	err = db.Select(&items, "SELECT kind, target_type, target_id FROM recommendation_feedback WHERE user_id = ?", userID)
	return
}
