package mysql

import (
	"database/sql"
	"errors"
	"time"

	"bluebell/models"
)

// ---------- 申诉 ----------

var ErrAppealConflict = errors.New("该内容已有进行中的申诉")
var ErrAppealNotExist = errors.New("申诉不存在")

// CreateAppeal 创建申诉（同作者同目标同决定版本最多一个待处理）
func CreateAppeal(a *models.ModerationAppeal) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 待处理查重：同作者同目标同决定版本已有 pending 则冲突
	var n int
	if err = tx.Get(&n,
		"SELECT COUNT(appeal_id) FROM moderation_appeal WHERE author_id = ? AND target_type = ? AND target_id = ? AND decision_version = ? AND status = 0",
		a.AuthorID, a.TargetType, a.TargetID, a.DecisionVersion); err != nil {
		return err
	}
	if n > 0 {
		return ErrAppealConflict
	}
	if _, err = tx.Exec(
		"INSERT INTO moderation_appeal (appeal_id, author_id, target_type, target_id, decision_version, reason) VALUES (?, ?, ?, ?, ?, ?)",
		a.AppealID, a.AuthorID, a.TargetType, a.TargetID, a.DecisionVersion, a.Reason); err != nil {
		return err
	}
	return tx.Commit()
}

// ListAppeals 审核端申诉队列
func ListAppeals(status int32, cursorID int64, limit int) (appeals []*models.ModerationAppeal, err error) {
	appeals = make([]*models.ModerationAppeal, 0)
	q := limit + 1
	where := "WHERE status = ?"
	args := []interface{}{status}
	if cursorID > 0 {
		where += " AND appeal_id < ?"
		args = append(args, cursorID)
	}
	args = append(args, q)
	err = db.Select(&appeals,
		"SELECT appeal_id, author_id, target_type, target_id, decision_version, reason, status, reviewer_id, version, create_time, reviewed_at FROM moderation_appeal "+
			where+" ORDER BY appeal_id DESC LIMIT ?", args...)
	return
}

// ResolveAppeal 处理申诉（乐观锁：version 匹配才更新）
func ResolveAppeal(appealID, reviewerID int64, newStatus int32, note string, expectVersion int64) (bool, error) {
	tx, err := db.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(
		"UPDATE moderation_appeal SET status = ?, reviewer_id = ?, version = version + 1, reviewed_at = NOW() "+
			"WHERE appeal_id = ? AND status = 0 AND version = ?",
		newStatus, reviewerID, appealID, expectVersion)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return false, nil
	}
	return true, tx.Commit()
}

// GetAppeal 查询申诉
func GetAppeal(appealID int64) (*models.ModerationAppeal, error) {
	a := new(models.ModerationAppeal)
	err := db.Get(a, "SELECT appeal_id, author_id, target_type, target_id, decision_version, reason, status, reviewer_id, version, create_time, reviewed_at FROM moderation_appeal WHERE appeal_id = ?", appealID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAppealNotExist
		}
		return nil, err
	}
	return a, nil
}

// GetPendingAppeal 查询目标的进行中申诉
func GetPendingAppeal(targetType int32, targetID int64) (*models.ModerationAppeal, error) {
	a := new(models.ModerationAppeal)
	err := db.Get(a, "SELECT appeal_id, author_id, target_type, target_id, decision_version, reason, status, reviewer_id, version, create_time, reviewed_at FROM moderation_appeal WHERE target_type = ? AND target_id = ? AND status = 0", targetType, targetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAppealNotExist
		}
		return nil, err
	}
	return a, nil
}

// ---------- 通知单条已读 ----------

