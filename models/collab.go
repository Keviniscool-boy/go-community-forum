package models

import "time"

// ---------- 系列 ----------

// Series 系列知识目录
type Series struct {
	SeriesID     int64      `json:"id,string" db:"series_id"`
	ColumnID     int64      `json:"column_id,string" db:"column_id"`
	Title        string     `json:"title" db:"title"`
	Description  string     `json:"description" db:"description"`
	Status       int32      `json:"status" db:"status"`
	ModerationStatus int32  `json:"moderation_status" db:"moderation_status"`
	Visibility   int32      `json:"visibility" db:"visibility"`
	DeletedAt    *time.Time `json:"-" db:"deleted_at"`
	Version      int64      `json:"version" db:"version"`
	CreateTime   time.Time  `json:"create_time" db:"create_time"`
	UpdateTime   time.Time  `json:"update_time" db:"update_time"`
}

// ParamCreateSeries 创建系列
type ParamCreateSeries struct {
	Title       string `json:"title" binding:"required,min=1,max=100"`
	Description string `json:"description" binding:"max=1000"`
}

// ParamUpdateSeries 更新系列（编辑权限）
type ParamUpdateSeries struct {
	Title       string `json:"title" binding:"required,min=1,max=100"`
	Description string `json:"description" binding:"max=1000"`
	Version     int64  `json:"version" binding:"required,min=1"`
}

// ParamSeriesOrder 目录排序（全量文章 ID 列表 + 版本）
type ParamSeriesOrder struct {
	ArticleIDs []string `json:"article_ids" binding:"required,max=200"`
	Version    int64    `json:"version" binding:"required,min=1"`
}

// ---------- 专栏成员 ----------

// ColumnMemberRole 成员角色
const (
	ColumnRoleOwner    = "owner"
	ColumnRoleEditor   = "editor"
	ColumnRoleReviewer = "reviewer"
)

// ColumnMember 专栏成员
type ColumnMember struct {
	ColumnID  int64     `json:"column_id,string" db:"column_id"`
	UserID    int64     `json:"user_id,string" db:"user_id"`
	Role      string    `json:"role" db:"role"`
	CreateTime time.Time `json:"create_time" db:"create_time"`
}

// ColumnInvitation 专栏邀请
type ColumnInvitation struct {
	InvitationID int64      `json:"id,string" db:"invitation_id"`
	ColumnID     int64      `json:"column_id,string" db:"column_id"`
	InviterID    int64      `json:"inviter_id,string" db:"inviter_id"`
	InviteeID    int64      `json:"invitee_id,string" db:"invitee_id"`
	Role         string     `json:"role" db:"role"`
	Status       int32      `json:"status" db:"status"` // 0 pending 1 accepted 2 declined 3 revoked
	ExpiresAt    time.Time  `json:"expires_at" db:"expires_at"`
	CreateTime   time.Time  `json:"create_time" db:"create_time"`
	ReviewedAt   *time.Time `json:"reviewed_at" db:"reviewed_at"`
}

// ColumnInvitationStatus 邀请状态
const (
	InvitationPending  = 0
	InvitationAccepted = 1
	InvitationDeclined = 2
	InvitationRevoked  = 3
)

// ParamInviteMember 邀请成员
type ParamInviteMember struct {
	UserID string `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required,oneof=editor reviewer"`
}

// ---------- 版本与审阅 ----------

// ArticleRevision 文章不可变版本
type ArticleRevision struct {
	RevisionID  int64     `json:"id,string" db:"revision_id"`
	ArticleID   int64     `json:"article_id,string" db:"article_id"`
	RevisionNo  int64     `json:"revision_no" db:"revision_no"`
	Title       string    `json:"title" db:"title"`
	Body        string    `json:"body" db:"body"`
	AuthorID    int64     `json:"author_id,string" db:"author_id"`
	ContentHash string    `json:"content_hash" db:"content_hash"`
	CreateTime  time.Time `json:"create_time" db:"create_time"`
}

// ArticleCollabState 协作审阅状态
const (
	CollabNone            = 0
	CollabInReview        = 1
	CollabApproved        = 2
	CollabChangesRequested = 3
)

// ReviewRequest 审阅请求（绑定不可变版本）
type ReviewRequest struct {
	ReviewID    int64      `json:"id,string" db:"review_id"`
	ArticleID   int64      `json:"article_id,string" db:"article_id"`
	RevisionID  int64      `json:"revision_id,string" db:"revision_id"`
	State       int32      `json:"state" db:"state"` // 0 in_review 1 approved 2 changes_requested
	InitiatorID int64      `json:"initiator_id,string" db:"initiator_id"`
	CreateTime  time.Time  `json:"create_time" db:"create_time"`
	ReviewedAt  *time.Time `json:"reviewed_at" db:"reviewed_at"`
}

// ReviewComment 审阅评论（纯文本，仅成员可见）
type ReviewComment struct {
	CommentID  int64     `json:"id,string" db:"comment_id"`
	ReviewID   int64     `json:"review_id,string" db:"review_id"`
	AuthorID   int64     `json:"author_id,string" db:"author_id"`
	Body       string    `json:"body" db:"body"`
	CreateTime time.Time `json:"create_time" db:"create_time"`
}

// ParamSubmitReview 提交审阅
type ParamSubmitReview struct {
	RevisionID string `json:"revision_id" binding:"required"`
}

// ParamReviewDecision 审阅决定
type ParamReviewDecision struct {
	Decision string `json:"decision" binding:"required,oneof=approve changes_requested"`
	Revision string `json:"revision" binding:"required"` // 审阅人批准的版本
}

// ParamReviewComment 审阅评论
type ParamReviewComment struct {
	Body string `json:"body" binding:"required,min=1,max=2000"`
}
