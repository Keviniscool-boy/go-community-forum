package mysql

// 问答解决闭环（迁移 014，产品审计 PRD §4.5）
// 采纳/撤销/关闭/重开全部使用「锁定问题行 → 校验 → CAS 更新 → 通知/审计」单事务，
// 并发采纳同一问题的不同答案时只有一个成功（后到者拿到行锁后发现版本已推进 → 冲突）。

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"bluebell/models"
	"bluebell/pkg/snowflake"

	"github.com/jmoiron/sqlx"
)

var (
	// ErrResolutionConflict 采纳/状态在并发窗口内被其他人修改（版本 CAS 失败）
	ErrResolutionConflict = errors.New("问题状态已变化，请刷新后重试")
	// ErrQuestionClosedState 问题已关闭，不允许采纳
	ErrQuestionClosed = errors.New("问题已关闭，无法采纳回答")
	// ErrAnswerNotAcceptable 回答不属于该问题、不可见或作者不可用
	ErrAnswerNotAcceptable = errors.New("该回答不可采纳")
	// ErrNotQuestionAuthor 仅问题作者可采纳/撤销/关闭/重开
	ErrNotQuestionAuthor = errors.New("仅问题作者可执行该操作")
	// ErrResolutionTransitionNotAllowed open/closed 之外的状态转换不允许（solved 由采纳维护）
	ErrResolutionTransitionNotAllowed = errors.New("该状态转换不允许")
)

// questionResolutionRow FOR UPDATE 读取的问题解决状态行
type questionResolutionRow struct {
	QuestionID       int64        `db:"question_id"`
	AuthorID         int64        `db:"author_id"`
	Status           int32        `db:"status"`
	ResolutionStatus int32        `db:"resolution_status"`
	AcceptedAnswerID sql.NullInt64 `db:"accepted_answer_id"`
	Version          int64        `db:"version"`
}

const questionResolutionColumns = `question_id, author_id, status, resolution_status, accepted_answer_id, version`

// lockQuestionResolution 事务内锁定问题行（行锁串行化并发采纳）
func lockQuestionResolution(tx *sqlx.Tx, questionID int64) (*questionResolutionRow, error) {
	q := new(questionResolutionRow)
	err := tx.Get(q, "SELECT "+questionResolutionColumns+" FROM question WHERE question_id = ? FOR UPDATE", questionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrQuestionNotExist
		}
		return nil, err
	}
	return q, nil
}

// txAudit 事务内写审计（与业务同事务，保证审计不丢）
func txAudit(tx *sqlx.Tx, actorID int64, action, targetType, targetID, requestID, metadata string) error {
	_, err := tx.Exec(
		"INSERT INTO audit_log (audit_id, actor_id, action, target_type, target_id, request_id, ip_hash, metadata_json) VALUES (?, ?, ?, ?, ?, ?, '', CAST(? AS JSON))",
		snowflake.GenerateID(), actorID, action, targetType, targetID, requestID, metadata)
	return err
}

