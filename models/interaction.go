package models

import (
	"database/sql"
	"time"
)

// 评论目标类型（存储用 tinyint，JSON 交互用字符串）
const (
	TargetAnswer   = 1
	TargetQuestion = 2
)

func IsPrivilegedRole(role string) bool {
	return role == RoleModerator || role == RoleAdmin
}

// TargetTypeToCode 字符串目标类型转存储编码，非法返回 0
func TargetTypeToCode(s string) int32 {
	switch s {
	case "answer":
		return TargetAnswer
	case "question":
		return TargetQuestion
	}
	return 0
}

// 通知类型
const (
	NotifyTypeLike    = 1 // 赞同
	NotifyTypeComment = 2 // 评论
	NotifyTypeFollow  = 3 // 关注
	NotifyTypeCollect = 4 // 收藏
	// 第四阶段：评论体系补全（追加在末尾，不改动已发布类型编号）
	NotifyTypeCommentLike = 5 // 赞同了你的评论
	NotifyTypeReply       = 6 // 回复了你的评论
	NotifyTypeDM          = 7 // 私信（第五阶段 F6，时间窗聚合）
)

// Comment 评论模型
type Comment struct {
	CommentID        int64        `json:"id,string" db:"comment_id"`
	UserID           int64        `json:"user_id,string" db:"user_id"`
	TargetType       int32        `json:"target_type" db:"target_type"`
	TargetID         int64        `json:"target_id,string" db:"target_id"`
	ParentID         int64        `json:"parent_id,string" db:"parent_id"`
	ReplyToUserID    int64        `json:"-" db:"reply_to_user_id"` // 被回复的回复的作者（0 = 直接回复一级评论）
	Content          string       `json:"content" db:"content"`
	Status           int32        `json:"status" db:"status"`
	ModerationStatus int32        `json:"-" db:"moderation_status"`
	DeletedAt        sql.NullTime `json:"-" db:"deleted_at"`
	CreateTime       time.Time    `json:"create_time" db:"create_time"`
	UpdateTime       sql.NullTime `json:"-" db:"update_time"` // 编辑时间（>create_time 视为已编辑）
}

// BlockedUserItem 黑名单条目
type BlockedUserItem struct {
	BlockedID  int64      `json:"user_id,string" db:"blocked_id"`
	Username   string     `json:"username" db:"username"`
	AvatarURL  string     `json:"avatar_url" db:"avatar_url"`
	CreateTime time.Time  `json:"create_time" db:"create_time"`
}

// FollowListUser 关注/粉丝列表条目（PRD F1）
type FollowListUser struct {
	UserID       string `json:"user_id" db:"user_id"`
	Username     string `json:"username" db:"username"`
	AvatarURL    string `json:"avatar_url" db:"avatar_url"`
	Bio          string `json:"bio" db:"bio"`
	IsFollowing  bool   `json:"is_following"`           // 登录视角下我是否已关注对方；游客恒 false
	ListCursorID int64  `json:"-" db:"follow_id"` // 分页游标（不输出）
}

// ApiCommentDetail 评论详情（含评论者信息）
type ApiCommentDetail struct {
	*Comment
	Username    string `json:"username"`
	AvatarURL   string `json:"avatar_url"`
	ReplyToUser string `json:"reply_to_user,omitempty"` // 被回复的回复的作者（「回复 @xxx」）
	LikeCount   int32  `json:"like_count"`
	ReplyCount  int32  `json:"reply_count"` // 直接回复数（仅一级评论有效）
	Liked       bool   `json:"liked"`       // 当前用户是否已点赞
	CanDelete   bool   `json:"can_delete"`  // 当前用户是否可删除（作者或管理员/审核员）
	IsDeleted   bool   `json:"is_deleted"`  // 已删除但保留占位（下方仍有可见回复）
	CanEdit     bool   `json:"can_edit"`    // 当前用户是否可编辑（作者且未超窗口，PRD F4）
	Edited      bool   `json:"edited"`      // update_time > create_time 即已编辑
}

// ParamCreateComment 创建评论请求
type ParamCreateComment struct {
	TargetType string `json:"target_type" binding:"required,oneof=answer question"`
	TargetID   string `json:"target_id" binding:"required"`
	ParentID   string `json:"parent_id"`
	Content    string `json:"content" binding:"required,min=1,max=2000"`
}

// ParamVoteAnswer 回答投票请求（1 赞同 0 取消 -1 反对）
type ParamVoteAnswer struct {
	Direction int8 `json:"direction" binding:"oneof=1 0 -1"`
}

// Notification 通知模型
type Notification struct {
	NotificationID int64     `json:"id,string" db:"notification_id"`
	UserID         int64     `json:"user_id,string" db:"user_id"`
	ActorID        int64     `json:"actor_id,string" db:"actor_id"`
	Type           int32     `json:"type" db:"type"`
	TargetType     int32     `json:"target_type" db:"target_type"`
	TargetID       int64     `json:"target_id,string" db:"target_id"`
	IsRead         bool      `json:"is_read" db:"is_read"`
	CreateTime     time.Time `json:"create_time" db:"create_time"`
}

// ApiNotificationDetail 通知详情（含触发者用户名与摘要文案）
type ApiNotificationDetail struct {
	*Notification
	ActorName string `json:"actor_name"`
	Excerpt   string `json:"excerpt"`
	// 目标为回答时的所属问题 ID：通知跳转用（回答通知的 target_id 是回答 ID，不能直接拼问题地址）
	QuestionID string `json:"question_id,omitempty"`
}

// FeedAuthor Feed 条目的作者信息
type FeedAuthor struct {
	UserID    string `json:"user_id"` // 已是字符串，不加 ,string（避免双重引号）
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}

// FeedStats Feed 条目的互动数据
type FeedStats struct {
	VoteCount    int32 `json:"vote_count"`
	CommentCount int32 `json:"comment_count"`
	CollectCount int32 `json:"collect_count"`
	AnswerCount  int32 `json:"answer_count,omitempty"`
}

// FeedItem Feed 流条目（问题或回答）
type FeedItem struct {
	ItemType   string      `json:"item_type"` // question | answer
	ItemID     string      `json:"item_id"`   // 已是字符串，不加 ,string（避免双重引号）
	QuestionID string      `json:"question_id,omitempty"`
	Title      string      `json:"title"`
	Excerpt    string      `json:"excerpt"`
	Author     FeedAuthor  `json:"author"`
	Topic      *TopicBrief `json:"topic,omitempty"`
	Stats      FeedStats   `json:"stats"`
	CreateTime time.Time   `json:"create_time"`
}

// ---------- Swagger 文档用具体分页类型（swag CLI 对泛型解析有限制） ----------

// FeedItemPage Feed 分页响应
type FeedItemPage struct {
	Items      []*FeedItem `json:"items"`
	NextCursor string      `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
}

// AnswerDetailPage 回答分页响应
type AnswerDetailPage struct {
	Items      []*ApiAnswerDetail `json:"items"`
	NextCursor string             `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
}

// CommentDetailPage 评论分页响应
type CommentDetailPage struct {
	Items      []*ApiCommentDetail `json:"items"`
	NextCursor string              `json:"next_cursor"`
	HasMore    bool                `json:"has_more"`
}

// NotificationDetailPage 通知分页响应
type NotificationDetailPage struct {
	Items      []*ApiNotificationDetail `json:"items"`
	NextCursor string                   `json:"next_cursor"`
	HasMore    bool                     `json:"has_more"`
}

// ParamEditComment 评论编辑请求（第五阶段 F4）
type ParamEditComment struct {
	Content string `json:"content" binding:"required,min=1,max=2000"`
}
