package mysql

// 私信 DAO（第五阶段 PRD F6）
// 会话 (min,max) 唯一；删除会话仅自己视角；未读 = last_message_at 之后对方消息数。

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"bluebell/models"
	"bluebell/pkg/snowflake"

)

var ErrThreadNotExist = errors.New("会话不存在")

// pairOf 返回 (min, max) 排序后的用户对
func pairOf(a, b int64) (int64, int64) {
	if a < b {
		return a, b
	}
	return b, a
}

// peerOf 我在会话中的对方 id
func peerOf(t *models.MessageThread, me int64) int64 {
	if t.UserA == me {
		return t.UserB
	}
	return t.UserA
}

// GetThreadByID 查会话
func GetThreadByID(threadID int64) (*models.MessageThread, error) {
	t := new(models.MessageThread)
	err := db.Get(t, `SELECT thread_id, user_a, user_b, last_message_id, last_message_at,
		a_read_at, b_read_at, a_deleted_at, b_deleted_at, create_time
		FROM message_thread WHERE thread_id = ?`, threadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrThreadNotExist
		}
		return nil, err
	}
	return t, nil
}

// findThreadByPair 查既有会话（含自删标记；调用方决定是否复活）
func findThreadByPair(a, b int64) (*models.MessageThread, error) {
	lo, hi := pairOf(a, b)
	t := new(models.MessageThread)
	err := db.Get(t, `SELECT thread_id, user_a, user_b, last_message_id, last_message_at,
		a_read_at, b_read_at, a_deleted_at, b_deleted_at, create_time
		FROM message_thread WHERE user_a = ? AND user_b = ?`, lo, hi)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrThreadNotExist
		}
		return nil, err
	}
	return t, nil
}

// SendMessageInThreadTx 在事务内插入消息并更新会话（开聊与追加共用；调用方负责校验与限流）
func SendMessageInThreadTx(tx TxLike, threadID int64, senderID int64, content string, now time.Time) (*models.Message, error) {
	msg := &models.Message{
		MessageID:  snowflake.GenerateID(),
		ThreadID:   threadID,
		SenderID:   senderID,
		Content:    content,
		CreateTime: now,
	}
	if _, err := tx.Exec(
		"INSERT INTO message (message_id, thread_id, sender_id, content, create_time) VALUES (?, ?, ?, ?, ?)",
		msg.MessageID, msg.ThreadID, msg.SenderID, msg.Content, msg.CreateTime); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		"UPDATE message_thread SET last_message_id = ?, last_message_at = ?, "+
			"a_deleted_at = NULL, b_deleted_at = NULL WHERE thread_id = ?",
		msg.MessageID, msg.CreateTime, threadID); err != nil {
		return nil, err
	}
	return msg, nil
}

// CreateThreadWithMessageTx 新建会话并发首条消息（单事务）
func CreateThreadWithMessageTx(tx TxLike, a, b, senderID int64, content string) (*models.Message, int64, error) {
	lo, hi := pairOf(a, b)
	threadID := snowflake.GenerateID()
	now := time.Now().UTC()
	if _, err := tx.Exec(
		"INSERT INTO message_thread (thread_id, user_a, user_b, last_message_id, last_message_at, create_time) VALUES (?, ?, ?, ?, ?, ?)",
		threadID, lo, hi, 0, now, now); err != nil {
		return nil, 0, err
	}
	msg, err := SendMessageInThreadTx(tx, threadID, senderID, content, now)
	if err != nil {
		return nil, 0, err
	}
	return msg, threadID, nil
}

// ListThreads 我的会话列表（按最近消息倒序；带对方资料与未读数；已删除的会话不出现，收到新消息时复活）
func ListThreads(userID int64, cursorTime time.Time, cursorID int64, limit int) (threads []*models.MessageThread, err error) {
	threads = make([]*models.MessageThread, 0)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := `
		SELECT t.thread_id, t.user_a, t.user_b, t.last_message_id, t.last_message_at,
			t.a_read_at, t.b_read_at, t.a_deleted_at, t.b_deleted_at, t.create_time,
			CASE WHEN t.user_a = ? THEN t.user_b ELSE t.user_a END AS peer_id,
			pu.username AS peer_name, pu.avatar_url AS peer_avatar,
			COALESCE((SELECT COUNT(*) FROM message m
				WHERE m.thread_id = t.thread_id AND m.sender_id != ?
				  AND m.create_time > COALESCE(CASE WHEN t.user_a = ? THEN t.a_read_at ELSE t.b_read_at END, '1970-01-01')), 0) AS unread_count,
			(SELECT LEFT(m2.content, 60) FROM message m2 WHERE m2.message_id = t.last_message_id) AS last_excerpt
		FROM message_thread t
		JOIN user pu ON pu.user_id = CASE WHEN t.user_a = ? THEN t.user_b ELSE t.user_a END
		WHERE (t.user_a = ? OR t.user_b = ?)
		  AND CASE WHEN t.user_a = ? THEN t.a_deleted_at ELSE t.b_deleted_at END IS NULL`
	// 参数与占位符逐一对应（7 个 ?）
	args := []interface{}{userID, userID, userID, userID, userID, userID, userID}
	if !cursorTime.IsZero() && cursorID > 0 {
		query += " AND (t.last_message_at < ? OR (t.last_message_at = ? AND t.thread_id < ?))"
		args = append(args, cursorTime, cursorTime, cursorID)
	}
	query += " ORDER BY t.last_message_at DESC, t.thread_id DESC LIMIT ?"
	args = append(args, limit+1)

	rows := make([]*models.MessageThread, 0, limit+1)
	if err = db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	// 过滤 self_deleted（在 SQL 中已排除；此处兜底）
	for _, t := range rows {
		t.PeerID = fmt.Sprintf("%d", peerOf(t, userID))
		threads = append(threads, t)
	}
	return threads, nil
}

