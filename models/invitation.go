package models

// 回答邀请（待回答问题发现与回答邀请 PRD §3/§8）
// 状态机：pending → accepted/declined（仅接收者）；accepted → answered（公开回答落地）；
// pending/accepted → revoked（作者撤回，P1）/ expired（7 天过期或问题/账号/屏蔽失效）。

import "time"

// 邀请状态
const (
	InviteStatusPending  = int32(0)
	InviteStatusAccepted = int32(1)
	InviteStatusDeclined = int32(2)
	InviteStatusAnswered = int32(3)
	InviteStatusRevoked  = int32(4)
	InviteStatusExpired  = int32(5)
)

// AnswerInvitation 回答邀请模型
type AnswerInvitation struct {
	InvitationID  int64      `json:"id,string" db:"invitation_id"`
	QuestionID    int64      `json:"question_id,string" db:"question_id"`
	InviterID     int64      `json:"inviter_id,string" db:"inviter_id"`
	InviteeID     int64      `json:"invitee_id,string" db:"invitee_id"`
	Status        int32      `json:"status" db:"status"`
	ExpiresAt     time.Time  `json:"expires_at" db:"expires_at"`
	RespondedAt   *time.Time `json:"responded_at" db:"responded_at"`
	AnsweredAt    *time.Time `json:"answered_at" db:"answered_at"`
	CreateTime    time.Time  `json:"create_time" db:"create_time"`
	UpdateTime    time.Time  `json:"update_time" db:"update_time"`
	// 展示字段（联查派生，非持久化）
	InviteeName   string `json:"invitee_name,omitempty" db:"invitee_name"`
	InviterName   string `json:"inviter_name,omitempty" db:"inviter_name"`
	QuestionTitle string `json:"question_title,omitempty" db:"question_title"`
}

// 邀请有效期（PRD §3：默认发送后 7 天）
const InvitationValidity = 7 * 24 * time.Hour

// 频控初始默认值（PRD §8：运营后可配置）
const (
	InviteQuotaPerQuestionPerDay = 5  // 每问题每 24h 最多邀请的不同用户数
	InviteQuotaPerInviterPerDay  = 20 // 每邀请者每 24h 最多发送条数
	InviteBatchMax               = 5  // 单次最多邀请人数
	InviteReinviteCooldown       = 7 * 24 * time.Hour // 同一用户+问题两次邀请最小间隔
)

// ParamSendAnswerInvitations 批量邀请请求
type ParamSendAnswerInvitations struct {
	InviteeIDs []string `json:"invitee_ids" binding:"required,min=1,max=5,dive,min=1"`
}

// ApiCandidateUser 邀请候选（粗粒度理由，不泄露精确行为统计）
type ApiCandidateUser struct {
	UserID       string `json:"user_id,string"`
	Username     string `json:"username"`
	AvatarURL    string `json:"avatar_url"`
	Reason       string `json:"reason"` // 回答过该话题 / 关注了该话题
	TopicAnswers int64  `json:"-"`      // 排序用，不输出精确数字
	LastAnswerAt int64  `json:"-"`      // 排序用
}

// ParamAnswerInvitationPreference 接收偏好
type ParamAnswerInvitationPreference struct {
	Enabled *bool `json:"enabled" binding:"required"`
}
