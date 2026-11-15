package mysql

import (
	"database/sql"
	"errors"

	"bluebell/models"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

var ErrQuestionNotExist = errors.New("问题不存在")

// CreateQuestion 创建问题及话题关联
func CreateQuestion(q *models.Question, topicIDs []int64) error {
	// 问题和关联表在同个事务内写入，保证不出现半截数据
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(
		"INSERT INTO question (question_id, title, description, author_id) VALUES (?, ?, ?, ?)",
		q.ID, q.Title, q.Description, q.AuthorID); err != nil {
		return err
	}
	for _, tid := range topicIDs {
		if _, err = tx.Exec(
			"INSERT IGNORE INTO question_topic (question_id, topic_id) VALUES (?, ?)",
			q.ID, tid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetQuestionByID 根据问题id查询问题详情
func GetQuestionByID(id int64) (q *models.Question, err error) {
	q = new(models.Question)
	sqlStr := `SELECT question_id, title, description, author_id, answer_count,
		      follower_count, view_count, status, create_time
		      FROM question WHERE question_id = ? AND status = 1`
	if err = db.Get(q, sqlStr, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			zap.L().Warn("question not found", zap.Int64("id", id))
			return nil, ErrQuestionNotExist
		}
		return nil, err
	}
	return q, nil
}

// GetQuestionTopics 查询问题关联的话题列表
func GetQuestionTopics(questionID int64) (topics []*models.TopicBrief, err error) {
	topics = make([]*models.TopicBrief, 0)
	sqlStr := `SELECT t.topic_id, t.name
		   FROM question_topic qt
		   JOIN topic t ON t.topic_id = qt.topic_id
		   WHERE qt.question_id = ?`
	err = db.Select(&topics, sqlStr, questionID)
	return
}

// IncrementQuestionViewCount 问题浏览量自增
func IncrementQuestionViewCount(questionID int64) error {
	_, err := db.Exec("UPDATE question SET view_count = view_count + 1 WHERE question_id = ?", questionID)
	return err
}

// GetQuestionsByIDs 按 id 列表批量查询问题（Feed 组装用）
func GetQuestionsByIDs(ids []int64) (questions []*models.Question, err error) {
	if len(ids) == 0 {
		return make([]*models.Question, 0), nil
	}
	questions = make([]*models.Question, 0, len(ids))
	query, args, err := sqlx.In(`SELECT question_id, title, description, author_id, answer_count,
		follower_count, view_count, status, create_time
		FROM question WHERE question_id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	err = db.Select(&questions, query, args...)
	return
}

// LatestQuestionIDsForFeed Feed 兜底：按时间倒序取最新问题 id（M4 使用）
func LatestQuestionIDsForFeed(limit int) (ids []int64, err error) {
	ids = make([]int64, 0, limit)
	err = db.Select(&ids, "SELECT question_id FROM question WHERE status = 1 ORDER BY create_time DESC LIMIT ?", limit)
	return
}
