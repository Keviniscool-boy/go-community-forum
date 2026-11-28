package logic

// 私信业务层（第五阶段 PRD F6）
// 规则：拉黑任一方向不可发送；命中拒绝词直接拒绝发送；
// 非互关关系下同一发送方对同一接收方每日上限 20 条（Redis 计数，互关不限制）；
// 通知 type 7，同一会话 10 分钟内多条消息只产生一条未读通知（落库时间窗去重）。

import (
	"context"
	"errors"
	"strconv"
	"time"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
)

var (
	ErrMessageSelf      = errors.New("不能给自己发私信")
	ErrMessageBlocked   = errors.New("无法向该用户发送私信")
	ErrMessageRejected  = errors.New("消息包含违规内容，发送失败")
	ErrMessageRateLimit = errors.New("今日私信已达上限，明天再试吧")
	ErrMessageNotPeer   = errors.New("无权访问该会话")
)

const messageDailyLimit = 20 // 非互关每日上限（PRD F6）

// messageDailyKey 非互关每日计数键（跨天自然重置：key 带日期 + TTL 兜底）
func messageDailyKey(senderID, receiverID int64, now time.Time) string {
	return "dm:day:" + now.Format("20060102") + ":" + strconv.FormatInt(senderID, 10) + ":" + strconv.FormatInt(receiverID, 10)
}

// dmIncrWithLimit Redis 计数限流：返回是否放行
func dmIncrWithLimit(key string, limit int64) (bool, error) {
	ctx := context.Background()
	cnt, err := redis.GetClient().Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if cnt == 1 {
		redis.GetClient().Expire(ctx, key, 48*time.Hour)
	}
	return cnt <= limit, nil
}

// hasMutualFollow 互关判定
func hasMutualFollow(a, b int64) (bool, error) {
	f1, err := mysql.IsFollowing(a, b)
	if err != nil {
		return false, err
	}
	f2, err := mysql.IsFollowing(b, a)
	if err != nil {
		return false, err
	}
	return f1 && f2, nil
}

// hasUnreadInviteNotification 同会话 10 分钟内已有未读私信通知（时间窗去重）
func hasRecentUnreadDMNotification(tx mysql.TxLike, userID, threadID int64) (bool, error) {
	var n int
	err := tx.Get(&n,
		"SELECT COUNT(*) FROM notification WHERE user_id = ? AND type = ? AND target_id = ? AND is_read = 0"+
			" AND create_time > NOW() - INTERVAL 10 MINUTE",
		userID, models.NotifyTypeDM, threadID)
	return n > 0, err
}

// notifyNewMessage 新私信通知（type 7；时间窗聚合）
func notifyNewMessageTx(tx mysql.TxLike, receiverID, senderID, threadID int64) error {
	recent, err := hasRecentUnreadDMNotification(tx, receiverID, threadID)
	if err != nil {
		return err
	}
	if recent {
		return nil
	}
	return mysql.CreateNotificationTx(tx, receiverID, senderID, models.NotifyTypeDM, models.TargetQuestion, threadID)
}

// OpenDirectMessage 与某用户开聊（已存在会话则追加；PRD F6）
func OpenDirectMessage(senderID int64, targetIDStr string, p *models.ParamSendMessage) (*models.ApiMessageDetail, int64, error) {
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil || targetID <= 0 {
		return nil, 0, mysql.ErrInvalidParams
	}
	if targetID == senderID {
		return nil, 0, ErrMessageSelf
	}
	return sendDM(senderID, targetID, p, 0)
}

// ReplyInThread 会话内发消息
func ReplyInThread(senderID int64, threadIDStr string, p *models.ParamSendMessage) (*models.ApiMessageDetail, int64, error) {
	threadID, err := strconv.ParseInt(threadIDStr, 10, 64)
	if err != nil || threadID <= 0 {
		return nil, 0, mysql.ErrInvalidParams
	}
	thread, err := mysql.GetThreadByID(threadID)
	if err != nil {
		if errors.Is(err, mysql.ErrThreadNotExist) {
			return nil, 0, ErrMessageNotPeer
		}
		return nil, 0, err
	}
	if thread.UserA != senderID && thread.UserB != senderID {
		return nil, 0, ErrMessageNotPeer
	}
	peer := thread.UserA
	if thread.UserB != senderID {
		peer = thread.UserB
	}
	return sendDM(senderID, peer, p, threadID)
}

