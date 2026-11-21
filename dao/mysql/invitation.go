package mysql

// 回答邀请 DAO（待回答问题发现与回答邀请 PRD §7/§8）
// 事务约定：发送（问题行锁 → 校验 → 逐人筛选 → 批量插入邀请/通知/审计）单事务；
// 接受/婉拒（问题行锁 → 邀请行锁，与发送同序，防死锁）单事务；
// 回答公开落地 → 邀请 pending/accepted → answered（与回答写入/审核同事务）。

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"bluebell/models"
	"bluebell/pkg/snowflake"

	drvmysql "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	ErrInvitationNotExist  = errors.New("邀请不存在")
	ErrInviteState         = errors.New("邀请已处理或已失效")
	ErrInviteNotReceiver   = errors.New("仅邀请接收者可以操作")
	ErrInviteQuestionState = errors.New("问题当前不可邀请")
)

// questionInviteRow 邀请场景下的问题行（FOR UPDATE 全字段）
type questionInviteRow struct {
	QuestionID       int64        `db:"question_id"`
	AuthorID         int64        `db:"author_id"`
	Status           int32        `db:"status"`
	ResolutionStatus int32        `db:"resolution_status"`
	ModerationStatus int32        `db:"moderation_status"`
	Visibility       int32        `db:"visibility"`
	DeletedAt        sql.NullTime `db:"deleted_at"`
}

const questionInviteColumns = `question_id, author_id, status, resolution_status, moderation_status, visibility, deleted_at`

// 邀请相关公共可见条件片段（与 search/feed 同口径）
const invitationQuestionVisible = `q.status = 1 AND q.moderation_status = 1 AND q.visibility = 1 AND q.deleted_at IS NULL`
// q2 别名版（候选子查询中问题表别名为 q2）
const invitationQuestionVisibleQ2 = `q2.status = 1 AND q2.moderation_status = 1 AND q2.visibility = 1 AND q2.deleted_at IS NULL`
const invitationAnswerVisible = `a.status = 1 AND a.moderation_status = 1 AND a.visibility = 1 AND a.deleted_at IS NULL`

// ---------- 待回答问题列表（BE-02） ----------

// UnansweredPageParams 待回答列表查询参数
type UnansweredPageParams struct {
	TopicID    int64  // 0 = 全部话题
	Sort       string // latest | oldest_unanswered
	CursorTime time.Time
	CursorID   int64
	Limit      int
}

// ListUnansweredQuestions 待回答问题：公开可见 + resolution=open + 无公开审核通过回答
// （PRD §3：是否有被隐藏/删除回答不影响资格 → 用 NOT EXISTS 公开回答判定，不依赖派生计数）
func ListUnansweredQuestions(p *UnansweredPageParams) (questions []*models.Question, err error) {
	questions = make([]*models.Question, 0)
	limit := p.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	fetchLimit := limit + 1

	// 稳定排序：(create_time, question_id)；latest 倒序 / oldest_unanswered 正序
	order := "q.create_time DESC, q.question_id DESC"
	cursorOp, cursorJoin := "<", ""
	if p.Sort == "oldest_unanswered" {
		order = "q.create_time ASC, q.question_id ASC"
		cursorOp = ">"
	}
	if p.CursorID > 0 {
		cursorJoin = fmt.Sprintf(" AND (q.create_time %s ? OR (q.create_time = ? AND q.question_id %s ?))", cursorOp, cursorOp)
	}

	topicJoin := ""
	args := []interface{}{}
	if p.TopicID > 0 {
		topicJoin = " JOIN question_topic qt ON qt.question_id = q.question_id AND qt.topic_id = ?"
		args = append(args, p.TopicID)
	}
	if p.CursorID > 0 {
		args = append(args, p.CursorTime, p.CursorTime, p.CursorID)
	}
	args = append(args, fetchLimit)

	query := `SELECT q.question_id, q.title, q.description, q.author_id, q.answer_count,
		q.comment_count, q.follower_count, q.view_count, q.status, q.resolution_status,
		COALESCE(q.accepted_answer_id, 0) AS accepted_answer_id, q.create_time, q.update_time
		FROM question q` + topicJoin + `
		JOIN user u ON u.user_id = q.author_id AND u.status = 1
		WHERE ` + invitationQuestionVisible + ` AND q.resolution_status = 0
		AND NOT EXISTS (SELECT 1 FROM answer a WHERE a.question_id = q.question_id AND ` + invitationAnswerVisible + `)` +
		cursorJoin + `
		ORDER BY ` + order + ` LIMIT ?`

	err = db.Select(&questions, query, args...)
	return
}