// AcceptAnswerAtomic 原子采纳回答（仅问题作者；同问题唯一有效采纳；幂等）
// 返回 nil 表示成功（含幂等重试：已采纳同一回答直接返回成功，不重复通知）。
func AcceptAnswerAtomic(questionID, answerID, actorID int64, requestID string) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	q, err := lockQuestionResolution(tx, questionID)
	if err != nil {
		return err
	}
	if q.Status != 1 {
		return ErrQuestionNotExist
	}
	if q.AuthorID != actorID {
		return ErrNotQuestionAuthor
	}
	if q.AcceptedAnswerID.Valid && q.AcceptedAnswerID.Int64 == answerID {
		return nil // 幂等：已采纳同一回答
	}
	if q.AcceptedAnswerID.Valid && q.AcceptedAnswerID.Int64 != 0 {
		// 已有其他采纳：必须先撤销（撤销→再采纳两个显式步骤，保证并发语义可测试）
		return ErrResolutionConflict
	}
	if q.ResolutionStatus == models.QuestionResolutionClosed {
		return ErrQuestionClosed
	}
	// 校验回答：属于本问题、公开可见、作者账号有效（FOR UPDATE 下校验，采纳瞬间成立）
	var answerAuthor int64
	err = tx.Get(&answerAuthor,
		`SELECT a.author_id FROM answer a
		 JOIN user u ON u.user_id = a.author_id AND u.status = 1
		 WHERE a.answer_id = ? AND a.question_id = ? AND a.status = 1
		   AND a.moderation_status = 1 AND a.visibility = 1 AND a.deleted_at IS NULL`, answerID, questionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAnswerNotAcceptable
		}
		return err
	}
	// CAS 更新：版本不符说明并发修改（理论上已被行锁串行化，仍以防万一）
	result, err := tx.Exec(
		"UPDATE question SET accepted_answer_id = ?, resolution_status = 1, version = version + 1 WHERE question_id = ? AND version = ?",
		answerID, questionID, q.Version)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrResolutionConflict
	}
	// 问题已解决：待处理邀请失效（PRD §3 关闭或解决不允许新邀请）
	if err = ExpireQuestionInvitationsTx(tx, questionID); err != nil {
		return err
	}
	// 通知回答作者（同事务；自己采纳自己的回答不通知）
	if err = txCreateNotification(tx, answerAuthor, actorID, models.NotifyTypeAccept, models.TargetQuestion, questionID); err != nil {
		return err
	}
	if err = txAudit(tx, actorID, "answer_accept", "answer",
		int64ToStr(answerID), requestID, `{"question_id":"`+int64ToStr(questionID)+`"}`); err != nil {
		return err
	}
	return tx.Commit()
}

// UnacceptAnswerAtomic 撤销采纳（仅问题作者；幂等：未采纳该回答时直接成功）
func UnacceptAnswerAtomic(questionID, answerID, actorID int64, requestID string) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	q, err := lockQuestionResolution(tx, questionID)
	if err != nil {
		return err
	}
	if q.Status != 1 {
		return ErrQuestionNotExist
	}
	if q.AuthorID != actorID {
		return ErrNotQuestionAuthor
	}
	if !q.AcceptedAnswerID.Valid || q.AcceptedAnswerID.Int64 != answerID {
		return nil // 幂等：当前采纳的不是该回答（或无采纳）
	}
	var answerAuthor int64
	if err = tx.Get(&answerAuthor,
		"SELECT author_id FROM answer WHERE answer_id = ? AND question_id = ?", answerID, questionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAnswerNotAcceptable
		}
		return err
	}
	result, err := tx.Exec(
		"UPDATE question SET accepted_answer_id = NULL, resolution_status = 0, version = version + 1 WHERE question_id = ? AND accepted_answer_id = ? AND version = ?",
		questionID, answerID, q.Version)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrResolutionConflict
	}
	if err = txCreateNotification(tx, answerAuthor, actorID, models.NotifyTypeUnaccept, models.TargetQuestion, questionID); err != nil {
		return err
	}
	if err = txAudit(tx, actorID, "answer_unaccept", "answer",
		int64ToStr(answerID), requestID, `{"question_id":"`+int64ToStr(questionID)+`"}`); err != nil {
		return err
	}
	return tx.Commit()
}

// SetQuestionResolutionStatusAtomic open/closed 状态转换（作者或管理员；CAS 条件更新）
func SetQuestionResolutionStatusAtomic(questionID, actorID int64, from, to int32, isAdmin bool, requestID string) error {
	if (from == models.QuestionResolutionOpen && to == models.QuestionResolutionClosed) ||
		(from == models.QuestionResolutionClosed && to == models.QuestionResolutionOpen) {
		// 合法转换
	} else {
		return ErrResolutionTransitionNotAllowed
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	q, err := lockQuestionResolution(tx, questionID)
	if err != nil {
		return err
	}
	if q.Status != 1 {
		return ErrQuestionNotExist
	}
	if q.AuthorID != actorID && !isAdmin {
		return ErrNotQuestionAuthor
	}
	// 当前状态与来源状态不符（如 solved 上直接关闭）→ 转换不允许，非并发冲突
	if q.ResolutionStatus != from {
		return ErrResolutionTransitionNotAllowed
	}
	result, err := tx.Exec(
		"UPDATE question SET resolution_status = ?, version = version + 1 WHERE question_id = ? AND resolution_status = ? AND version = ?",
		to, questionID, from, q.Version)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrResolutionConflict
	}
	// 关闭/解决后不允许新邀请：待处理邀请失效（PRD §3）
	if to == models.QuestionResolutionClosed {
		if err = ExpireQuestionInvitationsTx(tx, questionID); err != nil {
			return err
		}
	}
	action := "question_close"
	if to == models.QuestionResolutionOpen {
		action = "question_reopen"
	}
	if err = txAudit(tx, actorID, action, "question",
		int64ToStr(questionID), requestID, "{}"); err != nil {
		return err
	}
	return tx.Commit()
}