// sendDM 私信发送公共实现：拉黑/审核/限流 → 事务写消息 → 通知
func sendDM(senderID, receiverID int64, p *models.ParamSendMessage, knownThreadID int64) (*models.ApiMessageDetail, int64, error) {
	// 拉黑（双向任一）不可发送
	if blocked, err := mysql.IsBlockedBetween(senderID, receiverID); err != nil {
		return nil, 0, err
	} else if blocked {
		return nil, 0, ErrMessageBlocked
	}
	// 接收方必须有效
	if _, uerr := mysql.GetUserByID(receiverID); uerr != nil {
		return nil, 0, mysql.ErrUserNotExist
	}
	// 敏感词：命中拒绝词直接拒绝发送（PRD F6 与评论同一策略）
	decision, err := CheckContentModeration(p.Content)
	if err != nil {
		return nil, 0, err
	}
	if decision.Status == models.ModerationPending {
		return nil, 0, ErrMessageRejected
	}
	// 非互关每日 20 条（互关不限制）
	mutual, err := hasMutualFollow(senderID, receiverID)
	if err != nil {
		return nil, 0, err
	}
	if !mutual {
		ok, err := dmIncrWithLimit(messageDailyKey(senderID, receiverID, time.Now().UTC()), messageDailyLimit)
		if err != nil {
			return nil, 0, err
		}
		if !ok {
			return nil, 0, ErrMessageRateLimit
		}
	}

	// 事务：查/建会话 + 写消息 + 通知
	var msg *models.Message
	var threadID int64
	txErr := mysql.WithTx(func(tx mysql.TxLike) error {
		var tErr error
		var existing *models.MessageThread
		if knownThreadID > 0 {
			existing, tErr = mysql.GetThreadByIDTx(tx, knownThreadID)
		} else {
			existing, tErr = mysql.FindThreadByPairTx(tx, senderID, receiverID)
		}
		if tErr != nil {
			return tErr
		}
		if existing != nil {
			threadID = existing.ThreadID
		}
		if existing == nil {
			msg, threadID, tErr = mysql.CreateThreadWithMessageTx(tx, senderID, receiverID, senderID, p.Content)
			if tErr != nil {
				return tErr
			}
		} else {
			msg, tErr = mysql.SendMessageInThreadTx(tx, threadID, senderID, p.Content, time.Now().UTC())
			if tErr != nil {
				return tErr
			}
		}
		// 通知接收者（type 7；10 分钟窗去重）
		if nErr := notifyNewMessageTx(tx, receiverID, senderID, threadID); nErr != nil {
			return nErr
		}
		return nil
	})
	if txErr != nil {
		return nil, 0, txErr
	}
	sender, gerr := mysql.GetUserByID(senderID)
	if gerr != nil {
		return nil, 0, gerr
	}
	return &models.ApiMessageDetail{Message: msg, SenderName: sender.Username}, threadID, nil
}

// ListMyThreads 我的会话列表（cursor 分页）
func ListMyThreads(userID int64, cursor string, limit int) (*models.PageResult[*models.MessageThread], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var cursorTime time.Time
	var cursorID int64
	if cursor != "" {
		c, err := parseTimeCursor(cursor)
		if err != nil {
			return nil, ErrCursorInvalid
		}
		cursorTime, cursorID = c.Time, c.ID
	}
	items, err := mysql.ListThreads(userID, cursorTime, cursorID, limit)
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(items) > limit {
		hasMore = true
		items = items[:limit]
	}
	next := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		if last.LastMessageAt != nil {
			next = encodeTimeCursor(*last.LastMessageAt, last.ThreadID)
		}
	}
	return &models.PageResult[*models.MessageThread]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// GetThreadMessages 会话消息（最早在前；打开即更新已读水位）
func GetThreadMessages(userID int64, threadIDStr string, cursor string, limit int) (*models.PageResult[*models.ApiMessageDetail], error) {
	threadID, err := strconv.ParseInt(threadIDStr, 10, 64)
	if err != nil || threadID <= 0 {
		return nil, mysql.ErrInvalidParams
	}
	thread, err := mysql.GetThreadByID(threadID)
	if err != nil {
		if errors.Is(err, mysql.ErrThreadNotExist) {
			return nil, ErrMessageNotPeer
		}
		return nil, err
	}
	if thread.UserA != userID && thread.UserB != userID {
		return nil, ErrMessageNotPeer
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 50 {
		limit = 50
	}
	var cursorTime time.Time
	var cursorID int64
	if cursor != "" {
		c, cerr := parseTimeCursor(cursor)
		if cerr != nil {
			return nil, ErrCursorInvalid
		}
		cursorTime, cursorID = c.Time, c.ID
	}
	items, err := mysql.ListThreadMessages(threadID, cursorTime, cursorID, limit)
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(items) > 0 {
		first := items[0]
		hasMore, _ = mysql.HasMoreOlderMessages(threadID, first.CreateTime, first.MessageID)
	}
	// 打开会话即更新我的已读水位
	if uerr := mysql.MarkThreadRead(threadID, userID); uerr != nil {
		return nil, uerr
	}
	next := ""
	if hasMore && len(items) > 0 {
		first := items[0]
		next = encodeTimeCursor(first.CreateTime, first.MessageID)
	}
	return &models.PageResult[*models.ApiMessageDetail]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// DeleteMyThread 删除会话（仅自己视角隐藏）
func DeleteMyThread(userID int64, threadIDStr string) error {
	threadID, err := strconv.ParseInt(threadIDStr, 10, 64)
	if err != nil || threadID <= 0 {
		return mysql.ErrInvalidParams
	}
	thread, err := mysql.GetThreadByID(threadID)
	if err != nil {
		if errors.Is(err, mysql.ErrThreadNotExist) {
			return ErrMessageNotPeer
		}
		return err
	}
	if thread.UserA != userID && thread.UserB != userID {
		return ErrMessageNotPeer
	}
	return mysql.SoftDeleteThread(threadID, userID)
}

// CountMyMessageUnread TopNav 角标
func CountMyMessageUnread(userID int64) (int64, error) {
	return mysql.CountThreadUnread(userID)
}

