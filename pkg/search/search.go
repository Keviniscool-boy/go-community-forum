// Package search 搜索抽象：业务层依赖接口，不依赖具体搜索产品。
// 开发环境使用 MySQL fallback（实时查询）；生产环境可接入 Elasticsearch/OpenSearch 适配器。
// 搜索索引允许重建，不是事实来源；只返回公开、审核通过、未删除内容。
package search

import (
	"context"
)

// Document 搜索索引文档
type Document struct {
	DocumentID string // document_type:id
	Type       string // question | answer | user | topic
	Title      string
	PlainText  string
	Excerpt    string
	AuthorID   string
	TopicIDs   []string
	CreatedAt  int64 // unix
	Popularity float64
}

// Query 查询参数
type Query struct {
	Keyword  string // 1-100 字符
	Scope    string // all | question | answer | article | column | user | topic
	Type     string // 内容类型过滤（scope 为空时生效）
	Sort     string // relevance | latest
	Cursor   string // 不透明游标（实现自定义；上一页 NextCursor 原样传回）
	Limit    int
	ViewerID int64 // 0 = 游客
}

// ResultItem 结果条目
type ResultItem struct {
	DocumentType string  `json:"document_type"`
	DocumentID   string  `json:"document_id"` // 已是字符串，不加 ,string（避免双重引号）
	Title        string  `json:"title"`
	Excerpt      string  `json:"excerpt"`
	AuthorID     string  `json:"author_id,omitempty"`
	Solved       bool    `json:"solved,omitempty"`    // question：是否已解决
	QuestionID   string  `json:"question_id,omitempty"` // answer：所属问题 id（跳转用）
	Popularity   float64 `json:"-"`
	CreatedAt    int64   `json:"create_time"`
}

// Result 搜索结果（cursor 由实现生成）
type Result struct {
	Items      []ResultItem `json:"items"`
	NextCursor string       `json:"next_cursor"`
	HasMore    bool         `json:"has_more"`
}

// SearchIndex 搜索索引接口（规格 6.2）
type SearchIndex interface {
	Search(ctx context.Context, q Query) (Result, error)
	Suggest(ctx context.Context, prefix string, limit int) ([]string, error)
	Rebuild(ctx context.Context) error // 全量重建（MySQL fallback 无索引可重建，空实现）
}
