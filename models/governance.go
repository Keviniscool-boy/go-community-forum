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

// ---------- Swagger 文档用具体分页类型 ----------

// ReportPage 举报队列分页
type ReportPage struct {
	Items      []*ApiReportDetail `json:"items"`
	NextCursor string             `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
}

// AuditLogPage 审计日志分页
type AuditLogPage struct {
	Items      []*AuditLog `json:"items"`
	NextCursor string      `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}

// MediaAsset 媒体资产
type MediaAsset struct {
	AssetID   int64      `json:"asset_id,string" db:"asset_id"`
	OwnerID   int64      `json:"owner_id,string" db:"owner_id"`
	ObjectKey string     `json:"object_key" db:"object_key"`
	MimeType  string     `json:"mime_type" db:"mime_type"`
	ByteSize  int64      `json:"byte_size" db:"byte_size"`
	SHA256    string     `json:"sha256" db:"sha256"`
	Width     int32      `json:"width" db:"width"`
	Height    int32      `json:"height" db:"height"`
	Status    int32      `json:"status" db:"status"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	DeletedAt *time.Time `json:"deleted_at" db:"deleted_at"`
}

// Draft 草稿
type Draft struct {
	DraftID    int64      `json:"id,string" db:"draft_id"`
	UserID     int64      `json:"user_id,string" db:"user_id"`
	DraftType  int32      `json:"draft_type" db:"draft_type"` // 0 question 1 answer
	QuestionID int64      `json:"question_id,string" db:"question_id"`
	Title      string     `json:"title" db:"title"`
	Content    string     `json:"content" db:"content"`
	Version    int64      `json:"version" db:"version"`
	UpdateTime time.Time  `json:"update_time" db:"update_time"`
}

// ParamSaveDraft 保存草稿请求
type ParamSaveDraft struct {
	DraftType  string `json:"draft_type" binding:"required,oneof=question answer"`
	QuestionID string `json:"question_id"`
	Title      string `json:"title" binding:"max=200"`
	Content    string `json:"content" binding:"required,max=20000"`
}

// BookmarkFolder 收藏夹
type BookmarkFolder struct {
	FolderID  int64     `json:"folder_id,string" db:"folder_id"`
	UserID    int64     `json:"user_id,string" db:"user_id"`
	Name      string    `json:"name" db:"name"`
	IsPrivate bool      `json:"is_private" db:"is_private"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ParamCreateFolder 创建收藏夹请求
type ParamCreateFolder struct {
	Name string `json:"name" binding:"required,min=1,max=64"`
}

// BookmarkItem 收藏条目
type BookmarkItem struct {
	TargetType int32     `json:"target_type" db:"target_type"`
	TargetID   int64     `json:"target_id,string" db:"target_id"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// RecommendationEvent 推荐事件
type RecommendationEvent struct {
	EventID         int64   `json:"event_id,string" db:"event_id"`
	AnonymousIDHash string  `json:"-" db:"anonymous_id_hash"`
	UserID          int64   `json:"user_id,omitempty,string" db:"user_id"`
	SessionIDHash   string  `json:"-" db:"session_id_hash"`
	EventType       string  `json:"event_type" db:"event_type"`
	ItemType        string  `json:"item_type" db:"item_type"`
	ItemID          int64   `json:"item_id,string" db:"item_id"`
	FeedType        string  `json:"feed_type" db:"feed_type"`
	Position        *int    `json:"position" db:"position"`
	RequestID       string  `json:"-" db:"request_id"`
	MetadataStr     string  `json:"-" db:"metadata_json"`
}

// RenderMarkdownSafe logic 层包装 sanitizer（避免 controller 直接依赖 pkg 内部实现细节）

// ArticlePage 文章分页响应（Swagger）
type ArticlePage struct {
	Items      []*ApiArticleDetail `json:"items"`
	NextCursor string              `json:"next_cursor"`
	HasMore    bool                `json:"has_more"`
}
