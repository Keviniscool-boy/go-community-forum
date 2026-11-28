package models

import "time"

// Question 问题模型
type Question struct {
	ID            int64     `json:"id,string" db:"question_id"`
	Title         string    `json:"title" db:"title"`
	Description   string    `json:"description" db:"description"`
	AuthorID      int64     `json:"author_id,string" db:"author_id"`
	AnswerCount   int32     `json:"answer_count" db:"answer_count"`
	CommentCount  int32     `json:"comment_count" db:"comment_count"`
	FollowerCount int32     `json:"follower_count" db:"follower_count"`
	ViewCount     int32     `json:"view_count" db:"view_count"`
	Status        int32     `json:"status" db:"status"`
	CreateTime    time.Time `json:"create_time" db:"create_time"`
	RedirectTo    *int64    `json:"redirect_to,omitempty" db:"redirect_to"` // 治理合并目标（PRD F8）
}

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
type Answer struct {
	ID           int64     `json:"id,string" db:"answer_id"`
	QuestionID   int64     `json:"question_id,string" db:"question_id"`
	AuthorID     int64     `json:"author_id,string" db:"author_id"`
	Content      string    `json:"content" db:"content" binding:"required"`
	VoteCount    int32     `json:"vote_count" db:"vote_count"`
	CommentCount int32     `json:"comment_count" db:"comment_count"`
	CollectCount int32     `json:"collect_count" db:"collect_count"`
	Status       int32     `json:"status" db:"status"`
	CreateTime   time.Time `json:"create_time" db:"create_time"`
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

// ParamQuestionRedirect 问题重定向请求（PRD F8）
type ParamQuestionRedirect struct {
	TargetID string `json:"target_id" binding:"required,min=1"`
	Note     string `json:"note" binding:"max=500"`
}
