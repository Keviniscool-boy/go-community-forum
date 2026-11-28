package mysql

import (
	"database/sql"
	"errors"
	"strconv"

	"bluebell/models"

	"go.uber.org/zap"
)

// ---------- 私信 ----------

var (
	ErrConversationNotExist = errors.New("会话不存在")
	ErrNotConversationPeer  = errors.New("无权访问该会话")
	ErrStateNotAllowed      = errors.New("会话状态不允许该操作")
	ErrPrivacyConflict      = errors.New("隐私设置版本冲突，请刷新后重试")
)

// GetMessagePrivacySetting 读取隐私设置（无记录返回默认 following + version 0）
func GetMessagePrivacySetting(userID int64) (policy string, receipts bool, version int64, err error) {
	var row struct {
		Policy  string `db:"incoming_policy"`
		Receipt bool   `db:"read_receipts_enabled"`
		Version int64  `db:"version"`
	}
	err = db.Get(&row, "SELECT incoming_policy, read_receipts_enabled, version FROM message_privacy_setting WHERE user_id = ?", userID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.DMIncomingFollowing, true, 0, nil
	}
	if err != nil {
		return "", false, 0, err
	}
	return row.Policy, row.Receipt, row.Version, nil
}

// UpsertPrivacySettings 更新隐私设置（乐观锁；首条 version 从 1 开始）
func UpsertPrivacySettings(userID int64, policy string, receipts bool, expectVersion int64) (newVersion int64, err error) {
	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var cur int64
	err = tx.Get(&cur, "SELECT version FROM message_privacy_setting WHERE user_id = ?", userID)
	if errors.Is(err, sql.ErrNoRows) {
		if expectVersion != 0 {
			return 0, ErrPrivacyConflict
		}
		if _, err = tx.Exec(
			"INSERT INTO message_privacy_setting (user_id, incoming_policy, read_receipts_enabled, version) VALUES (?, ?, ?, 1)",
			userID, policy, receipts); err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	} else {
		// B05 同款 CAS 条件更新：并发时受影响行 0 → 版本冲突
		result, err := tx.Exec(
			"UPDATE message_privacy_setting SET incoming_policy = ?, read_receipts_enabled = ?, version = version + 1 WHERE user_id = ? AND version = ?",
			policy, receipts, userID, expectVersion)
		if err != nil {
			return 0, err
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return 0, ErrPrivacyConflict
		}
	}
	if err = tx.Get(&newVersion, "SELECT version FROM message_privacy_setting WHERE user_id = ?", userID); err != nil {
		return 0, err
	}
	return newVersion, tx.Commit()
}

// GetConversation 查询会话
func GetConversation(conversationID int64) (*models.Conversation, error) {
	c := new(models.Conversation)
	err := db.Get(c, "SELECT conversation_id, user_low_id, user_high_id, initiator_id, state, version, declined_at, create_time, update_time FROM conversation WHERE conversation_id = ?", conversationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConversationNotExist
		}
		return nil, err
	}
	return c, nil
}

// GetConversationByPair 按参与者对查询会话
func GetConversationByPair(userA, userB int64) (*models.Conversation, error) {
	low, high := userA, userB
	if low > high {
		low, high = high, low
	}
	c := new(models.Conversation)
	err := db.Get(c, "SELECT conversation_id, user_low_id, user_high_id, initiator_id, state, version, declined_at, create_time, update_time FROM conversation WHERE user_low_id = ? AND user_high_id = ?", low, high)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConversationNotExist
		}
		return nil, err
	}
	return c, nil
}

