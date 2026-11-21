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
		   FROM answer WHERE answer_id = ? AND status = 1
		   AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL`
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
	// AcceptedAnswerID 采纳回答（>0 时从游标流中排除并在首页置顶返回，
	// 保证采纳置顶与 keyset 分页不重复不遗漏）
	AcceptedAnswerID int64
}

// answerVisibleCols 回答列表统一查询列
const answerVisibleCols = `answer_id, question_id, author_id, content, vote_count,
		   comment_count, collect_count, status, create_time, update_time`

// ListAnswersByQuestion 按 cursor 分页查询问题的回答
// latest: create_time DESC, answer_id DESC；hot: vote_count DESC, answer_id DESC
// 采纳回答始终排在第一页首位（从游标流排除，翻页不重复出现）
func ListAnswersByQuestion(p *AnswerPageParams) (answers []*models.Answer, err error) {
	answers = make([]*models.Answer, 0)
	zeroCursor := p.CursorID == 0
	limit := p.Limit

	excludeAccepted := ""
	argsBase := []interface{}{p.QuestionID}
	if p.AcceptedAnswerID > 0 {
		excludeAccepted = " AND answer_id != ?"
		argsBase = append(argsBase, p.AcceptedAnswerID)
		if zeroCursor {
			// 首页给采纳回答留一个位置（has_more 由下方 limit+1 判定）
			limit--
		}
	}
	if limit <= 0 {
		// 首页只有采纳回答一条时仍需探测 has_more
		limit = 1
	}
	fetchLimit := limit + 1 // 多取一条判断 has_more

	var query string
	var args []interface{}
	if p.Order == models.AnswerOrderHot {
		if zeroCursor {
			query = `SELECT ` + answerVisibleCols + ` FROM answer
				 WHERE question_id = ? AND status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL` + excludeAccepted + `
				 ORDER BY vote_count DESC, answer_id DESC LIMIT ?`
			args = append(argsBase, fetchLimit)
		} else {
			query = `SELECT ` + answerVisibleCols + ` FROM answer
				 WHERE question_id = ? AND status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL` + excludeAccepted + `
				 AND (vote_count < ? OR (vote_count = ? AND answer_id < ?))
				 ORDER BY vote_count DESC, answer_id DESC LIMIT ?`
			args = append(argsBase, p.CursorVote, p.CursorVote, p.CursorID, fetchLimit)
		}
	} else {
		if zeroCursor {
			query = `SELECT ` + answerVisibleCols + ` FROM answer
				 WHERE question_id = ? AND status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL` + excludeAccepted + `
				 ORDER BY create_time DESC, answer_id DESC LIMIT ?`
			args = append(argsBase, fetchLimit)
		} else {
			query = `SELECT ` + answerVisibleCols + ` FROM answer
				 WHERE question_id = ? AND status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL` + excludeAccepted + `
				 AND (create_time < ? OR (create_time = ? AND answer_id < ?))
				 ORDER BY create_time DESC, answer_id DESC LIMIT ?`
			args = append(argsBase, p.CursorTime, p.CursorTime, p.CursorID, fetchLimit)
		}
	}
	err = db.Select(&answers, query, args...)
	if err != nil {
		zap.L().Error("query answers failed", zap.Error(err))
		return nil, err
	}
	// 首页把采纳回答插到最前
	if p.AcceptedAnswerID > 0 && zeroCursor {
		accepted := new(models.Answer)
		err = db.Get(accepted,
			"SELECT "+answerVisibleCols+" FROM answer WHERE answer_id = ? AND question_id = ? AND status = 1"+
				" AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL",
			p.AcceptedAnswerID, p.QuestionID)
		if err == nil {
			answers = append([]*models.Answer{accepted}, answers...)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return answers, nil
}

// RecountAnswerCountByAnswer 按回答 id 重算所属问题的公开回答数（派生计数对账用）
func RecountAnswerCountByAnswer(answerID int64) error {
	_, err := db.Exec(
		"UPDATE question SET answer_count = (SELECT COUNT(*) FROM answer WHERE answer.question_id = question.question_id"+
			" AND status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL)"+
			" WHERE question_id = (SELECT question_id FROM answer WHERE answer_id = ?)", answerID)
	return err
}

// CountAnswersByQuestion 查询问题的公开回答总数（口径与列表一致：隔离/隐藏/已删除不计入）
func CountAnswersByQuestion(questionID int64) (int, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(answer_id) FROM answer WHERE question_id = ? AND status = 1"+
		" AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL", questionID)
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

// SoftDeleteAnswer 作者软删除回答，并以关系表为准重算问题回答数。
// 若该回答是采纳答案：同一事务内清空采纳并恢复问题为 open（审计事件记录），避免悬空采纳。
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
		"UPDATE question SET answer_count = (SELECT COUNT(*) FROM answer WHERE answer.question_id = question.question_id"+
			" AND status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL)"+
			" WHERE question_id = (SELECT question_id FROM answer WHERE answer_id = ?)", answerID); err != nil {
		return false, err
	}
	// 删除采纳答案 → 清空采纳关系并恢复 open（幂等：未被采纳时无行受影响）
	q := new(questionResolutionRow)
	err = tx.Get(q, "SELECT "+questionResolutionColumns+" FROM question WHERE accepted_answer_id = ? FOR UPDATE", answerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil {
		if _, err = clearAcceptanceLocked(tx, q.QuestionID, q.Version, "answer_deleted", "", authorID); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}
