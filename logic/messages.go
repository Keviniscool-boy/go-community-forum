package logic

// 私信会话与消息逻辑

import (
	"errors"
	"strconv"

	"context"
	"time"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/snowflake"

)

// 私信业务错误（对外统一不可联系提示，防枚举）
var (
	ErrCannotContact      = errors.New("无法向该用户发起沟通")
	ErrDMRateLimited      = errors.New("发送过于频繁，请稍后再试")
	ErrNotRequestReceiver = errors.New("仅会话接收方可以处理该请求")
	ErrNotConversationPeer = errors.New("无权访问该会话")
	ErrStateNotAllowed    = errors.New("会话状态不允许该操作")
)

// StartConversation 发起会话（首条消息）：权限/隐私/屏蔽/限流校验 + 幂等
func StartConversation(initiatorID int64, p *models.ParamStartConversation) (conversationID int64, message *models.DirectMessage, err error) {
	recipientID, err := strconv.ParseInt(p.RecipientID, 10, 64)
	if err != nil || recipientID <= 0 || recipientID == initiatorID {
		return 0, nil, ErrCannotContact
	}
	// 接收者存在且未禁用（对外统一不可联系，防枚举）
	peer, err := mysql.GetUserByID(recipientID)
	if err != nil || peer.Status == 0 {
		return 0, nil, ErrCannotContact
	}
	// 屏蔽联动（双向）：任一方屏蔽对方即不可发
	blocked1, _ := mysql.IsBlocked(recipientID, initiatorID)
	blocked2, _ := mysql.IsBlocked(initiatorID, recipientID)
	if blocked1 || blocked2 {
		return 0, nil, ErrCannotContact
	}
	// 隐私策略：nobody 拒绝；following 仅接收方关注发起方时放行（对外统一不可联系）
	policy, _, _, err := mysql.GetMessagePrivacySetting(recipientID)
	if err != nil {
		return 0, nil, err
	}
	if policy == models.DMIncomingNobody {
		return 0, nil, ErrCannotContact
	}
	if policy == models.DMIncomingFollowing {
		following, err := mysql.IsFollowing(recipientID, initiatorID)
		if err != nil {
			return 0, nil, err
		}
		if !following {
			return 0, nil, ErrCannotContact
		}
	}
	// 限流：每账号每小时 20 个新请求
	cnt, err := dmIncrRateLimit("dm:req:"+strconv.FormatInt(initiatorID, 10), 3600)
	if err != nil {
		return 0, nil, err
	}
	if cnt > 20 {
		return 0, nil, ErrDMRateLimited
	}

	// 既有会话：pending 复用（首条消息幂等键更换仍可发一条）；accepted 正常发；declined 冷却由产品配置（v1 允许重新发起，复用会话并重置为 pending）
	var conv *models.Conversation
	conv, err = mysql.GetConversationByPair(initiatorID, recipientID)
	if err != nil && !errors.Is(err, mysql.ErrConversationNotExist) {
		return 0, nil, err
	}
	if conv == nil {
		low, high := initiatorID, recipientID
		if low > high {
			low, high = high, low
		}
		conv = &models.Conversation{
			ConversationID: snowflake.GenerateID(),
			UserLowID:      low,
			UserHighID:     high,
			InitiatorID:    initiatorID,
			State:          models.ConvStatePending,
		}
		if err = mysql.CreateConversation(conv); err != nil {
			return 0, nil, err
		}
	} else {
		switch conv.State {
		case models.ConvStateAccepted:
			// 已接受：直接进入发送流程（会话建立后不再受隐私策略限制）
			msg, err := SendMessageToConversation(initiatorID, conv.ConversationID, &models.ParamSendMessage{
				Body: p.Body, ClientMessageID: p.ClientMessageID,
			})
			if err != nil {
				return conv.ConversationID, nil, err
			}
			return conv.ConversationID, msg, nil
		case models.ConvStateDeclined:
			// 冷却策略 v1：允许重新发起（复用会话，重置为 pending）
			ok, err := mysql.UpdateConversationState(conv.ConversationID, models.ConvStateDeclined, models.ConvStatePending, conv.Version)
			if err != nil {
				return 0, nil, err
			}
			if !ok {
				return 0, nil, ErrStateNotAllowed
			}
			conv.State = models.ConvStatePending
		case models.ConvStatePending:
			// 已有待处理请求：允许发送一条首条消息（幂等键区分）
		default:
			return 0, nil, ErrStateNotAllowed
		}
	}

	// 首条消息（pending 状态写入；限流：会话级每分钟 30 条）
	msg, err := SendMessageToConversation(initiatorID, conv.ConversationID, &models.ParamSendMessage{
		Body: p.Body, ClientMessageID: p.ClientMessageID,
	})
	if err != nil {
		return conv.ConversationID, nil, err
	}
	return conv.ConversationID, msg, nil
}

