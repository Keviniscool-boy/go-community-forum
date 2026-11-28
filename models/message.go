package models

// 私信（第五阶段 PRD F6）：一对一会话 + 消息；删除会话仅自己视角隐藏。

import "time"

// 通知类型 7（私信）已在 models/interaction.go 定义。

// MessageThread 会话（user_a < user_b 唯一对）
type MessageThread struct {
	ThreadID      int64      `json:"id,string" db:"thread_id"`
	UserA         int64      `json:"-" db:"user_a"`
	UserB         int64      `json:"-" db:"user_b"`
	LastMessageID int64      `json:"-" db:"last_message_id"`
	LastMessageAt *time.Time `json:"last_message_at" db:"last_message_at"`
	AReadAt       *time.Time `json:"-" db:"a_read_at"`
	BReadAt       *time.Time `json:"-" db:"b_read_at"`
	ADeletedAt    *time.Time `json:"-" db:"a_deleted_at"`
	BDeletedAt    *time.Time `json:"-" db:"b_deleted_at"`
	CreateTime    time.Time  `json:"create_time" db:"create_time"`
	// 展示字段（联查派生）
	PeerID      string `json:"peer_id,omitempty" db:"peer_id"`
	PeerName    string `json:"peer_name,omitempty" db:"peer_name"`
	PeerAvatar  string `json:"peer_avatar,omitempty" db:"peer_avatar"`
	UnreadCount int64  `json:"unread_count" db:"unread_count"`
	LastExcerpt string `json:"last_excerpt,omitempty" db:"last_excerpt"`
}

// Message 私信消息
type Message struct {
	MessageID  int64     `json:"id,string" db:"message_id"`
	ThreadID   int64     `json:"thread_id,string" db:"thread_id"`
	SenderID   int64     `json:"sender_id,string" db:"sender_id"`
	Content    string    `json:"content" db:"content"`
	CreateTime time.Time `json:"create_time" db:"create_time"`
}

// ParamSendMessage 发送消息
type ParamSendMessage struct {
	Content string `json:"content" binding:"required,min=1,max=2000"`
}

// ApiMessageDetail 消息详情（会话页；带发送者名）
type ApiMessageDetail struct {
	*Message
	SenderName string `json:"sender_name" db:"sender_name"`
}
