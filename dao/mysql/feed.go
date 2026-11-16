package mysql

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// FeedRow Feed 查询的中间行：问题/回答/文章统一结构
// question：title=问题标题，excerpt_src=描述，answer_count 有效
// answer：title=所属问题标题，excerpt_src=回答内容，vote/comment/collect 有效
// article：title=文章标题，excerpt_src=文章内容（截断），无互动数
type FeedRow struct {
	ItemType     string    `db:"item_type"`
	ItemID       int64     `db:"item_id"`
	QuestionID   int64     `db:"question_id"`
	ColumnID     int64     `db:"column_id"`
	AuthorID     int64     `db:"author_id"`
	Title        string    `db:"title"`
	ExcerptSrc   string    `db:"excerpt_src"`
	AnswerCount  int32     `db:"answer_count"`
	VoteCount    int32     `db:"vote_count"`
	CommentCount int32     `db:"comment_count"`
	CollectCount int32     `db:"collect_count"`
	CreateTime   time.Time `db:"create_time"`
}

// 统一公开可见条件（规格 3.3）：status=1 AND moderation_status=1 AND visibility=1 AND deleted_at IS NULL
// 且作者账号未被禁用；Feed 只展示公开状态内容
const (
	questionVisible = `q.status = 1 AND q.moderation_status = 1 AND q.visibility = 1 AND q.deleted_at IS NULL`
	answerVisible   = `a.status = 1 AND a.moderation_status = 1 AND a.visibility = 1 AND a.deleted_at IS NULL`
	articleVisible  = `art.status = 1 AND art.moderation_status = 1 AND art.visibility = 1 AND art.deleted_at IS NULL`
	quVisibleByQu   = `qu.status = 1 AND qu.moderation_status = 1 AND qu.visibility = 1 AND qu.deleted_at IS NULL`
)

// feedUnionSQL 三段 UNION（question / answer / article），各段 %s 为附加过滤条件
const feedUnionSQL = `(SELECT 'question' AS item_type, q.question_id AS item_id, q.question_id AS question_id,
		q.author_id AS author_id, q.title AS title, q.description AS excerpt_src,
		q.answer_count AS answer_count, 0 AS vote_count, q.comment_count AS comment_count, 0 AS collect_count,
		q.create_time AS create_time
	FROM question q JOIN user q_author ON q_author.user_id = q.author_id AND q_author.status = 1
	WHERE ` + questionVisible + ` AND %s)
UNION ALL
(SELECT 'answer' AS item_type, a.answer_id AS item_id, a.question_id AS question_id,
		a.author_id AS author_id, qu.title AS title, a.content AS excerpt_src,
		0 AS answer_count, a.vote_count AS vote_count, a.comment_count AS comment_count, a.collect_count AS collect_count,
		a.create_time AS create_time
	FROM answer a
	JOIN question qu ON qu.question_id = a.question_id AND ` + quVisibleByQu + `
	JOIN user a_author ON a_author.user_id = a.author_id AND a_author.status = 1
	WHERE ` + answerVisible + ` AND %s)
UNION ALL
(SELECT 'article' AS item_type, art.article_id AS item_id, art.article_id AS question_id,
		art.author_id AS author_id, art.title AS title, LEFT(art.content, 500) AS excerpt_src,
		0 AS answer_count, 0 AS vote_count, 0 AS comment_count, 0 AS collect_count,
		art.create_time AS create_time
	FROM article art JOIN user art_author ON art_author.user_id = art.author_id AND art_author.status = 1
	WHERE ` + articleVisible + ` AND %s)`

const feedOrder = " ORDER BY create_time DESC, item_type DESC, item_id DESC LIMIT ?"

// cursorCond 生成某别名的 keyset 游标条件（itemType 用于 tie-break 常量）
func cursorCond(timeCol, itemType, idCol string) string {
	return "(" + timeCol + " < ? OR (" + timeCol + " = ? AND ('" + itemType + "' < ? OR ('" + itemType + "' = ? AND " + idCol + " < ?))))"
}

// appendCursorArgs 追加 5 个游标参数
func appendCursorArgs(args []interface{}, t time.Time, itemType string, id int64) []interface{} {
	return append(args, t, t, itemType, itemType, id)
}

// int64sToArgs []int64 转 []interface{}
func int64sToArgs(ids []int64) []interface{} {
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	return args
}

// placeholders 生成 n 个 "?" 逗号连接
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// LatestFeedPage 最新 Feed：三类型统一 (create_time, item_type, item_id) 降序 keyset 分页
func LatestFeedPage(cursorTime time.Time, cursorType string, cursorID int64, limit int) (rows []*FeedRow, err error) {
	rows = make([]*FeedRow, 0, limit)
	qCond, aCond, artCond := "1 = 1", "1 = 1", "1 = 1"
	args := make([]interface{}, 0, 20)
	if cursorID > 0 {
		qCond = cursorCond("q.create_time", "question", "q.question_id")
		aCond = cursorCond("a.create_time", "answer", "a.answer_id")
		artCond = cursorCond("art.create_time", "article", "art.article_id")
		args = append(args, appendCursorArgs(appendCursorArgs(appendCursorArgs(
			make([]interface{}, 0, 16), cursorTime, "question", cursorID), cursorTime, "answer", cursorID), cursorTime, "article", cursorID)...)
	}
	query := fmt.Sprintf(feedUnionSQL, qCond, aCond, artCond) + feedOrder
	args = append(args, limit)
	err = db.Select(&rows, query, args...)
	return
}

