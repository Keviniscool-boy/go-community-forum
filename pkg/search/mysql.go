package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

// MySQLFallback 基于 MySQL LIKE 的实时搜索实现。
// 查询直接带统一公开可见条件（status=1 AND moderation_status=1 AND visibility=1
// AND deleted_at IS NULL 且作者未禁用），不存在索引泄露问题；数据量大时切换 ES 适配器。
type MySQLFallback struct {
	DB *sqlx.DB
}

var _ SearchIndex = (*MySQLFallback)(nil)

type scopedRow struct {
	ItemType   string  `db:"item_type"`
	ItemID     int64   `db:"item_id"`
	Title      string  `db:"title"`
	Excerpt    string  `db:"excerpt"`
	AuthorID   int64   `db:"author_id"`
	Solved     bool    `db:"solved"`
	QuestionID int64   `db:"question_id"`
	Score      float64 `db:"score"`
	CreateTime int64   `db:"create_time"`
}

// 公开可见条件片段
const (
	questionVisible = `q.status = 1 AND q.moderation_status = 1 AND q.visibility = 1 AND q.deleted_at IS NULL`
	questionVisibleByQu = `qu.status = 1 AND qu.moderation_status = 1 AND qu.visibility = 1 AND qu.deleted_at IS NULL`
	answerVisible   = `a.status = 1 AND a.moderation_status = 1 AND a.visibility = 1 AND a.deleted_at IS NULL`
)

