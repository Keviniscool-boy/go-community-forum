package mysql

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// FeedRow Feed 查询的中间行：问题行与回答行统一结构
// 问题行：item_type=question，title=问题标题，excerpt_src=描述，answer_count 有效
// 回答行：item_type=answer，title=所属问题标题，excerpt_src=回答内容，vote/comment/collect 有效
type FeedRow struct {
	ItemType     string    `db:"item_type"`
	ItemID       int64     `db:"item_id"`
	QuestionID   int64     `db:"question_id"`
	AuthorID     int64     `db:"author_id"`
	Title        string    `db:"title"`
	ExcerptSrc   string    `db:"excerpt_src"`
	AnswerCount  int32     `db:"answer_count"`
	VoteCount    int32     `db:"vote_count"`
	CommentCount int32     `db:"comment_count"`
	CollectCount int32     `db:"collect_count"`
	CreateTime   time.Time `db:"create_time"`
}

// feedUnionSQL 问题 + 回答的 UNION 查询主体（whereConds 按别名列拼接）
// 公开可见条件统一为：status=1 AND moderation_status=1 AND visibility=1 AND deleted_at IS NULL
// 且作者账号未被禁用；Feed 只展示公开状态内容（规格 3.3）
const feedUnionSQL = `(SELECT 'question' AS item_type, q.question_id AS item_id, q.question_id AS question_id,
		q.author_id AS author_id, q.title AS title, q.description AS excerpt_src,
		q.answer_count AS answer_count, 0 AS vote_count, q.comment_count AS comment_count, 0 AS collect_count,
		q.create_time AS create_time
	FROM question q JOIN user q_author ON q_author.user_id = q.author_id AND q_author.status = 1
	WHERE q.status = 1 AND q.moderation_status = 1 AND q.visibility = 1 AND q.deleted_at IS NULL AND %s)
UNION ALL
(SELECT 'answer' AS item_type, a.answer_id AS item_id, a.question_id AS question_id,
		a.author_id AS author_id, qu.title AS title, a.content AS excerpt_src,
		0 AS answer_count, a.vote_count AS vote_count, a.comment_count AS comment_count, a.collect_count AS collect_count,
		a.create_time AS create_time
	FROM answer a
	JOIN question qu ON qu.question_id = a.question_id AND qu.status = 1
		AND qu.moderation_status = 1 AND qu.visibility = 1 AND qu.deleted_at IS NULL
	JOIN user a_author ON a_author.user_id = a.author_id AND a_author.status = 1
	WHERE a.status = 1 AND a.moderation_status = 1 AND a.visibility = 1 AND a.deleted_at IS NULL AND %s)`

// int64sToArgs []int64 转 []interface{}
func int64sToArgs(ids []int64) []interface{} {
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	return args
}

// LatestFeedPage 最新 Feed：按 (create_time, item_type, item_id) 降序 cursor 分页
// WHERE 的游标比较与 ORDER BY 使用同一字典序；item_type 保证跨类型行也全序
func LatestFeedPage(cursorTime time.Time, cursorType string, cursorID int64, limit int) (rows []*FeedRow, err error) {
	rows = make([]*FeedRow, 0, limit)
	qCond := "1 = 1"
	aCond := "1 = 1"
	args := make([]interface{}, 0, 12)
	if cursorID > 0 {
		qCond = "(q.create_time < ? OR (q.create_time = ? AND ('question' < ? OR ('question' = ? AND q.question_id < ?))))"
		aCond = "(a.create_time < ? OR (a.create_time = ? AND ('answer' < ? OR ('answer' = ? AND a.answer_id < ?))))"
		args = append(args, cursorTime, cursorTime, cursorType, cursorType, cursorID,
			cursorTime, cursorTime, cursorType, cursorType, cursorID)
	}
	query := fmt.Sprintf(feedUnionSQL, qCond, aCond) + " ORDER BY create_time DESC, item_type DESC, item_id DESC LIMIT ?"
	args = append(args, limit)
	err = db.Select(&rows, query, args...)
	return
}