// clearAcceptanceLocked 在已持有问题行锁的事务内清空采纳并恢复 open。
// 返回是否实际清空；由回答删除/下架/禁用路径复用。
func clearAcceptanceLocked(tx *sqlx.Tx, questionID int64, version int64, reason, requestID string, operatorID int64) (bool, error) {
	result, err := tx.Exec(
		"UPDATE question SET accepted_answer_id = NULL, resolution_status = 0, version = version + 1 WHERE question_id = ? AND version = ? AND accepted_answer_id IS NOT NULL",
		questionID, version)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return false, nil
	}
	if err = txAudit(tx, operatorID, "acceptance_auto_clear", "question",
		int64ToStr(questionID), requestID, `{"reason":"`+reason+`"}`); err != nil {
		return false, err
	}
	return true, nil
}

// ClearAcceptanceByAnswer 回答被删除/下架/隐藏后自动清空采纳（独立事务；幂等）。
// reason 记录进审计 metadata（answer_deleted / moderation / visibility / author_disabled）。
func ClearAcceptanceByAnswer(answerID int64, reason, requestID string, operatorID int64) (cleared bool, err error) {
	tx, err := db.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	q := new(questionResolutionRow)
	err = tx.Get(q, "SELECT "+questionResolutionColumns+" FROM question WHERE accepted_answer_id = ? FOR UPDATE", answerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, tx.Commit() // 该回答未被采纳：幂等成功
		}
		return false, err
	}
	cleared, err = clearAcceptanceLocked(tx, q.QuestionID, q.Version, reason, requestID, operatorID)
	if err != nil || !cleared {
		return cleared, err
	}
	return true, tx.Commit()
}

// ClearAcceptancesForAuthor 作者被禁用时清空其全部有效采纳（独立事务；审计汇总）
func ClearAcceptancesForAuthor(authorID int64, requestID string, operatorID int64) (cleared int64, err error) {
	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var questionIDs []int64
	if err = tx.Select(&questionIDs,
		`SELECT q.question_id FROM question q
		 JOIN answer a ON a.answer_id = q.accepted_answer_id AND a.author_id = ?
		 WHERE q.accepted_answer_id IS NOT NULL FOR UPDATE`, authorID); err != nil {
		return 0, err
	}
	if len(questionIDs) == 0 {
		return 0, tx.Commit()
	}
	result, err := tx.Exec(
		`UPDATE question q JOIN answer a ON a.answer_id = q.accepted_answer_id AND a.author_id = ?
		 SET q.accepted_answer_id = NULL, q.resolution_status = 0, q.version = q.version + 1
		 WHERE q.accepted_answer_id IS NOT NULL`, authorID)
	if err != nil {
		return 0, err
	}
	cleared, _ = result.RowsAffected()
	if cleared > 0 {
		meta, _ := json.Marshal(map[string]interface{}{"reason": "author_disabled", "cleared_question_ids": questionIDs})
		if err = txAudit(tx, operatorID, "acceptance_auto_clear", "user",
			int64ToStr(authorID), requestID, string(meta)); err != nil {
			return 0, err
		}
	}
	return cleared, tx.Commit()
}

// int64ToStr 雪花 id 转字符串（审计 target_id 为字符串列）
func int64ToStr(v int64) string {
	return strconv.FormatInt(v, 10)
}