// Search LIKE 全文搜索（scope 过滤；relevance = 命中权重 + 新鲜度）
func (m *MySQLFallback) Search(ctx context.Context, q Query) (Result, error) {
	kw := strings.TrimSpace(q.Keyword)
	if kw == "" {
		return Result{Items: []ResultItem{}}, nil
	}
	like := "%" + strings.ReplaceAll(kw, "%", "\\%") + "%"
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	limit++
	scopes := map[string]bool{"question": true, "answer": true, "user": true, "topic": true, "article": true, "column": true}
	selected := []string{"question", "answer", "article", "user", "topic", "column"}
	if q.Scope != "all" && scopes[q.Scope] {
		selected = []string{q.Scope}
	}
	// 游标解析：sort:offset 简化实现（相关性排序的页边界由 limit+1 判定，静态数据稳定）
	offset := int64(0)
	if q.CursorID > 0 {
		offset = q.CursorID
	}

	order := "score DESC, create_time DESC"
	if q.Sort == "latest" {
		order = "create_time DESC"
	}

	// 分段查询（每段 limit 简化为全量候选上限，合并后内存排序分页）
	const perScopeLimit = 50
	union := ""
	switch {
	case contains(selected, "question"):
		union += fmt.Sprintf(`(SELECT 'question' AS item_type, q.question_id AS item_id, q.title AS title,
			LEFT(q.description, 200) AS excerpt, q.author_id AS author_id,
			q.resolution_status = 1 AS solved, 0 AS question_id,
			(CASE WHEN q.title LIKE ? THEN 3 ELSE 0 END + CASE WHEN q.description LIKE ? THEN 1 ELSE 0 END) AS score,
			UNIX_TIMESTAMP(q.create_time) AS create_time
			FROM question q JOIN user u ON u.user_id = q.author_id AND u.status = 1
			WHERE %s AND (q.title LIKE ? OR q.description LIKE ?) LIMIT %d)`, questionVisible, perScopeLimit)
		union += " UNION ALL "
	case false:
	}
	if contains(selected, "article") {
		union += fmt.Sprintf(`(SELECT 'article' AS item_type, a.article_id AS item_id, a.title AS title,
			LEFT(a.content, 200) AS excerpt, a.author_id AS author_id,
			0 AS solved, 0 AS question_id,
			(CASE WHEN a.title LIKE ? THEN 3 ELSE 0 END + CASE WHEN a.content LIKE ? THEN 1 ELSE 0 END) AS score,
			UNIX_TIMESTAMP(a.create_time) AS create_time
			FROM article a JOIN user u ON u.user_id = a.author_id AND u.status = 1
			WHERE a.status = 1 AND a.moderation_status = 1 AND a.visibility = 1 AND a.deleted_at IS NULL
			AND (a.title LIKE ? OR a.content LIKE ?) LIMIT %d)`, perScopeLimit)
		union += " UNION ALL "
	}
	if contains(selected, "column") {
		union += fmt.Sprintf(`(SELECT 'column' AS item_type, c.column_id AS item_id, c.name AS title,
			LEFT(c.description, 200) AS excerpt, c.owner_id AS author_id,
			0 AS solved, 0 AS question_id,
			(CASE WHEN c.name LIKE ? THEN 3 ELSE 0 END + CASE WHEN c.description LIKE ? THEN 1 ELSE 0 END) AS score,
			UNIX_TIMESTAMP(c.create_time) AS create_time
			FROM column_info c JOIN user u ON u.user_id = c.owner_id AND u.status = 1
			WHERE c.status = 1 AND c.moderation_status = 1 AND c.visibility = 1 AND c.deleted_at IS NULL
			AND (c.name LIKE ? OR c.description LIKE ?) LIMIT %d)`, perScopeLimit)
		union += " UNION ALL "
	}
	if contains(selected, "answer") {
		union += fmt.Sprintf(`(SELECT 'answer' AS item_type, a.answer_id AS item_id, qu.title AS title,
			LEFT(a.content, 200) AS excerpt, a.author_id AS author_id,
			0 AS solved, qu.question_id AS question_id,
			(CASE WHEN a.content LIKE ? THEN 1 ELSE 0 END + CASE WHEN qu.title LIKE ? THEN 2 ELSE 0 END) AS score,
			UNIX_TIMESTAMP(a.create_time) AS create_time
			FROM answer a
			JOIN question qu ON qu.question_id = a.question_id AND %s
			JOIN user u ON u.user_id = a.author_id AND u.status = 1
			WHERE %s AND a.content LIKE ? LIMIT %d)`, questionVisibleByQu, answerVisible, perScopeLimit)
		union += " UNION ALL "
	}
	if contains(selected, "user") {
		union += fmt.Sprintf(`(SELECT 'user' AS item_type, u.user_id AS item_id, u.username AS title,
			LEFT(u.bio, 200) AS excerpt, u.user_id AS author_id,
			0 AS solved, 0 AS question_id,
			(CASE WHEN u.username LIKE ? THEN 3 ELSE 0 END + CASE WHEN u.bio LIKE ? THEN 1 ELSE 0 END) AS score,
			UNIX_TIMESTAMP(NOW()) AS create_time
			FROM user u WHERE u.status = 1 AND (u.username LIKE ? OR u.bio LIKE ?) LIMIT %d)`, perScopeLimit)
		union += " UNION ALL "
	}
	if contains(selected, "topic") {
		union += fmt.Sprintf(`(SELECT 'topic' AS item_type, t.topic_id AS item_id, t.name AS title,
			LEFT(t.description, 200) AS excerpt, 0 AS author_id,
			0 AS solved, 0 AS question_id,
			(CASE WHEN t.name LIKE ? THEN 3 ELSE 0 END + CASE WHEN t.description LIKE ? THEN 1 ELSE 0 END) AS score,
			UNIX_TIMESTAMP(t.create_time) AS create_time
			FROM topic t WHERE (t.name LIKE ? OR t.description LIKE ?) LIMIT %d)`, perScopeLimit)
	} else {
		union = strings.TrimSuffix(union, " UNION ALL ")
	}
	if union == "" {
		return Result{Items: []ResultItem{}}, nil
	}

	// 参数：每段 LIKE 参数顺序与占位符一致
	var args []interface{}
	addLikes := func(n int) {
		for i := 0; i < n; i++ {
			args = append(args, like)
		}
	}
	// 逐段注入参数（与上面 CASE 顺序一致）
	if contains(selected, "question") {
		addLikes(4)
	}
	if contains(selected, "article") {
		addLikes(4)
	}
	if contains(selected, "column") {
		addLikes(4)
	}
	if contains(selected, "answer") {
		addLikes(3)
	}
	if contains(selected, "user") {
		addLikes(4)
	}
	if contains(selected, "topic") {
		addLikes(4)
	}

	query := fmt.Sprintf("SELECT * FROM (%s) combined ORDER BY %s LIMIT ? OFFSET ?", union, order)
	args = append(args, limit, offset)
	rows := make([]scopedRow, 0)
	if err := m.DB.SelectContext(ctx, &rows, query, args...); err != nil {
		return Result{}, err
	}
	hasMore := false
	if len(rows) > limit-1 {
		hasMore = true
		rows = rows[:limit-1]
	}
	items := make([]ResultItem, 0, len(rows))
	for _, r := range rows {
		item := ResultItem{
			DocumentType: r.ItemType,
			DocumentID:   fmt.Sprintf("%d", r.ItemID),
			Title:        r.Title,
			Excerpt:      r.Excerpt,
			AuthorID:     fmt.Sprintf("%d", r.AuthorID),
			Solved:       r.Solved,
			CreatedAt:    r.CreateTime,
		}
		if r.QuestionID > 0 {
			item.QuestionID = fmt.Sprintf("%d", r.QuestionID)
		}
		items = append(items, item)
	}
	next := ""
	if hasMore {
		next = fmt.Sprintf("%d", offset+int64(len(items)))
	}
	return Result{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

// Suggest 建议词：话题名与用户名前缀匹配（不暴露未审核内容）
func (m *MySQLFallback) Suggest(ctx context.Context, prefix string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 10
	}
	p := prefix + "%"
	out := make([]string, 0, limit)
	var topicNames []string
	if err := m.DB.SelectContext(ctx, &topicNames,
		"SELECT name FROM topic WHERE name LIKE ? ORDER BY follower_count DESC LIMIT ?", p, limit); err == nil {
		out = append(out, topicNames...)
	}
	if len(out) < limit {
		var userNames []string
		if err := m.DB.SelectContext(ctx, &userNames,
			"SELECT username FROM user WHERE status = 1 AND username LIKE ? LIMIT ?", p, limit-len(out)); err == nil {
			out = append(out, userNames...)
		}
	}
	return out, nil
}

// Rebuild MySQL fallback 为实时查询，无需重建
func (m *MySQLFallback) Rebuild(ctx context.Context) error { return nil }

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