// ---------- 候选用户（BE-03） ----------

type candidateRow struct {
	UserID              int64  `db:"user_id"`
	Username            string `db:"username"`
	AvatarURL           string `db:"avatar_url"`
	TopicAnswers        int64  `db:"topic_answers"`
	TopicFollow         bool   `db:"topic_follow"`
	LastAnswerAt        int64  `db:"last_answer_at"`
	AnswerInviteEnabled int32  `db:"answer_invite_enabled"`
}

// ListInviteCandidates 邀请候选（PRD §7.2）：
// 同话题公开回答数 DESC → 最近公开回答时间 DESC → user_id ASC；理由只给粗粒度文案。
// 排除：提问者本人、双向屏蔽、偏好关闭、该问题已有有效邀请的人。最多返回 20 人（单页）。
func ListInviteCandidates(questionID, inviterID int64, keyword string, limit int) (candidates []*models.ApiCandidateUser, err error) {
	candidates = make([]*models.ApiCandidateUser, 0)
	if limit <= 0 || limit > 20 {
		limit = 20
	}

	// 目标问题的话题集合
	var topicIDs []int64
	if err = db.Select(&topicIDs, "SELECT topic_id FROM question_topic WHERE question_id = ?", questionID); err != nil {
		return nil, err
	}

	// 基础过滤：有效账号 + 偏好开启 + 非本人 + 无双向屏蔽 + 无该问题的有效邀请
	base := `SELECT u.user_id, u.username, u.avatar_url, u.answer_invite_enabled,
		IFNULL(sa.topic_answers, 0) AS topic_answers,
		IFNULL(sa.last_answer_at, 0) AS last_answer_at,
		EXISTS (SELECT 1 FROM topic_follow tf WHERE tf.user_id = u.user_id AND tf.topic_id IN (?)) AS topic_follow
		FROM user u
		LEFT JOIN (
			SELECT a.author_id, COUNT(*) AS topic_answers, MAX(UNIX_TIMESTAMP(a.create_time)) AS last_answer_at
			FROM answer a
			JOIN question q2 ON q2.question_id = a.question_id AND ` + invitationQuestionVisibleQ2 + `
			JOIN question_topic qt2 ON qt2.question_id = a.question_id AND qt2.topic_id IN (?)
			WHERE ` + invitationAnswerVisible + `
			GROUP BY a.author_id
		) sa ON sa.author_id = u.user_id
		WHERE u.status = 1 AND u.answer_invite_enabled = 1 AND u.user_id != ?
		AND u.user_id NOT IN (SELECT blocked_id FROM user_block WHERE blocker_id = ?)
		AND u.user_id NOT IN (SELECT blocker_id FROM user_block WHERE blocked_id = ?)
		AND NOT EXISTS (SELECT 1 FROM question_answer_invitation inv
			WHERE inv.question_id = ? AND inv.invitee_id = u.user_id AND inv.status IN (0, 1))`

	args := []interface{}{topicIDs, topicIDs, inviterID, inviterID, inviterID, questionID}
	if keyword != "" {
		base += " AND u.username LIKE ?"
		args = append(args, "%"+escapeLike(keyword)+"%")
	}
	// 排序：同话题回答数 → 最近回答时间 → user_id（确定性；topic_follow 只作理由不作排序）
	base += " ORDER BY topic_answers DESC, last_answer_at DESC, u.user_id ASC LIMIT ?"
	args = append(args, limit)

	rows := make([]*candidateRow, 0, limit)
	cq, cargs, err := sqlx.In(base, args...)
	if err != nil {
		return nil, err
	}
	if err = db.Select(&rows, cq, cargs...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		reason := "社区成员"
		if r.TopicAnswers > 0 {
			reason = "回答过该话题"
		} else if r.TopicFollow {
			reason = "关注了该话题"
		}
		candidates = append(candidates, &models.ApiCandidateUser{
			UserID:       fmt.Sprintf("%d", r.UserID),
			Username:     r.Username,
			AvatarURL:    r.AvatarURL,
			Reason:       reason,
			TopicAnswers: r.TopicAnswers,
			LastAnswerAt: r.LastAnswerAt,
		})
	}
	return candidates, nil
}