// MarkThreadRead 打开会话即更新我的已读水位
func MarkThreadRead(threadID, userID int64) error {
	_, err := db.Exec(
		"UPDATE message_thread SET a_read_at = IF(user_a = ?, NOW(), a_read_at), "+
			"b_read_at = IF(user_b = ?, NOW(), b_read_at) WHERE thread_id = ?",
		userID, userID, threadID)
	return err
}

// SoftDeleteThread 删除会话（仅自己视角）
func SoftDeleteThread(threadID, userID int64) error {
	_, err := db.Exec(
		"UPDATE message_thread SET a_deleted_at = IF(user_a = ?, NOW(), a_deleted_at), "+
			"b_deleted_at = IF(user_b = ?, NOW(), b_deleted_at) WHERE thread_id = ?",
		userID, userID, threadID)
	return err
}

// ListThreadMessages 会话内消息（最早在前；cursor 向上翻页取更早的）
func ListThreadMessages(threadID int64, cursorTime time.Time, cursorID int64, limit int) (msgs []*models.ApiMessageDetail, err error) {
	msgs = make([]*models.ApiMessageDetail, 0)
	if limit <= 0 || limit > 50 {
		limit = 30
	}
	query := `SELECT m.message_id, m.thread_id, m.sender_id, m.content, m.create_time, u.username AS sender_name
		FROM message m JOIN user u ON u.user_id = m.sender_id
		WHERE m.thread_id = ?`
	args := []interface{}{threadID}
	if !cursorTime.IsZero() && cursorID > 0 {
		query += " AND (m.create_time < ? OR (m.create_time = ? AND m.id < ?))"
		args = append(args, cursorTime, cursorTime, cursorID)
	}
	query += " ORDER BY m.create_time DESC, m.id DESC LIMIT ?"
	args = append(args, limit+1)

	rows := make([]*models.ApiMessageDetail, 0, limit+1)
	if err = db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	// 反转：最早在前
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	if len(rows) > limit {
		// 多取的一条是最早的（用于 has_more 判定），截掉后仍需返回最早的 limit 条
		rows = rows[1:]
	}
	return rows, nil
}

// HasMoreOlderMessages 判断是否还有更早消息
func HasMoreOlderMessages(threadID int64, oldestTime time.Time, oldestID int64) (bool, error) {
	var n int
	err := db.Get(&n,
		"SELECT COUNT(*) FROM message WHERE thread_id = ? AND (create_time < ? OR (create_time = ? AND id < ?))",
		threadID, oldestTime, oldestTime, oldestID)
	return n > 0, err
}

// CountThreadUnread 合计我的私信未读数（TopNav 角标）
func CountThreadUnread(userID int64) (int64, error) {
	var n int64
	err := db.Get(&n, `
		SELECT COALESCE(SUM(COALESCE((SELECT COUNT(*) FROM message m
			WHERE m.thread_id = t.thread_id AND m.sender_id != ?
			AND m.create_time > COALESCE(CASE WHEN t.user_a = ? THEN t.a_read_at ELSE t.b_read_at END, '1970-01-01')), 0)), 0)
		FROM message_thread t
		WHERE (t.user_a = ? OR t.user_b = ?)
		AND CASE WHEN t.user_a = ? THEN t.a_deleted_at ELSE t.b_deleted_at END IS NULL`,
		userID, userID, userID, userID, userID)
	return n, err
}

// GetThreadByIDTx 事务内查会话（存在返回，不存在返回 nil 无错误）
func GetThreadByIDTx(tx TxLike, threadID int64) (*models.MessageThread, error) {
	t := new(models.MessageThread)
	err := tx.Get(t, `SELECT thread_id, user_a, user_b, last_message_id, last_message_at,
		a_read_at, b_read_at, a_deleted_at, b_deleted_at, create_time
		FROM message_thread WHERE thread_id = ?`, threadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// FindThreadByPairTx 事务内按用户对查会话（不存在返回 nil 无错误）
func FindThreadByPairTx(tx TxLike, a, b int64) (*models.MessageThread, error) {
	lo, hi := pairOf(a, b)
	t := new(models.MessageThread)
	err := tx.Get(t, `SELECT thread_id, user_a, user_b, last_message_id, last_message_at,
		a_read_at, b_read_at, a_deleted_at, b_deleted_at, create_time
		FROM message_thread WHERE user_a = ? AND user_b = ?`, lo, hi)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}
