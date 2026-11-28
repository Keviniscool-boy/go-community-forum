package models

import "time"

// ---------- 文章 ----------

// Article 文章模型（Markdown 源文本，渲染走安全子集）
type Article struct {
	ArticleID        int64     `json:"id,string" db:"article_id"`
	AuthorID         int64     `json:"author_id,string" db:"author_id"`
	ColumnID         int64     `json:"column_id,string" db:"column_id"` // 0 表示无专栏
	Title            string    `json:"title" db:"title"`
	Content          string    `json:"content" db:"content"`
	ViewCount        int32     `json:"view_count" db:"view_count"`
	Status           int32     `json:"status" db:"status"`
	ModerationStatus int32     `json:"moderation_status" db:"moderation_status"`
	Visibility       int32     `json:"visibility" db:"visibility"`
	CollabState      int32     `json:"collab_state" db:"collab_state"`
	ApprovedRevisionID int64   `json:"approved_revision_id,string" db:"approved_revision_id"`
	DeletedAt        *time.Time `json:"-" db:"deleted_at"`
	UpdateTime       time.Time `json:"update_time" db:"update_time"`
	CreateTime       time.Time `json:"create_time" db:"create_time"`
}

// ApiArticleDetail 文章详情（含作者与专栏信息）
type ApiArticleDetail struct {
	*Article
	AuthorName string      `json:"author_name"`
	Column     *ColumnBrief `json:"column,omitempty"`
}

// ColumnBrief 专栏简要信息
type ColumnBrief struct {
	ColumnID string `json:"column_id,string" db:"column_id"`
	Name     string `json:"name" db:"name"`
}

// ParamCreateArticle 创建文章请求
type ParamCreateArticle struct {
	Title    string `json:"title" binding:"required,min=1,max=200"`
	Content  string `json:"content" binding:"required,min=1,max=50000"`
	ColumnID string `json:"column_id"` // 可选；无专栏文章允许（记录的默认决策）
}

// ParamUpdateArticle 编辑文章请求
type ParamUpdateArticle struct {
	Title   string `json:"title" binding:"required,min=1,max=200"`
	Content string `json:"content" binding:"required,min=1,max=50000"`
}

// ---------- 专栏 ----------

// Column 专栏模型（表名 column_info）
type Column struct {
	ColumnID         int64      `json:"id,string" db:"column_id"`
	OwnerID          int64      `json:"owner_id,string" db:"owner_id"`
	Name             string     `json:"name" db:"name"`
	Description      string     `json:"description" db:"description"`
	Status           int32      `json:"status" db:"status"`
	ModerationStatus int32      `json:"moderation_status" db:"moderation_status"`
	Visibility       int32      `json:"visibility" db:"visibility"`
	DeletedAt        *time.Time `json:"-" db:"deleted_at"`
	CreateTime       time.Time  `json:"create_time" db:"create_time"`
}

// ApiColumnDetail 专栏详情（含所有者信息）
type ApiColumnDetail struct {
	*Column
	OwnerName    string `json:"owner_name"`
	ArticleCount int32  `json:"article_count"`
}

// ParamCreateColumn 创建专栏请求
type ParamCreateColumn struct {
	Name        string `json:"name" binding:"required,min=1,max=64"`
	Description string `json:"description" binding:"max=1000"`
}

// ParamUpdateColumn 编辑专栏请求
type ParamUpdateColumn struct {
	Name        string `json:"name" binding:"required,min=1,max=64"`
	Description string `json:"description" binding:"max=1000"`
}
