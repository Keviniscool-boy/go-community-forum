package models

import "time"

// 内容治理状态
const (
	ModerationPending    = 0 // 待审核
	ModerationApproved   = 1 // 审核通过
	ModerationRejected   = 2 // 审核拒绝
	ModerationQuarantined = 3 // 隔离（仅管理员可见）
)

const (
	VisibilityHidden     = 0 // 隐藏
	VisibilityPublic     = 1 // 公开
	VisibilityAuthorOnly = 2 // 仅作者可见
)

// 用户角色
const (
	RoleUser      = "user"
	RoleModerator = "moderator"
	RoleAdmin     = "admin"
)

// 举报目标类型（JSON 交互用字符串）
const (
	ReportTargetQuestion = 1
	ReportTargetAnswer   = 2
	ReportTargetComment  = 3
	ReportTargetUser     = 4
)

// ReportTargetTypeToCode 字符串转编码，非法返回 0
func ReportTargetTypeToCode(s string) int32 {
	switch s {
	case "question":
		return ReportTargetQuestion
	case "answer":
		return ReportTargetAnswer
	case "comment":
		return ReportTargetComment
	case "user":
		return ReportTargetUser
	}
	return 0
}

// 举报状态
const (
	ReportStatusPending  = 0
	ReportStatusResolved = 1
	ReportStatusDismissed = 2
)

// Report 举报模型
type Report struct {
	ReportID    int64      `json:"id,string" db:"report_id"`
	ReporterID  int64      `json:"reporter_id,string" db:"reporter_id"`
	TargetType  int32      `json:"target_type" db:"target_type"`
	TargetID    int64      `json:"target_id,string" db:"target_id"`
	ReasonCode  string     `json:"reason_code" db:"reason_code"`
	Description string     `json:"description" db:"description"`
	Status      int32      `json:"status" db:"status"`
	ReviewerID  int64      `json:"reviewer_id,string" db:"reviewer_id"`
	ReviewNote  string     `json:"review_note" db:"review_note"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	ReviewedAt  *time.Time `json:"reviewed_at" db:"reviewed_at"`
}

// ApiReportDetail 举报详情（含举报者用户名，管理员视图）
type ApiReportDetail struct {
	*Report
	ReporterName string `json:"reporter_name"`
}

// ParamCreateReport 创建举报请求
type ParamCreateReport struct {
	TargetType  string `json:"target_type" binding:"required,oneof=question answer comment user"`
	TargetID    string `json:"target_id" binding:"required"`
	ReasonCode  string `json:"reason_code" binding:"required,oneof=spam abuse copyright unsafe other"`
	Description string `json:"description" binding:"max=1000"`
}

// AuditLog 审计日志（只追加）
type AuditLog struct {
	AuditID    int64     `json:"id,string" db:"audit_id"`
	ActorID    int64     `json:"actor_id,string" db:"actor_id"`
	Action     string    `json:"action" db:"action"`
	TargetType string    `json:"target_type" db:"target_type"`
	TargetID   string    `json:"target_id" db:"target_id"`
	RequestID  string    `json:"request_id" db:"request_id"`
	IPHash     string    `json:"ip_hash" db:"ip_hash"`
	Metadata   string    `json:"metadata" db:"metadata_json"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}
