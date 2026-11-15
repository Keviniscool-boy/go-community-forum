package mysql

import (
	"database/sql"
	"errors"

	"bluebell/models"
	"bluebell/pkg/snowflake"

	"go.uber.org/zap"
)

var ErrReportNotExist = errors.New("举报不存在")

// CreateReport 创建举报（同用户同目标唯一，重复提交幂等返回已有记录）
func CreateReport(r *models.Report) (dup bool, err error) {
	result, err := db.Exec(
		"INSERT IGNORE INTO report (report_id, reporter_id, target_type, target_id, reason_code, description) VALUES (?, ?, ?, ?, ?, ?)",
		r.ReportID, r.ReporterID, r.TargetType, r.TargetID, r.ReasonCode, r.Description)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return true, nil
	}
	return false, nil
}

// ReportPageParams 举报队列分页参数
type ReportPageParams struct {
	Status    *int32 // nil 表示全部
	CursorID  int64
	Limit     int
}

// ListReports 审核队列（按创建时间倒序 cursor 分页）
func ListReports(p *ReportPageParams) (reports []*models.Report, err error) {
	reports = make([]*models.Report, 0)
	limit := p.Limit + 1
	where := "WHERE 1 = 1"
	args := make([]interface{}, 0, 4)
	if p.Status != nil {
		where += " AND status = ?"
		args = append(args, *p.Status)
	}
	if p.CursorID > 0 {
		where += " AND report_id < ?"
		args = append(args, p.CursorID)
	}
	query := "SELECT report_id, reporter_id, target_type, target_id, reason_code, description, status, reviewer_id, review_note, created_at, reviewed_at FROM report " +
		where + " ORDER BY report_id DESC LIMIT ?"
	args = append(args, limit)
	err = db.Select(&reports, query, args...)
	return
}

// GetReportByID 查询单条举报
func GetReportByID(reportID int64) (*models.Report, error) {
	r := new(models.Report)
	err := db.Get(r, "SELECT report_id, reporter_id, target_type, target_id, reason_code, description, status, reviewer_id, review_note, created_at, reviewed_at FROM report WHERE report_id = ?", reportID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrReportNotExist
		}
		return nil, err
	}
	return r, nil
}

// ResolveReport 审核处理举报：乐观锁（status 从 pending 变更，防两管理员并发覆盖）
func ResolveReport(reportID, reviewerID int64, status int32, note string) (bool, error) {
	result, err := db.Exec(
		"UPDATE report SET status = ?, reviewer_id = ?, review_note = ?, reviewed_at = NOW() WHERE report_id = ? AND status = ?",
		status, reviewerID, note, reportID, models.ReportStatusPending)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// SetModeration 管理员设置内容审核状态（question/answer/comment）
// targetType: 1 question 2 answer 3 comment
func SetModeration(targetType int32, targetID int64, moderationStatus int32, operatorID int64) error {
	var query string
	switch targetType {
	case models.ReportTargetQuestion:
		query = "UPDATE question SET moderation_status = ?, updated_by = ? WHERE question_id = ?"
	case models.ReportTargetAnswer:
		query = "UPDATE answer SET moderation_status = ?, updated_by = ? WHERE answer_id = ?"
	case models.ReportTargetComment:
		query = "UPDATE comment SET moderation_status = ?, updated_by = ? WHERE comment_id = ?"
	default:
		return ErrInvalidParams
	}
	_, err := db.Exec(query, moderationStatus, operatorID, targetID)
	return err
}

// SetVisibility 管理员设置可见性
func SetVisibility(targetType int32, targetID int64, visibility int32, operatorID int64) error {
	var query string
	switch targetType {
	case models.ReportTargetQuestion:
		query = "UPDATE question SET visibility = ?, updated_by = ? WHERE question_id = ?"
	case models.ReportTargetAnswer:
		query = "UPDATE answer SET visibility = ?, updated_by = ? WHERE answer_id = ?"
	default:
		// comment 无 visibility 列，通过 moderation_status 控制隐藏
		return ErrInvalidParams
	}
	_, err := db.Exec(query, visibility, operatorID, targetID)
	return err
}

// SetUserStatus 管理员禁用/启用用户
func SetUserStatus(userID int64, status int32) error {
	_, err := db.Exec("UPDATE user SET status = ? WHERE user_id = ?", status, userID)
	return err
}

// ListUsersByRole 查询某角色用户列表
func ListUsersByRole(role string) (users []*models.User, err error) {
	users = make([]*models.User, 0)
	err = db.Select(&users, "SELECT user_id, username, email, status, role FROM user WHERE role = ?", role)
	return
}

// SetUserRole 设置用户角色（管理员初始账号流程使用）
func SetUserRole(userID int64, role string) error {
	_, err := db.Exec("UPDATE user SET role = ? WHERE user_id = ?", role, userID)
	return err
}

// CreateAuditLog 追加审计日志（metadata 为合法 JSON 字符串）
func CreateAuditLog(a *models.AuditLog) error {
	_, err := db.Exec(
		"INSERT INTO audit_log (audit_id, actor_id, action, target_type, target_id, request_id, ip_hash, metadata_json) VALUES (?, ?, ?, ?, ?, ?, ?, CAST(? AS JSON))",
		a.AuditID, a.ActorID, a.Action, a.TargetType, a.TargetID, a.RequestID, a.IPHash, a.Metadata)
	return err
}

// AuditLogPageParams 审计日志分页
type AuditLogPageParams struct {
	ActorID  int64 // 0 = 全部
	CursorID int64
	Limit    int
}

// ListAuditLogs 审计日志查询（只追加，不提供删除）
func ListAuditLogs(p *AuditLogPageParams) (logs []*models.AuditLog, err error) {
	logs = make([]*models.AuditLog, 0)
	limit := p.Limit + 1
	where := "WHERE 1 = 1"
	args := make([]interface{}, 0, 4)
	if p.ActorID > 0 {
		where += " AND actor_id = ?"
		args = append(args, p.ActorID)
	}
	if p.CursorID > 0 {
		where += " AND audit_id < ?"
		args = append(args, p.CursorID)
	}
	query := "SELECT audit_id, actor_id, action, target_type, target_id, request_id, ip_hash, metadata_json, created_at FROM audit_log " +
		where + " ORDER BY audit_id DESC LIMIT ?"
	args = append(args, limit)
	err = db.Select(&logs, query, args...)
	return
}

// logAdminAction 记录管理员动作审计（失败仅记录日志，不阻塞业务）
func logAdminAction(actorID int64, action, targetType, targetID, requestID string, metadata string) {
	a := &models.AuditLog{
		AuditID:    snowflake.GenerateID(),
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		RequestID:  requestID,
		Metadata:   metadata,
	}
	if err := CreateAuditLog(a); err != nil {
		zap.L().Error("create audit log failed", zap.Error(err), zap.String("action", action))
	}
}