// MarkNotificationRead 单条已读（校验接收者，幂等）
func MarkNotificationRead(notificationID, userID int64) (bool, error) {
	result, err := db.Exec(
		"UPDATE notification SET is_read = 1 WHERE notification_id = ? AND user_id = ? AND is_read = 0",
		notificationID, userID)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// ---------- 创作者中心 ----------

// CreatorOverview 创作者概览统计
type CreatorOverview struct {
	Questions   int64 `json:"questions"`
	Answers     int64 `json:"answers"`
	Articles    int64 `json:"articles"`
	Likes       int64 `json:"likes_received"`
	Comments    int64 `json:"comments_received"`
	Followers   int64 `json:"followers"`
	Views       int64 `json:"views"`
	AsOf        string `json:"as_of"`
}

// GetCreatorOverview 创作者概览（本人数据）
func GetCreatorOverview(userID int64) (*CreatorOverview, error) {
	o := &CreatorOverview{}
	if err := db.Get(&o.Questions, "SELECT COUNT(*) FROM question WHERE author_id = ? AND status = 1", userID); err != nil {
		return nil, err
	}
	if err := db.Get(&o.Answers, "SELECT COUNT(*) FROM answer WHERE author_id = ? AND status = 1", userID); err != nil {
		return nil, err
	}
	if err := db.Get(&o.Articles, "SELECT COUNT(*) FROM article WHERE author_id = ? AND status = 1 AND deleted_at IS NULL", userID); err != nil {
		return nil, err
	}
	// 收到的赞同：answer_vote JOIN answer（作者）
	if err := db.Get(&o.Likes, "SELECT COUNT(*) FROM answer_vote av JOIN answer a ON a.answer_id = av.answer_id WHERE a.author_id = ? AND av.direction = 1", userID); err != nil {
		return nil, err
	}
	// 收到的评论：回答评论数之和 + 问题评论数之和
	if err := db.Get(&o.Comments,
		"SELECT COALESCE((SELECT SUM(comment_count) FROM answer WHERE author_id = ? AND status = 1), 0) + "+
			"COALESCE((SELECT SUM(comment_count) FROM question WHERE author_id = ? AND status = 1), 0)", userID, userID); err != nil {
		return nil, err
	}
	if err := db.Get(&o.Followers, "SELECT COUNT(*) FROM user_follow WHERE following_id = ?", userID); err != nil {
		return nil, err
	}
	// 文章阅读量
	if err := db.Get(&o.Views, "SELECT COALESCE(SUM(view_count), 0) FROM article WHERE author_id = ? AND status = 1", userID); err != nil {
		return nil, err
	}
	return o, nil
}

// ListCreatorContents 创作者内容列表（type: question/answer/article；status 过滤）
func ListCreatorContents(userID int64, contentType string, status int32, limit int) (rows []map[string]interface{}, err error) {
	rows = make([]map[string]interface{}, 0, limit)
	type itemRow struct {
		ID         int64  `db:"id"`
		Title      string `db:"title"`
		ModStatus  int32  `db:"moderation_status"`
		Status     int32  `db:"status"`
		UpdateTime string `db:"update_time"`
	}
	var items []itemRow
	switch contentType {
	case "question":
		err = db.Select(&items, "SELECT question_id AS id, title, moderation_status, status, update_time FROM question WHERE author_id = ? AND status = ? ORDER BY create_time DESC LIMIT ?",
			userID, status, limit)
	case "answer":
		err = db.Select(&items, "SELECT answer_id AS id, LEFT(content, 100) AS title, moderation_status, status, update_time FROM answer WHERE author_id = ? AND status = ? ORDER BY create_time DESC LIMIT ?",
			userID, status, limit)
	case "article":
		err = db.Select(&items, "SELECT article_id AS id, title, moderation_status, status, update_time FROM article WHERE author_id = ? AND status = ? AND deleted_at IS NULL ORDER BY create_time DESC LIMIT ?",
			userID, status, limit)
	default:
		return nil, ErrInvalidParams
	}
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		rows = append(rows, map[string]interface{}{
			"id": it.ID, "title": it.Title, "moderation_status": it.ModStatus,
			"status": it.Status, "update_time": it.UpdateTime,
		})
	}
	return rows, nil
}

// ContentDailyMetrics 内容逐日事件聚合（recommendation_event）
func ContentDailyMetrics(contentType string, contentID int64, days int) (rows []map[string]interface{}, asOf string, err error) {
	rows = make([]map[string]interface{}, 0)
	err = db.Select(&rows, `SELECT DATE(occurred_at) AS date, event_type, COUNT(*) AS cnt
		FROM recommendation_event WHERE item_type = ? AND item_id = ? AND occurred_at > NOW() - INTERVAL ? DAY
		GROUP BY DATE(occurred_at), event_type ORDER BY date DESC`, contentType, contentID, days)
	asOf = time.Now().UTC().Format(time.RFC3339)
	return
}