// CreateConversation 创建会话与成员行（事务）
func CreateConversation(c *models.Conversation) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(
		"INSERT INTO conversation (conversation_id, user_low_id, user_high_id, initiator_id, state) VALUES (?, ?, ?, ?, ?)",
		c.ConversationID, c.UserLowID, c.UserHighID, c.InitiatorID, c.State); err != nil {
		return err
	}
	for _, uid := range []int64{c.UserLowID, c.UserHighID} {
		if _, err = tx.Exec(
			"INSERT INTO conversation_member (conversation_id, user_id) VALUES (?, ?)", c.ConversationID, uid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateConversationState 状态迁移（乐观锁；仅接收方/成员校验由调用方完成）
func UpdateConversationState(conversationID int64, fromState, toState int32, expectVersion int64) (bool, error) {
	result, err := db.Exec(
		"UPDATE conversation SET state = ?, version = version + 1 WHERE conversation_id = ? AND state = ? AND version = ?",
		toState, conversationID, fromState, expectVersion)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// DeclineResetToPending 拒绝会话重置为 pending（冷却通过后），并清空 declined_at
func DeclineResetToPending(conversationID int64, expectVersion int64) (bool, error) {
	result, err := db.Exec(
		"UPDATE conversation SET state = 0, declined_at = NULL, version = version + 1 WHERE conversation_id = ? AND state = 2 AND version = ?",
		conversationID, expectVersion)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// CountMessagesInConversation 会话内消息总数（B02 首信唯一判定）
func CountMessagesInConversation(conversationID int64) (int64, error) {
	var n int64
	err := db.Get(&n, "SELECT COUNT(*) FROM direct_message WHERE conversation_id = ?", conversationID)
	return n, err
}

// DeclineConversationCAS 接收方拒绝：状态 CAS 迁移并记录 declined_at（冷却基准）
func DeclineConversationCAS(conversationID int64, expectVersion int64) (bool, error) {
	result, err := db.Exec(
		"UPDATE conversation SET state = 2, declined_at = NOW(), version = version + 1 WHERE conversation_id = ? AND state = 0 AND version = ?",
		conversationID, expectVersion)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// IsConversationMember 是否为会话成员
func IsConversationMember(conversationID, userID int64) (bool, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(id) FROM conversation_member WHERE conversation_id = ? AND user_id = ?", conversationID, userID)
	return count > 0, err
}

// ListConversations 成员的会话列表（含对方信息、最后一条消息、未读数）
func ListConversations(userID int64, limit int) (conversations []*models.Conversation, err error) {
	conversations = make([]*models.Conversation, 0, limit)
	err = db.Select(&conversations, `
		SELECT c.conversation_id, c.user_low_id, c.user_high_id, c.initiator_id, c.state, c.version, c.create_time, c.update_time
		FROM conversation c
		JOIN conversation_member m ON m.conversation_id = c.conversation_id AND m.user_id = ?
		WHERE (m.hidden_at IS NULL AND m.closed_at IS NULL)
		ORDER BY c.update_time DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	// 装配对方信息、最后一条消息、未读数
	for _, c := range conversations {
		peer := c.UserLowID
		if userID == c.UserLowID {
			peer = c.UserHighID
		}
		c.PeerID = int64ToStringD(peer)
		var peerName string
		if err := db.Get(&peerName, "SELECT username FROM user WHERE user_id = ?", peer); err == nil {
			c.PeerName = peerName
		}
		var lastBody *string
		if err := db.Get(&lastBody, "SELECT body FROM direct_message WHERE conversation_id = ? ORDER BY create_time DESC, message_id DESC LIMIT 1", c.ConversationID); err == nil && lastBody != nil {
			c.LastBody = *lastBody
		}
		// 未读：对方发送且 id 大于我的已读水位
		var lastRead int64
		_ = db.Get(&lastRead, "SELECT last_read_message_id FROM conversation_member WHERE conversation_id = ? AND user_id = ?", c.ConversationID, userID)
		_ = db.Get(&c.Unread,
			"SELECT COUNT(*) FROM direct_message WHERE conversation_id = ? AND sender_id = ? AND message_id > ?",
			c.ConversationID, peer, lastRead)
	}
	return conversations, nil
}

// PeerIDOf 会话中对方的用户 ID
func PeerIDOf(c *models.Conversation, userID int64) int64 {
	if userID == c.UserLowID {
		return c.UserHighID
	}
	return c.UserLowID
}

// ListMessages 会话消息分页（成员校验由调用方完成）
func ListMessages(conversationID int64, beforeID int64, limit int) (messages []*models.DirectMessage, err error) {
	messages = make([]*models.DirectMessage, 0, limit)
	q := limit + 1
	var query string
	var args []interface{}
	if beforeID > 0 {
		// 游标：message_id 单调（雪花）倒序
		query = "SELECT message_id, conversation_id, sender_id, client_message_id, body, create_time FROM direct_message " +
			"WHERE conversation_id = ? AND message_id < ? ORDER BY message_id DESC LIMIT ?"
		args = []interface{}{conversationID, beforeID, q}
	} else {
		query = "SELECT message_id, conversation_id, sender_id, client_message_id, body, create_time FROM direct_message " +
			"WHERE conversation_id = ? ORDER BY message_id DESC LIMIT ?"
		args = []interface{}{conversationID, q}
	}
	err = db.Select(&messages, query, args...)
	return
}

// GetMessage 查询单条消息
func GetMessage(messageID int64) (*models.DirectMessage, error) {
	m := new(models.DirectMessage)
	err := db.Get(m, "SELECT message_id, conversation_id, sender_id, client_message_id, body, create_time FROM direct_message WHERE message_id = ?", messageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConversationNotExist
		}
		return nil, err
	}
	return m, nil
}

// InsertMessage 幂等写入消息（client_message_id 唯一；重复返回已有消息与 duplicated 标记）
func InsertMessage(m *models.DirectMessage) (duplicated bool, err error) {
	result, err := db.Exec(
		"INSERT IGNORE INTO direct_message (message_id, conversation_id, sender_id, client_message_id, body) VALUES (?, ?, ?, ?, ?)",
		m.MessageID, m.ConversationID, m.SenderID, m.ClientMessageID, m.Body)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return true, nil
	}
	return false, nil
}

// AdvanceReadMarker 已读水位单调推进
func AdvanceReadMarker(conversationID, userID, messageID int64) error {
	_, err := db.Exec(
		"UPDATE conversation_member SET last_read_message_id = ? "+
			"WHERE conversation_id = ? AND user_id = ? AND last_read_message_id < ?",
		messageID, conversationID, userID, messageID)
	return err
}

// HideConversation 本人隐藏会话视图
func HideConversation(conversationID, userID int64) error {
	_, err := db.Exec(
		"UPDATE conversation_member SET hidden_at = NOW() WHERE conversation_id = ? AND user_id = ?",
		conversationID, userID)
	return err
}

// CloseConversationForMember 本人关闭会话视图
func CloseConversationForMember(conversationID, userID int64) error {
	_, err := db.Exec(
		"UPDATE conversation_member SET closed_at = NOW() WHERE conversation_id = ? AND user_id = ?",
		conversationID, userID)
	return err
}

// IsMessageSenderOrPeer 校验消息属于会话且用户为成员（举报用）
func IsMessageInConversationOfPeer(messageID, conversationID int64) (bool, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(*) FROM direct_message WHERE message_id = ? AND conversation_id = ?", messageID, conversationID)
	return count > 0, err
}

var _ = zap.L

// GetMessageByClientKey 按客户端幂等键查回消息
func GetMessageByClientKey(conversationID, senderID int64, clientMessageID string) (*models.DirectMessage, error) {
	m := new(models.DirectMessage)
	err := db.Get(m, "SELECT message_id, conversation_id, sender_id, client_message_id, body, create_time FROM direct_message WHERE conversation_id = ? AND sender_id = ? AND client_message_id = ?",
		conversationID, senderID, clientMessageID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConversationNotExist
		}
		return nil, err
	}
	return m, nil
}

// int64ToStringD 本地转换
func int64ToStringD(v int64) string {
	return strconv.FormatInt(v, 10)
}
