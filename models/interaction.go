package models

import "time"

// 评论目标类型（存储用 tinyint，JSON 交互用字符串）
const (
	TargetAnswer   = 1
	TargetQuestion = 2
	TargetArticle  = 3
)

// TargetTypeToCode 字符串目标类型转存储编码，非法返回 0
func TargetTypeToCode(s string) int32 {
	switch s {
	case "answer":
		return TargetAnswer
	case "question":
		return TargetQuestion
	case "article":
		return TargetArticle
	}
	return 0
}

// 通知类型
const (
	NotifyTypeLike    = 1 // 赞同
	NotifyTypeComment = 2 // 评论
	NotifyTypeFollow  = 3 // 关注
	NotifyTypeCollect = 4 // 收藏
	NotifyTypeDM      = 5 // 私信（outbox worker 投递）
)

// Comment 评论模型
type Comment struct {
	CommentID  int64     `json:"id,string" db:"comment_id"`
	UserID     int64     `json:"user_id,string" db:"user_id"`
	TargetType int32     `json:"target_type" db:"target_type"`
	TargetID   int64     `json:"target_id,string" db:"target_id"`
	ParentID   int64     `json:"parent_id,string" db:"parent_id"`
	Content    string    `json:"content" db:"content"`
	Status     int32     `json:"status" db:"status"`
	CreateTime time.Time `json:"create_time" db:"create_time"`
}

// ApiCommentDetail 评论详情（含评论者信息）
type ApiCommentDetail struct {
	*Comment
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
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
	AggCount  int    `json:"agg_count,omitempty"` // 同目标同类型聚合条数（仅展示层聚合，>1 时前端显示"某某等 N 人"）
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

// FeedItem Feed 流条目（问题/回答/文章）
type FeedItem struct {
	ItemType   string      `json:"item_type"` // question | answer | article
	ItemID     string      `json:"item_id"`   // 已是字符串，不加 ,string（避免双重引号）
	QuestionID string      `json:"question_id,omitempty"`
	Title      string      `json:"title"`
	Excerpt    string      `json:"excerpt"`
	Author     FeedAuthor  `json:"author"`
	Topic      *TopicBrief `json:"topic,omitempty"`
	Column     *ColumnBrief `json:"column,omitempty"`
	Stats      FeedStats   `json:"stats"`
	CreateTime time.Time   `json:"create_time"`
	// 关注流来源标注（规格 4.2）：author | topic | column
	Sources []string `json:"sources,omitempty"`
	// 推荐理由（可枚举安全文案，不含他人偏好）
	RecommendationReason string `json:"recommendation_reason,omitempty"`
	// 曝光关联 ID（服务端签发，点击事件上报时携带）
	ExposureID string `json:"exposure_id,omitempty"`
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