func escapeLike(kw string) string {
	out := make([]rune, 0, len(kw))
	for _, r := range kw {
		if r == '%' || r == '\\' || r == '_' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}

// ---------- 发送邀请（BE-04，PRD §8.1 发送事务） ----------

// InviteSendResult 单个被邀请人的结果
type InviteSendResult struct {
	InviteeID    int64  `json:"invitee_id,string"`
	InvitationID string `json:"invitation_id,omitempty"`
	OK           bool   `json:"ok"`
	Reason       string `json:"reason,omitempty"` // 粗粒度原因枚举文案
}

// inviteeCheckRow 逐人校验行
type inviteeCheckRow struct {
	UserID              int64 `db:"user_id"`
	Status              int32 `db:"status"`
	AnswerInviteEnabled int32 `db:"answer_invite_enabled"`
	Blocked             bool  `db:"blocked"`
	RecentInvite        bool  `db:"recent_invite"`
	ActiveInvite        bool  `db:"active_invite"`
}

// SendAnswerInvitations 批量发送邀请（单事务；逐人校验给出结果；失败请求不占用配额——校验在事务内、失败回滚）
func SendAnswerInvitations(inviterID, questionID int64, inviteeIDs []int64, requestID string) (results []InviteSendResult, err error) {
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. 锁定问题行并校验作者与公开状态
	q := new(questionInviteRow)
	if err = tx.Get(q, "SELECT "+questionInviteColumns+" FROM question WHERE question_id = ? FOR UPDATE", questionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrQuestionNotExist
		}
		return nil, err
	}
	if q.AuthorID != inviterID {
		return nil, ErrNotQuestionAuthor
	}
	if q.Status != 1 || q.ModerationStatus != 1 || q.Visibility != 1 || q.DeletedAt.Valid {
		return nil, ErrQuestionNotExist
	}
	if q.ResolutionStatus != models.QuestionResolutionOpen {
		return nil, ErrInviteQuestionState
	}
	// 无公开有效回答才可邀请
	var publicAnswers int
	if err = tx.Get(&publicAnswers,
		"SELECT COUNT(*) FROM answer a WHERE a.question_id = ? AND "+invitationAnswerVisible, questionID); err != nil {
		return nil, err
	}
	if publicAnswers > 0 {
		return nil, ErrInviteQuestionState
	}

	// 2. 逐人校验（批量查询，N≤5）
	checks := make([]*inviteeCheckRow, 0)
	query, args, err := sqlx.In(
		`SELECT u.user_id, u.status, u.answer_invite_enabled,
			EXISTS (SELECT 1 FROM user_block b WHERE (b.blocker_id = ? AND b.blocked_id = u.user_id) OR (b.blocker_id = u.user_id AND b.blocked_id = ?)) AS blocked,
			EXISTS (SELECT 1 FROM question_answer_invitation inv2 WHERE inv2.question_id = ? AND inv2.invitee_id = u.user_id
				AND inv2.create_time > NOW() - INTERVAL 7 DAY) AS recent_invite,
			EXISTS (SELECT 1 FROM question_answer_invitation inv3 WHERE inv3.question_id = ? AND inv3.invitee_id = u.user_id
				AND inv3.status IN (0, 1)) AS active_invite
			FROM user u WHERE u.user_id IN (?)`, inviterID, inviterID, questionID, questionID, inviteeIDs)
	if err != nil {
		return nil, err
	}
	rebQuery, rebArgs, err := sqlx.In(query, args...)
	if err != nil {
		return nil, err
	}
	if err = tx.Select(&checks, rebQuery, rebArgs...); err != nil {
		return nil, err
	}
	checkMap := make(map[int64]*inviteeCheckRow, len(checks))
	for _, c := range checks {
		checkMap[c.UserID] = c
	}

	// 3. 频控（事务内计数：失败回滚不占配额）
	var questionQuota, inviterQuota int
	if err = tx.Get(&questionQuota,
		"SELECT COUNT(DISTINCT invitee_id) FROM question_answer_invitation WHERE question_id = ? AND create_time > NOW() - INTERVAL 24 HOUR", questionID); err != nil {
		return nil, err
	}
	if err = tx.Get(&inviterQuota,
		"SELECT COUNT(*) FROM question_answer_invitation WHERE inviter_id = ? AND create_time > NOW() - INTERVAL 24 HOUR", inviterID); err != nil {
		return nil, err
	}
	remainingQuestion := models.InviteQuotaPerQuestionPerDay - questionQuota
	remainingInviter := models.InviteQuotaPerInviterPerDay - inviterQuota

	// 4. 逐人出结果；可邀请者统一插入（单事务）
	results = make([]InviteSendResult, 0, len(inviteeIDs))
	var toInsert []*models.AnswerInvitation
	for _, uid := range inviteeIDs {
		res := InviteSendResult{InviteeID: uid}
		c := checkMap[uid]
		switch {
		case c == nil:
			res.Reason = "用户不存在或不可邀请"
		case uid == inviterID:
			res.Reason = "不能邀请自己"
		case c.Status != 1:
			res.Reason = "用户不存在或不可邀请"
		case c.AnswerInviteEnabled == 0:
			res.Reason = "对方未开放回答邀请"
		case c.Blocked:
			res.Reason = "无法邀请该用户"
		case c.ActiveInvite:
			res.Reason = "已有进行中的邀请"
		case c.RecentInvite:
			res.Reason = "7 天内已邀请过该用户"
		default:
			if remainingQuestion <= 0 {
				res.Reason = "该问题今日邀请数已达上限"
			} else if remainingInviter <= 0 {
				res.Reason = "你今日的邀请数已达上限"
			}
		}
		if res.Reason == "" {
			inv := &models.AnswerInvitation{
				InvitationID: snowflake.GenerateID(),
				QuestionID:   questionID,
				InviterID:    inviterID,
				InviteeID:    uid,
				Status:       models.InviteStatusPending,
				ExpiresAt:    time.Now().UTC().Add(models.InvitationValidity),
			}
			toInsert = append(toInsert, inv)
			remainingQuestion--
			remainingInviter--
		} else {
			results = append(results, res)
		}
	}

	for _, inv := range toInsert {
		if _, err = tx.Exec(
			"INSERT INTO question_answer_invitation (invitation_id, question_id, inviter_id, invitee_id, status, request_id, expires_at)"+
				" VALUES (?, ?, ?, ?, ?, ?, ?)",
			inv.InvitationID, inv.QuestionID, inv.InviterID, inv.InviteeID, inv.Status, requestID, inv.ExpiresAt); err != nil {
			var mysqlErr *drvmysql.MySQLError
			if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
				// 并发窗口撞活跃唯一键
				results = append(results, InviteSendResult{InviteeID: inv.InviteeID, Reason: "已有进行中的邀请"})
				continue
			}
			return nil, err
		}
		// 通知被邀请人（同事务；target_id=invitation_id，前端经 question_id 字段跳转）
		if err = txCreateNotification(tx, inv.InviteeID, inv.InviterID, models.NotifyTypeAnswerInvite, models.TargetQuestion, inv.InvitationID); err != nil {
			return nil, err
		}
		results = append(results, InviteSendResult{InviteeID: inv.InviteeID, InvitationID: fmt.Sprintf("%d", inv.InvitationID), OK: true})
	}

	// 审计（同事务；只记操作者/动作/对象/原因枚举，不含正文）
	if len(toInsert) > 0 {
		if err = txAudit(tx, inviterID, "answer_invite_send", "question",
			int64ToStr(questionID), requestID, fmt.Sprintf(`{"sent":%d,"failed":%d}`, len(toInsert), len(results)-len(toInsert))); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

// ---------- 收件箱 / 状态机（BE-05，PRD §8.2） ----------

// ListInvitationsForInvitee 我收到的邀请（pending 优先展示由排序保证：状态升序 + 时间倒序）
func ListInvitationsForInvitee(inviteeID int64, statusFilter *int32, cursorTime time.Time, cursorID int64, limit int) (invitations []*models.AnswerInvitation, err error) {
	return listInvitations("invitee_id", inviteeID, statusFilter, cursorTime, cursorID, limit)
}

// ListInvitationsForQuestion 问题作者的邀请记录（最近 20 条）
func ListInvitationsForQuestion(questionID int64, limit int) (invitations []*models.AnswerInvitation, err error) {
	invitations = make([]*models.AnswerInvitation, 0)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	err = db.Select(&invitations, `SELECT inv.invitation_id, inv.question_id, inv.inviter_id, inv.invitee_id,
		inv.status, inv.expires_at, inv.responded_at, inv.answered_at, inv.create_time, inv.update_time,
		u.username AS invitee_name
		FROM question_answer_invitation inv JOIN user u ON u.user_id = inv.invitee_id
		WHERE inv.question_id = ? ORDER BY inv.create_time DESC, inv.invitation_id DESC LIMIT ?`, questionID, limit)
	return
}

func listInvitations(userCol string, userID int64, statusFilter *int32, cursorTime time.Time, cursorID int64, limit int) (invitations []*models.AnswerInvitation, err error) {
	invitations = make([]*models.AnswerInvitation, 0)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := `SELECT inv.invitation_id, inv.question_id, inv.inviter_id, inv.invitee_id,
		inv.status, inv.expires_at, inv.responded_at, inv.answered_at, inv.create_time, inv.update_time,
		u.username AS inviter_name, qu.title AS question_title
		FROM question_answer_invitation inv
		JOIN user u ON u.user_id = inv.inviter_id
		JOIN question qu ON qu.question_id = inv.question_id
		WHERE inv.` + userCol + ` = ?`
	args := []interface{}{userID}
	if statusFilter != nil {
		query += " AND inv.status = ?"
		args = append(args, *statusFilter)
	}
	if cursorID > 0 {
		query += " AND (inv.create_time < ? OR (inv.create_time = ? AND inv.invitation_id < ?))"
		args = append(args, cursorTime, cursorTime, cursorID)
	}
	query += " ORDER BY inv.create_time DESC, inv.invitation_id DESC LIMIT ?"
	args = append(args, limit+1)
	// 注意：调用方按 limit+1 判定 has_more；此处返回 limit+1 条，由 logic 截断
	err = db.Select(&invitations, query, args...)
	return
}

// RespondInvitation 接受/婉拒（PRD §8.2）：
// 只允许 pending → accepted/declined；重复同态操作幂等成功；过期/失效/终态返回状态错误。
// 锁序：问题行 → 邀请行（与发送事务一致）。
func RespondInvitation(invitationID, inviteeID int64, toStatus int32, requestID string) error {
	if toStatus != models.InviteStatusAccepted && toStatus != models.InviteStatusDeclined {
		return ErrInviteState
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 先读邀请（拿到 question_id 与归属校验）
	inv := new(models.AnswerInvitation)
	if err = tx.Get(inv, "SELECT invitation_id, question_id, inviter_id, invitee_id, status, expires_at FROM question_answer_invitation WHERE invitation_id = ?", invitationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvitationNotExist
		}
		return err
	}
	if inv.InviteeID != inviteeID {
		return ErrInviteNotReceiver
	}

	// 锁问题行（校验公开状态 + 双方账号有效）
	q := new(questionInviteRow)
	if err = tx.Get(q, "SELECT "+questionInviteColumns+" FROM question WHERE question_id = ? FOR UPDATE", inv.QuestionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInviteState
		}
		return err
	}
	if q.Status != 1 || q.ModerationStatus != 1 || q.Visibility != 1 || q.DeletedAt.Valid {
		// 问题已不可见：待处理邀请转失效，不可接受
		if inv.Status == models.InviteStatusPending {
			_, _ = tx.Exec("UPDATE question_answer_invitation SET status = 5 WHERE invitation_id = ? AND status = 0", invitationID)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return ErrInviteState
	}

	// 锁邀请行
	type invitationLockRow struct {
		Status    int32     `db:"status"`
		ExpiresAt time.Time `db:"expires_at"`
	}
	lock := new(invitationLockRow)
	if err = tx.Get(lock, "SELECT status, expires_at FROM question_answer_invitation WHERE invitation_id = ? FOR UPDATE", invitationID); err != nil {
		return err
	}
	status, expiresAt := lock.Status, lock.ExpiresAt
	if status == toStatus {
		// 幂等：重复同态操作成功
		return tx.Commit()
	}
	if status != models.InviteStatusPending {
		return ErrInviteState
	}
	if time.Now().UTC().After(expiresAt) {
		_, _ = tx.Exec("UPDATE question_answer_invitation SET status = 5 WHERE invitation_id = ? AND status = 0", invitationID)
		if err = tx.Commit(); err != nil {
			return err
		}
		return ErrInviteState
	}

	// 双方账号仍有效
	var inviterStatus, inviteeStatus int32
	if err = tx.Get(&inviterStatus, "SELECT status FROM user WHERE user_id = ?", inv.InviterID); err != nil {
		return err
	}
	if err = tx.Get(&inviteeStatus, "SELECT status FROM user WHERE user_id = ?", inviteeID); err != nil {
		return err
	}
	if inviterStatus != 1 || inviteeStatus != 1 {
		_, _ = tx.Exec("UPDATE question_answer_invitation SET status = 5 WHERE invitation_id = ? AND status = 0", invitationID)
		if err = tx.Commit(); err != nil {
			return err
		}
		return ErrInviteState
	}

	result, err := tx.Exec(
		"UPDATE question_answer_invitation SET status = ?, responded_at = NOW() WHERE invitation_id = ? AND status = 0",
		toStatus, invitationID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrInviteState
	}
	// 通知提问者（同事务）
	notifyType := int32(models.NotifyTypeInviteAccepted)
	if toStatus == models.InviteStatusDeclined {
		notifyType = int32(models.NotifyTypeInviteDeclined)
	}
	if err = txCreateNotification(tx, inv.InviterID, inviteeID, notifyType, models.TargetQuestion, inv.QuestionID); err != nil {
		return err
	}
	action := "answer_invite_accept"
	if toStatus == models.InviteStatusDeclined {
		action = "answer_invite_decline"
	}
	if err = txAudit(tx, inviteeID, action, "invitation", int64ToStr(invitationID), requestID,
		fmt.Sprintf(`{"question_id":"%s"}`, int64ToStr(inv.QuestionID))); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- 回答完成联动 / 失效钩子（BE-06） ----------

// MarkInvitationAnsweredTx 被邀请者的公开回答落地 → 邀请转 answered（PRD §8.3；§3 术语含 pending 直答场景）
func MarkInvitationAnsweredTx(tx *sqlx.Tx, questionID, inviteeID int64) error {
	_, err := tx.Exec(
		"UPDATE question_answer_invitation SET status = 3, answered_at = NOW()"+
			" WHERE question_id = ? AND invitee_id = ? AND status IN (0, 1)", questionID, inviteeID)
	return err
}

// MarkInvitationAnswered 独立事务版本（审核批准路径）
func MarkInvitationAnswered(questionID, inviteeID int64) error {
	_, err := db.Exec(
		"UPDATE question_answer_invitation SET status = 3, answered_at = NOW()"+
			" WHERE question_id = ? AND invitee_id = ? AND status IN (0, 1)", questionID, inviteeID)
	return err
}

// ExpireQuestionInvitationsTx 问题关闭/删除/下架/隐藏后，待处理邀请转失效
func ExpireQuestionInvitationsTx(tx *sqlx.Tx, questionID int64) error {
	_, err := tx.Exec(
		"UPDATE question_answer_invitation SET status = 5 WHERE question_id = ? AND status = 0", questionID)
	return err
}

// ExpireQuestionInvitations 独立事务版本
func ExpireQuestionInvitations(questionID int64) error {
	_, err := db.Exec(
		"UPDATE question_answer_invitation SET status = 5 WHERE question_id = ? AND status = 0", questionID)
	return err
}

// ExpireUserInvitations 用户禁用后，其作为任一方的待处理邀请转失效
func ExpireUserInvitations(userID int64) error {
	_, err := db.Exec(
		"UPDATE question_answer_invitation SET status = 5 WHERE status = 0 AND (invitee_id = ? OR inviter_id = ?)", userID, userID)
	return err
}

// ExpireInvitationsBetween 双方建立屏蔽后，两人之间的待处理邀请转失效
func ExpireInvitationsBetween(a, b int64) error {
	_, err := db.Exec(
		"UPDATE question_answer_invitation SET status = 5 WHERE status = 0 AND "+
			"((inviter_id = ? AND invitee_id = ?) OR (inviter_id = ? AND invitee_id = ?))", a, b, b, a)
	return err
}

// ---------- 偏好 ----------

// GetAnswerInvitePreference 读取接收偏好（默认开启）
func GetAnswerInvitePreference(userID int64) (bool, error) {
	var enabled int32
	err := db.Get(&enabled, "SELECT answer_invite_enabled FROM user WHERE user_id = ?", userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrUserNotExist
		}
		return false, err
	}
	return enabled == 1, nil
}

// SetAnswerInvitePreference 设置接收偏好
func SetAnswerInvitePreference(userID int64, enabled bool) error {
	v := int32(0)
	if enabled {
		v = 1
	}
	result, err := db.Exec("UPDATE user SET answer_invite_enabled = ? WHERE user_id = ? AND status = 1", v, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrUserNotExist
	}
	return nil
}
