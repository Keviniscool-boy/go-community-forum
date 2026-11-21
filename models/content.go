package models

import "time"

// Question 问题模型
// resolution_status 与审核/可见性状态分离（迁移 014）：0=open 1=solved 2=closed
type Question struct {
	ID               int64     `json:"id,string" db:"question_id"`
	Title            string    `json:"title" db:"title"`
	Description      string    `json:"description" db:"description"`
	AuthorID         int64     `json:"author_id,string" db:"author_id"`
	AnswerCount      int32     `json:"answer_count" db:"answer_count"`
	CommentCount     int32     `json:"comment_count" db:"comment_count"`
	FollowerCount    int32     `json:"follower_count" db:"follower_count"`
	ViewCount        int32     `json:"view_count" db:"view_count"`
	Status           int32     `json:"status" db:"status"`
	ResolutionStatus int32     `json:"resolution_status" db:"resolution_status"`
	AcceptedAnswerID int64     `json:"accepted_answer_id,string" db:"accepted_answer_id"`
	CreateTime       time.Time `json:"create_time" db:"create_time"`
	UpdateTime       time.Time `json:"update_time" db:"update_time"`
}

// 问题解决状态（业务状态，独立于审核/删除状态）
const (
	QuestionResolutionOpen   = int32(0) // 可回答
	QuestionResolutionSolved = int32(1) // 已采纳答案
	QuestionResolutionClosed = int32(2) // 已关闭（不接受新回答）
)

// ApiQuestionDetail 问题详情接口返回模型（问题 + 作者名 + 话题列表）
type ApiQuestionDetail struct {
	*Question
	AuthorName string           `json:"author_name"`
	Topics     []*TopicBrief    `json:"topics"`
}

// ParamCreateQuestion 创建问题请求
type ParamCreateQuestion struct {
	Title       string   `json:"title" binding:"required,min=5,max=200"`
	Description string   `json:"description" binding:"max=10000"`
	TopicIDs    []string `json:"topic_ids" binding:"max=5,dive,min=1"`
}

// TopicBrief 问题卡片上展示的话题简要信息
type TopicBrief struct {
	TopicID string `json:"topic_id,string" db:"topic_id"`
	Name    string `json:"name" db:"name"`
}

// PageResult 通用分页包装（cursor 分页）
type PageResult[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
}

// Answer 回答模型
// Accepted 标记为采纳状态（由 question.accepted_answer_id 派生，非持久化列）
type Answer struct {
	ID           int64     `json:"id,string" db:"answer_id"`
	QuestionID   int64     `json:"question_id,string" db:"question_id"`
	AuthorID     int64     `json:"author_id,string" db:"author_id"`
	Content      string    `json:"content" db:"content" binding:"required"`
	VoteCount    int32     `json:"vote_count" db:"vote_count"`
	CommentCount int32     `json:"comment_count" db:"comment_count"`
	CollectCount int32     `json:"collect_count" db:"collect_count"`
	Status       int32     `json:"status" db:"status"`
	Accepted     bool      `json:"accepted" db:"-"`
	CreateTime   time.Time `json:"create_time" db:"create_time"`
	UpdateTime   time.Time `json:"update_time" db:"update_time"`
}

// ApiAnswerDetail 回答详情接口返回模型（回答 + 作者名；用户主页场景带问题标题）
type ApiAnswerDetail struct {
	*Answer
	AuthorName    string `json:"author_name"`
	QuestionTitle string `json:"question_title,omitempty" db:"question_title"`
}

// ParamCreateAnswer 创建回答请求
type ParamCreateAnswer struct {
	Content string `json:"content" binding:"required,min=1,max=20000"`
}

// 回答列表排序方式
const (
	AnswerOrderHot    = "hot"
	AnswerOrderLatest = "latest"
)

// Topic 话题模型
type Topic struct {
	TopicID       int64     `json:"topic_id,string" db:"topic_id"`
	Name          string    `json:"name" db:"name"`
	Description   string    `json:"description" db:"description"`
	AvatarURL     string    `json:"avatar_url" db:"avatar_url"`
	FollowerCount int32     `json:"follower_count" db:"follower_count"`
	CreateTime    time.Time `json:"create_time" db:"create_time"`
}

// ParamCreateTopic 创建话题请求
type ParamCreateTopic struct {
	Name        string `json:"name" binding:"required,min=1,max=64"`
	Description string `json:"description" binding:"max=500"`
}

// ParamUpdateQuestion 编辑问题请求
type ParamUpdateQuestion struct {
	Title       string `json:"title" binding:"required,min=1,max=200"`
	Description string `json:"description" binding:"max=10000"`
}

// ParamSetQuestionStatus 问题状态变更请求（仅 open/closed；solved 由采纳操作维护）
type ParamSetQuestionStatus struct {
	Status string `json:"status" binding:"required,oneof=open closed"`
}

// GET /me/content 内容类型与状态过滤（我的内容管理，PRD §4.4）
const (
	MeContentQuestion = "question"
	MeContentAnswer   = "answer"
	MeContentArticle  = "article"

	MeContentPublished = "published" // 公开（审核通过）
	MeContentPending   = "pending"   // 审核中
	MeContentHidden    = "hidden"    // 退回/隔离/隐藏
	MeContentDeleted   = "deleted"   // 已删除
)

// ApiMyContentItem 我的内容管理条目（跨问题/回答/文章统一形状）
type ApiMyContentItem struct {
	ContentType      string    `json:"content_type"` // question | answer | article
	ContentID        int64     `json:"content_id,string"`
	Title            string    `json:"title"`                     // 问题/文章标题；回答为其所属问题标题
	Excerpt          string    `json:"excerpt"`                   // 回答/文章正文摘要
	Status           int32     `json:"status"`                    // 1=存在 0=已软删除
	ModerationStatus int32     `json:"moderation_status"`         // 0 待审 1 通过 2 拒绝 3 隔离
	ResolutionStatus int32     `json:"resolution_status"`         // 仅问题有值；回答/文章为 0
	CreateTime       time.Time `json:"create_time"`
}

// ParamMyContent GET /me/content 查询参数
type ParamMyContent struct {
	Type   string `form:"type" json:"type" binding:"required,oneof=question answer article"`
	Status string `form:"status" json:"status" binding:"omitempty,oneof=published pending hidden deleted"`
	Cursor string `form:"cursor" json:"cursor"`
	Limit  int    `form:"limit" json:"limit"`
}

// ParamUpdateAnswer 编辑回答请求
type ParamUpdateAnswer struct {
	Content string `json:"content" binding:"required,min=1,max=20000"`
}

// ParamUpdateProfile 更新个人资料请求（用户名可选，非空时更新）
type ParamUpdateProfile struct {
	Username  string `json:"username" binding:"omitempty,min=2,max=32"`
	Bio       string `json:"bio" binding:"max=500"`
	AvatarURL string `json:"avatar_url" binding:"max=500"`
}