// SendMessageToConversation 会话内发送消息：成员/状态/屏蔽/限流校验 + 幂等写入
func SendMessageToConversation(senderID int64, conversationID int64, p *models.ParamSendMessage) (*models.DirectMessage, error) {
	conv, err := mysql.GetConversation(conversationID)
	if err != nil {
		if errors.Is(err, mysql.ErrConversationNotExist) {
			return nil, ErrNotConversationPeer
		}
		return nil, err
	}
	isMember, err := mysql.IsConversationMember(conversationID, senderID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, ErrNotConversationPeer
	}
	if conv.State != models.ConvStateAccepted {
		return nil, ErrStateNotAllowed
	}
	peer := mysql.PeerIDOf(conv, senderID)
	// 屏蔽联动：任一方屏蔽即阻止
	b1, _ := mysql.IsBlocked(peer, senderID)
	b2, _ := mysql.IsBlocked(senderID, peer)
	if b1 || b2 {
		return nil, ErrCannotContact
	}
	// 会话级限流：每分钟 30 条（按会话计数）
	cnt, err := dmIncrRateLimit("dm:conv:"+strconv.FormatInt(conversationID, 10), 60)
	if err != nil {
		return nil, err
	}
	if cnt > 30 {
		return nil, ErrDMRateLimited
	}

	msg := &models.DirectMessage{
		MessageID:       snowflake.GenerateID(),
		ConversationID:  conversationID,
		SenderID:        senderID,
		ClientMessageID: p.ClientMessageID,
		Body:            p.Body,
	}
	dup, err := mysql.InsertMessage(msg)
	if err != nil {
		return nil, err
	}
	if dup {
		// 幂等重试：返回已存在消息（按 client 键查回）
		existing, err := mysql.GetMessageByClientKey(conversationID, senderID, p.ClientMessageID)
		if err != nil {
			return nil, err
		}
		return existing, nil
	}
	return msg, nil
}

// AcceptConversation 接受请求（仅接收方；幂等）
func AcceptConversation(userID, conversationID int64) error {
	conv, err := getConversationForPeerCheck(conversationID, userID)
	if err != nil {
		return err
	}
	if conv.State == models.ConvStateAccepted {
		return nil // 幂等
	}
	if conv.State != models.ConvStatePending {
		return ErrStateNotAllowed
	}
	if conv.InitiatorID == userID {
		return ErrNotRequestReceiver
	}
	ok, err := mysql.UpdateConversationState(conversationID, models.ConvStatePending, models.ConvStateAccepted, conv.Version)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAdminConflict // 并发版本冲突
	}
	return nil
}

// DeclineConversation 拒绝请求（仅接收方；幂等）
func DeclineConversation(userID, conversationID int64) error {
	conv, err := getConversationForPeerCheck(conversationID, userID)
	if err != nil {
		return err
	}
	if conv.State == models.ConvStateDeclined {
		return nil
	}
	if conv.State != models.ConvStatePending {
		return ErrStateNotAllowed
	}
	if conv.InitiatorID == userID {
		return ErrNotRequestReceiver
	}
	ok, err := mysql.UpdateConversationState(conversationID, models.ConvStatePending, models.ConvStateDeclined, conv.Version)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAdminConflict
	}
	return nil
}

// CloseConversation 关闭会话（任一成员；幂等）
func CloseConversation(userID, conversationID int64) error {
	conv, err := getConversationForPeerCheck(conversationID, userID)
	if err != nil {
		return err
	}
	if conv.State == models.ConvStateClosed {
		return nil
	}
	ok, err := mysql.UpdateConversationState(conversationID, conv.State, models.ConvStateClosed, conv.Version)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAdminConflict
	}
	return mysql.CloseConversationForMember(conversationID, userID)
}

func getConversationForPeerCheck(conversationID, userID int64) (*models.Conversation, error) {
	conv, err := mysql.GetConversation(conversationID)
	if err != nil {
		if errors.Is(err, mysql.ErrConversationNotExist) {
			return nil, ErrNotConversationPeer
		}
		return nil, err
	}
	isMember, err := mysql.IsConversationMember(conversationID, userID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, ErrNotConversationPeer
	}
	return conv, nil
}

// dmIncrRateLimit 通用限流计数（Redis INCR + TTL；故障时返回 0 不阻塞主流程）
func dmIncrRateLimit(key string, ttlSeconds int) (int64, error) {
	cnt, err := redis.GetClient().Incr(context.Background(), key).Result()
	if err != nil {
		return 0, err
	}
	if cnt == 1 {
		redis.GetClient().Expire(context.Background(), key, time.Duration(ttlSeconds)*time.Second)
	}
	return cnt, nil
}

// DeactivateAccount 注销账号（去标识化；规格 9.5）
func DeactivateAccount(userID int64) error {
	return mysql.DeactivateAccountAnonymized(userID)
}