// FollowingFeedPageParams 关注 Feed 多来源参数
type FollowingFeedPageParams struct {
	AuthorIDs  []int64 // 关注的作者（命中问题/回答/文章）
	TopicIDs   []int64 // 关注的话题（命中问题）
	ColumnIDs  []int64 // 关注的专栏（命中文章）
	CursorTime time.Time
	CursorType string
	CursorID   int64
	Limit      int
}

// FollowingFeedPage 关注 Feed 多来源（规格 4.2）：
//   作者关注 → 问题/回答/文章；话题关注 → 问题；专栏关注 → 文章。
//   同段用 OR 合并（单行不重复），跨段类型不同无重复；屏蔽与治理过滤在段外由调用方执行。
//   参数顺序与 SQL 占位符严格对应。
func FollowingFeedPage(p *FollowingFeedPageParams) (rows []*FeedRow, err error) {
	limit := p.Limit
	hasCursor := p.CursorID > 0

	// 各段条件构造（分桶参数见下方，保证与占位符顺序一致）
	qParts := make([]string, 0)
	artParts := make([]string, 0)

	// —— question 段 ——
	if len(p.AuthorIDs) > 0 {
		qParts = append(qParts, "q.author_id IN ("+placeholders(len(p.AuthorIDs))+")")
	}
	if len(p.TopicIDs) > 0 {
		qParts = append(qParts, "EXISTS (SELECT 1 FROM question_topic qt WHERE qt.question_id = q.question_id AND qt.topic_id IN ("+placeholders(len(p.TopicIDs))+"))")
	}
	qCond := "1 = 1"
	if len(qParts) > 0 {
		qCond = "(" + strings.Join(qParts, " OR ") + ")"
	}
	if hasCursor && len(qParts) > 0 {
		qCond += " AND " + cursorCond("q.create_time", "question", "q.question_id")
	}

	// —— answer 段 ——
	aCond := "1 = 1"
	if len(p.AuthorIDs) > 0 {
		aCond = "a.author_id IN (" + placeholders(len(p.AuthorIDs)) + ")"
		if hasCursor {
			aCond += " AND " + cursorCond("a.create_time", "answer", "a.answer_id")
		}
	}

	// —— article 段 ——
	if len(p.AuthorIDs) > 0 {
		artParts = append(artParts, "art.author_id IN ("+placeholders(len(p.AuthorIDs))+")")
	}
	if len(p.ColumnIDs) > 0 {
		artParts = append(artParts, "art.column_id IN ("+placeholders(len(p.ColumnIDs))+")")
	}
	artCond := "1 = 1"
	if len(artParts) > 0 {
		artCond = "(" + strings.Join(artParts, " OR ") + ")"
		if hasCursor {
			artCond += " AND " + cursorCond("art.create_time", "article", "art.article_id")
		}
	} else if hasCursor && len(qParts) > 0 {
		// question 段有游标而 article 段无过滤条件：article 段仍需游标条件避免返回超窗数据
		artCond = cursorCond("art.create_time", "article", "art.article_id")
	}

	// 分桶构造参数（与占位符顺序严格一致）：q 段、a 段、art 段、limit
	args := make([]interface{}, 0, 64)
	// q 段
	if len(p.AuthorIDs) > 0 {
		args = append(args, int64sToArgs(p.AuthorIDs)...)
	}
	if len(p.TopicIDs) > 0 {
		args = append(args, int64sToArgs(p.TopicIDs)...)
	}
	if hasCursor && (len(qParts) > 0 || len(p.TopicIDs) > 0) {
		args = append(args, p.CursorTime, p.CursorTime, p.CursorType, p.CursorType, p.CursorID)
	}
	// a 段
	if len(p.AuthorIDs) > 0 {
		args = append(args, int64sToArgs(p.AuthorIDs)...)
		if hasCursor {
			args = append(args, p.CursorTime, p.CursorTime, p.CursorType, p.CursorType, p.CursorID)
		}
	}
	// art 段
	if len(artParts) > 0 {
		if len(p.AuthorIDs) > 0 {
			args = append(args, int64sToArgs(p.AuthorIDs)...)
		}
		if len(p.ColumnIDs) > 0 {
			args = append(args, int64sToArgs(p.ColumnIDs)...)
		}
		if hasCursor {
			args = append(args, p.CursorTime, p.CursorTime, p.CursorType, p.CursorType, p.CursorID)
		}
	}
	args = append(args, limit)

	query := fmt.Sprintf(feedUnionSQL, qCond, aCond, artCond) + feedOrder
	err = db.Select(&rows, query, args...)
	if err != nil {
		zap.L().Error("following feed query failed", zap.Error(err))
		return nil, err
	}
	return rows, nil
}

// HotFeedCandidates 热榜候选：近 30 天三类内容，按互动数粗筛
func HotFeedCandidates(limit int) (rows []*FeedRow, err error) {
	rows = make([]*FeedRow, 0, limit)
	since := time.Now().AddDate(0, 0, -30)
	query := fmt.Sprintf(feedUnionSQL, "q.create_time > ?", "a.create_time > ?", "art.create_time > ?") +
		" ORDER BY (vote_count * 2 + comment_count * 3 + collect_count * 4) DESC LIMIT ?"
	args := []interface{}{since, since, since, limit}
	err = db.Select(&rows, query, args...)
	if err != nil {
		zap.L().Error("query hot feed candidates failed", zap.Error(err))
		return nil, err
	}
	return rows, nil
}