// FollowingFeedPage 关注 Feed：只取关注用户的内容，按 (create_time, item_type, item_id) 降序 cursor 分页
// 参数顺序必须与 SQL 占位符顺序一致：
//   第一段 SELECT：q.author_id IN (...) → 游标 5 个；
//   第二段 SELECT：a.author_id IN (...) → 游标 5 个
func FollowingFeedPage(authorIDs []int64, cursorTime time.Time, cursorType string, cursorID int64, limit int) (rows []*FeedRow, err error) {
	rows = make([]*FeedRow, 0, limit)
	// IN 占位符
	placeholders := ""
	for i := range authorIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}
	qCond := "q.author_id IN (" + placeholders + ")"
	aCond := "a.author_id IN (" + placeholders + ")"
	if cursorID > 0 {
		qCond += " AND (q.create_time < ? OR (q.create_time = ? AND ('question' < ? OR ('question' = ? AND q.question_id < ?))))"
		aCond += " AND (a.create_time < ? OR (a.create_time = ? AND ('answer' < ? OR ('answer' = ? AND a.answer_id < ?))))"
	}
	// 排序与游标使用同一字典序：(create_time DESC, item_type DESC, item_id DESC)，keyset 分页不漏不重
	query := fmt.Sprintf(feedUnionSQL, qCond, aCond) + " ORDER BY create_time DESC, item_type DESC, item_id DESC LIMIT ?"

	args := make([]interface{}, 0, len(authorIDs)*2+12)
	// 第一段：作者 IN + 游标
	args = append(args, int64sToArgs(authorIDs)...)
	if cursorID > 0 {
		args = append(args, cursorTime, cursorTime, cursorType, cursorType, cursorID)
	}
	// 第二段：作者 IN + 游标
	args = append(args, int64sToArgs(authorIDs)...)
	if cursorID > 0 {
		args = append(args, cursorTime, cursorTime, cursorType, cursorType, cursorID)
	}
	args = append(args, limit)
	err = db.Select(&rows, query, args...)
	return
}

// HotFeedCandidates 热榜候选：近 30 天问题+回答，按互动数粗筛
func HotFeedCandidates(limit int) (rows []*FeedRow, err error) {
	rows = make([]*FeedRow, 0, limit)
	since := time.Now().AddDate(0, 0, -30)
	query := fmt.Sprintf(feedUnionSQL, "q.create_time > ?", "a.create_time > ?") +
		" ORDER BY (vote_count * 2 + comment_count * 3 + collect_count * 4) DESC LIMIT ?"
	args := []interface{}{since, since, limit}
	err = db.Select(&rows, query, args...)
	if err != nil {
		zap.L().Error("query hot feed candidates failed", zap.Error(err))
		return nil, err
	}
	return rows, nil
}

// GetFeedRowsByIDs 按 id 批量取 Feed 行（推荐流从 Redis 拿到 id 后回查）
func GetFeedRowsByIDs(typeIDs map[string][]int64) (rows []*FeedRow, err error) {
	rows = make([]*FeedRow, 0)
	if len(typeIDs) == 0 {
		return rows, nil
	}
	if qids := typeIDs["question"]; len(qids) > 0 {
		qrows, err := queryFeedRows("question", qids)
		if err != nil {
			return nil, err
		}
		rows = append(rows, qrows...)
	}
	if aids := typeIDs["answer"]; len(aids) > 0 {
		arows, err := queryFeedRows("answer", aids)
		if err != nil {
			return nil, err
		}
		rows = append(rows, arows...)
	}
	return rows, nil
}

func queryFeedRows(itemType string, ids []int64) (rows []*FeedRow, err error) {
	rows = make([]*FeedRow, 0, len(ids))
	placeholders := ""
	for i := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}
	var cond string
	if itemType == "question" {
		cond = "q.question_id IN (" + placeholders + ")"
	} else {
		cond = "a.answer_id IN (" + placeholders + ")"
	}
	query := fmt.Sprintf(feedUnionSQL, cond, cond) + " ORDER BY create_time DESC, item_id DESC"
	// UNION ALL 两侧条件都带 IN，参数翻倍
	args := make([]interface{}, 0, len(ids)*2)
	args = append(args, int64sToArgs(ids)...)
	args = append(args, int64sToArgs(ids)...)
	err = db.Select(&rows, query, args...)
	return
}
