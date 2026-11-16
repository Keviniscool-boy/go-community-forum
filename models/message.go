package models

import "time"

// ---------- 私信 ----------

// 隐私策略
const (
	DMIncomingNobody    = "nobody"
	DMIncomingFollowing = "following"
	DMIncomingEveryone  = "everyone"
)

// 会话状态
const (
	ConvStatePending  = 0
	ConvStateAccepted = 1
	ConvStateDeclined = 2
	ConvStateClosed   = 3
)

// MessagePrivacySetting 私信隐私设置
type MessagePrivacySetting struct {
	UserID              int64     `json:"user_id,string" db:"user_id"`
	IncomingPolicy      string    `json:"incoming_policy" db:"incoming_policy"`
	ReadReceiptsEnabled bool      `json:"read_receipts_enabled" db:"read_receipts_enabled"`
	Version             int64     `json:"version" db:"version"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
}

// Conversation 会话
type Conversation struct {
	ConversationID int64      `json:"id,string" db:"conversation_id"`
	UserLowID      int64      `json:"-" db:"user_low_id"`
	UserHighID     int64      `json:"-" db:"user_high_id"`
	InitiatorID    int64      `json:"initiator_id,string" db:"initiator_id"`
	State          int32      `json:"state" db:"state"`
	Version        int64      `json:"version" db:"version"`
	CreateTime     time.Time  `json:"create_time" db:"create_time"`
	UpdateTime     time.Time  `json:"update_time" db:"update_time"`
	// 展示层字段（非表列）：对方信息与我的未读数
	PeerName  string `json:"peer_name,omitempty"`
	PeerID    string `json:"peer_id,omitempty"`
	LastBody  string `json:"last_body,omitempty"`
	Unread    int64  `json:"unread,omitempty"`
}

// DirectMessage 私信消息（纯文本）
type DirectMessage struct {
	MessageID      int64     `json:"id,string" db:"message_id"`
	ConversationID int64     `json:"conversation_id,string" db:"conversation_id"`
	SenderID       int64     `json:"sender_id,string" db:"sender_id"`
	ClientMessageID string   `json:"-" db:"client_message_id"`
	Body           string    `json:"body" db:"body"`
	CreateTime     time.Time `json:"create_time" db:"create_time"`
}

// ParamStartConversation 发起会话请求
type ParamStartConversation struct {
	RecipientID     string `json:"recipient_id" binding:"required"`
	Body            string `json:"body" binding:"required,min=1,max=2000"`
	ClientMessageID string `json:"client_message_id" binding:"required,min=1,max=64"`
}

// ParamSendMessage 会话内发送消息
type ParamSendMessage struct {
	Body            string `json:"body" binding:"required,min=1,max=2000"`
	ClientMessageID string `json:"client_message_id" binding:"required,min=1,max=64"`
}

// ParamReadRequest 已读上报
type ParamReadRequest struct {
	LastReadMessageID string `json:"last_read_message_id" binding:"required"`
}

// ParamPrivacySettings 隐私设置更新
type ParamPrivacySettings struct {
	IncomingPolicy      string `json:"incoming_policy" binding:"required,oneof=nobody following everyone"`
	ReadReceiptsEnabled *bool  `json:"read_receipts_enabled" binding:"required"`
	Version             int64  `json:"version" binding:"required,min=0"`
}
