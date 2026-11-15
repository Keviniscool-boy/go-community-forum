package logic

import (
	"encoding/json"
	"strconv"
	"time"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/snowflake"
	"go.uber.org/zap"
)

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
	if err := mysql.CreateAuditLog(a); err != nil {
		zap.L().Error("create audit log failed", zap.Error(err), zap.String("action", action))
	}
}

// ErrAdminConflict 乐观锁冲突：两名管理员并发审核同一目标
type adminConflictErr struct{}

func (adminConflictErr) Error() string { return "该记录已被其他管理员处理" }

var ErrAdminConflict error = adminConflictErr{}

// ListReportQueue 审核队列
func ListReportQueue(status *int32, cursor string, limit int) (*models.PageResult[*models.ApiReportDetail], error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	var cursorID int64
	if cursor != "" {
		c, err := parseTimeCursor(cursor)
		if err != nil || c.ID < 0 {
			return nil, ErrCursorInvalid
		}
		cursorID = c.ID
	}
	reports, err := mysql.ListReports(&mysql.ReportPageParams{Status: status, CursorID: cursorID, Limit: limit})
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(reports) > limit {
		hasMore = true
		reports = reports[:limit]
	}
	// 批量装配举报者用户名
	ids := make([]int64, 0, len(reports))
	for _, r := range reports {
		ids = append(ids, r.ReporterID)
	}
	users, err := mysql.GetUsersByIDs(ids)
	if err != nil {
		return nil, err
	}
	nameMap := make(map[int64]string, len(users))
	for _, u := range users {
		nameMap[u.UserID] = u.Username
	}
	items := make([]*models.ApiReportDetail, 0, len(reports))
	var last *models.Report
	for _, r := range reports {
		last = r
		items = append(items, &models.ApiReportDetail{Report: r, ReporterName: nameMap[r.ReporterID]})
	}
	next := ""
	if hasMore && last != nil {
		// 队列用 report_id 游标（复用 timeCursor 的 ID 字段）
		next = encodeTimeCursor(time.Time{}, last.ReportID)
	}
	return &models.PageResult[*models.ApiReportDetail]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// moderationActionResult 审核动作结果：目标内容联动处理
// resolve=dismiss 不动内容；resolve=approve_resolved 隐藏被举报内容（下架）
func ResolveReport(actorID int64, reportID int64, decision string, note string, requestID string) error {
	var newStatus int32
	switch decision {
	case "dismiss":
		newStatus = models.ReportStatusDismissed
	case "resolve":
		newStatus = models.ReportStatusResolved
	default:
		return ErrCommentInvalid
	}
	ok, err := mysql.ResolveReport(reportID, actorID, newStatus, note)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAdminConflict
	}
	// resolve（举报成立）：对 question/answer 目标执行隔离（下架），进审计
	if newStatus == models.ReportStatusResolved {
		r, err := mysql.GetReportByID(reportID)
		if err == nil && (r.TargetType == models.ReportTargetQuestion || r.TargetType == models.ReportTargetAnswer) {
			if err := mysql.SetModeration(r.TargetType, r.TargetID, models.ModerationQuarantined, actorID); err != nil {
				return err
			}
		}
	}
	metadata, _ := json.Marshal(map[string]string{"decision": decision, "note": note})
	logAdminAction(actorID, "report_resolve", "report", strconv.FormatInt(reportID, 10), requestID, string(metadata))
	return nil
}

// ModerateContent 管理员设置内容审核状态
func ModerateContent(actorID int64, targetType, action, targetID, requestID string) error {
	var code int32
	switch action {
	case "approve":
		code = models.ModerationApproved
	case "reject":
		code = models.ModerationRejected
	case "quarantine":
		code = models.ModerationQuarantined
	default:
		return ErrCommentInvalid
	}
	tt := models.ReportTargetTypeToCode(targetType)
	if tt == 0 {
		return ErrCommentInvalid
	}
	id, err := strconv.ParseInt(targetID, 10, 64)
	if err != nil {
		return ErrCommentInvalid
	}
	if err := mysql.SetModeration(tt, id, code, actorID); err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{"action": action})
	logAdminAction(actorID, "content_moderate", targetType, targetID, requestID, string(metadata))
	return nil
}

// SetUserEnabled 管理员禁用/启用用户
func SetUserEnabled(actorID int64, targetUserID int64, enable bool, requestID string) error {
	status := int32(0)
	if enable {
		status = 1
	}
	if err := mysql.SetUserStatus(targetUserID, status); err != nil {
		return err
	}
	// 禁用后撤销该用户全部会话（禁用即时生效）
	if !enable {
		if _, err := mysql.IncrementTokenVersion(targetUserID); err == nil {
			redis.InvalidateTokenVersionCache(targetUserID)
		}
	}
	action := "user_disable"
	if enable {
		action = "user_enable"
	}
	logAdminAction(actorID, action, "user", strconv.FormatInt(targetUserID, 10), requestID, "{}")
	return nil
}

// ListAuditLogsPage 审计日志分页（管理员；moderator 受限场景本版统一 admin）
func ListAuditLogsPage(cursor string, limit int) (*models.PageResult[*models.AuditLog], error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	var cursorID int64
	if cursor != "" {
		c, err := parseTimeCursor(cursor)
		if err != nil {
			return nil, ErrCursorInvalid
		}
		cursorID = c.ID
	}
	logs, err := mysql.ListAuditLogs(&mysql.AuditLogPageParams{CursorID: cursorID, Limit: limit})
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(logs) > limit {
		hasMore = true
		logs = logs[:limit]
	}
	next := ""
	if hasMore && len(logs) > 0 {
		last := logs[len(logs)-1]
		next = encodeTimeCursor(time.Time{}, last.AuditID)
	}
	return &models.PageResult[*models.AuditLog]{Items: logs, NextCursor: next, HasMore: hasMore}, nil
}

// MetricsOverview 管理端指标总览（规格 9.4 子集：以事实表计数为准）
func MetricsOverview() (map[string]interface{}, error) {
	m := map[string]interface{}{}
	if v, err := mysql.CountTable("user"); err == nil {
		m["users"] = v
	}
	if v, err := mysql.CountTableWhere("question", "status = 1 AND deleted_at IS NULL"); err == nil {
		m["questions"] = v
	}
	if v, err := mysql.CountTableWhere("question", "moderation_status = 0"); err == nil {
		m["questions_pending"] = v
	}
	if v, err := mysql.CountTableWhere("answer", "status = 1 AND deleted_at IS NULL"); err == nil {
		m["answers"] = v
	}
	if v, err := mysql.CountTableWhere("report", "status = 0"); err == nil {
		m["reports_pending"] = v
	}
	if v, err := mysql.CountTableWhere("user", "status = 0"); err == nil {
		m["users_disabled"] = v
	}
	if v, err := mysql.CountTable("audit_log"); err == nil {
		m["audit_events"] = v
	}
	return m, nil
}

