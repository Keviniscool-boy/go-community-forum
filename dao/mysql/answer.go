package mysql

import (
	"database/sql"
	"errors"
	"time"

	"bluebell/models"

	"go.uber.org/zap"
)

var ErrAnswerNotExist = errors.New("回答不存在")

// CreateAnswer 创建回答并同步累加问题的回答数（同一事务；moderationStatus 由发布审核流决定）
func CreateAnswer(a *models.Answer, moderationStatus int32) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(
		"INSERT INTO answer (answer_id, question_id, author_id, content, moderation_status) VALUES (?, ?, ?, ?, ?)",
		a.ID, a.QuestionID, a.AuthorID, a.Content, moderationStatus); err != nil {
		return err
	}
	if _, err = tx.Exec(
		"UPDATE question SET answer_count = answer_count + 1 WHERE question_id = ?", a.QuestionID); err != nil {
		return err
	}
	return tx.Commit()
}

// GetAnswerByID 根据 id 查询单条回答
func GetAnswerByID(id int64) (a *models.Answer, err error) {
	a = new(models.Answer)
	sqlStr := `SELECT answer_id, question_id, author_id, content, vote_count,
		   comment_count, collect_count, status, create_time
		   FROM answer WHERE answer_id = ? AND status = 1`
	if err = db.Get(a, sqlStr, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAnswerNotExist
		}
		return nil, err
	}
	return a, nil
}

// AnswerPageParams 回答列表查询参数（cursor 分页）
type AnswerPageParams struct {
	QuestionID int64
	Order      string // hot | latest
	CursorTime time.Time
	CursorID   int64
	CursorVote int32
	Limit      int
}

// ListAnswersByQuestion 按 cursor 分页查询问题的回答
// latest: create_time DESC, answer_id DESC；hot: vote_count DESC, answer_id DESC
func ListAnswersByQuestion(p *AnswerPageParams) (answers []*models.Answer, err error) {
	answers = make([]*models.Answer, 0)
	zeroCursor := p.CursorID == 0
	limit := p.Limit + 1 // 多取一条判断 has_more

	var query string
	var args []interface{}
	if p.Order == models.AnswerOrderHot {
		if zeroCursor {
			query = `SELECT answer_id, question_id, author_id, content, vote_count,
				 comment_count, collect_count, status, create_time
				 FROM answer WHERE question_id = ? AND status = 1
				 ORDER BY vote_count DESC, answer_id DESC LIMIT ?`
			args = []interface{}{p.QuestionID, limit}
		} else {
			query = `SELECT answer_id, question_id, author_id, content, vote_count,
				 comment_count, collect_count, status, create_time
				 FROM answer WHERE question_id = ? AND status = 1
				 AND (vote_count < ? OR (vote_count = ? AND answer_id < ?))
				 ORDER BY vote_count DESC, answer_id DESC LIMIT ?`
			args = []interface{}{p.QuestionID, p.CursorVote, p.CursorVote, p.CursorID, limit}
		}
	} else {
		if zeroCursor {
			query = `SELECT answer_id, question_id, author_id, content, vote_count,
				 comment_count, collect_count, status, create_time
				 FROM answer WHERE question_id = ? AND status = 1
				 ORDER BY create_time DESC, answer_id DESC LIMIT ?`
			args = []interface{}{p.QuestionID, limit}
		} else {
			query = `SELECT answer_id, question_id, author_id, content, vote_count,
				 comment_count, collect_count, status, create_time
				 FROM answer WHERE question_id = ? AND status = 1
				 AND (create_time < ? OR (create_time = ? AND answer_id < ?))
				 ORDER BY create_time DESC, answer_id DESC LIMIT ?`
			args = []interface{}{p.QuestionID, p.CursorTime, p.CursorTime, p.CursorID, limit}
		}
	}
	err = db.Select(&answers, query, args...)
	if err != nil {
		zap.L().Error("query answers failed", zap.Error(err))
		return nil, err
	}
	return answers, nil
}

// CountAnswersByQuestion 查询问题的回答总数
func CountAnswersByQuestion(questionID int64) (int, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(answer_id) FROM answer WHERE question_id = ? AND status = 1", questionID)
	return count, err
}

// UpdateAnswer 作者编辑回答
func UpdateAnswer(answerID, authorID int64, content string) (bool, error) {
	result, err := db.Exec(
		"UPDATE answer SET content = ? WHERE answer_id = ? AND author_id = ? AND status = 1",
		content, answerID, authorID)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// SoftDeleteAnswer 作者软删除回答，并以关系表为准重算问题回答数
func SoftDeleteAnswer(answerID, authorID int64) (bool, error) {
	tx, err := db.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(
		"UPDATE answer SET status = 0 WHERE answer_id = ? AND author_id = ? AND status = 1",
		answerID, authorID)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return false, nil
	}
	if _, err = tx.Exec(
		"UPDATE question SET answer_count = (SELECT COUNT(*) FROM answer WHERE question_id = ? AND status = 1) "+
			"WHERE question_id = (SELECT question_id FROM answer WHERE answer_id = ?)", answerID, answerID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
