package logic

// B07：事务性 outbox 的后台 worker——claim、处理、重试、死信与保留期清理。
// 事件由业务同事务写入（最少一次投递）；worker 单实例顺序处理，多实例时依赖
// outbox 行级状态 CAS claim（status 0→3），不会重复投递同一事件。

import (
	"context"
	"time"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"
	"go.uber.org/zap"
)

// outbox 状态（与 012 迁移对齐；3 = processing 由 worker claim 使用）
const (
	outboxStatusPending    = 0
	outboxStatusDone       = 1
	outboxStatusDead       = 2
	outboxStatusProcessing = 3
)

// ClaimOutboxEvent CAS 认领：status 0→3，单 worker 顺序处理；多实例也不会重复
func ClaimOutboxEvent(id int64) (bool, error) {
	return mysql.ClaimOutboxEvent(id)
}

// processOutboxEvent 处理单个事件（当前事件类型：私信通知）
func processOutboxEvent(e *mysql.PendingOutboxEvent) error {
	switch e.EventType {
	case "dm_message_sent":
		return deliverDMNotification(e)
	default:
		// 未知类型按已处理处理（避免死信堆积）
		return nil
	}
}

// deliverDMNotification 私信通知：接收者收到一条 type=dm 的通知（不含正文）
func deliverDMNotification(e *mysql.PendingOutboxEvent) error {
	var payload struct {
		MessageID int64 `json:"message_id"`
	}
	if err := jsonUnmarshalPayload(e.Payload, &payload); err != nil {
		return err // payload 非法：重试也不会成功，由上层死信
	}
	m, err := mysql.GetMessage(payload.MessageID)
	if err != nil {
		// 消息已不存在：视为完成
		return nil
	}
	conv, err := mysql.GetConversation(m.ConversationID)
	if err != nil {
		return err
	}
	peer := mysql.PeerIDOf(conv, m.SenderID)
	n := &models.Notification{
		NotificationID: snowflake.GenerateID(),
		UserID:         peer,
		ActorID:        m.SenderID,
		Type:           models.NotifyTypeDM,
		TargetType:     0,
		TargetID:       m.MessageID,
	}
	return mysql.CreateNotification(n)
}

// jsonUnmarshalPayload 解析事件 payload
func jsonUnmarshalPayload(payload string, v interface{}) error {
	return unmarshalJSONMust([]byte(payload), v)
}

// StartOutboxWorker 启动 outbox 后台 worker（阻塞；ctx 取消退出）
func StartOutboxWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOutboxBatch()
		}
	}
}

// runOutboxBatch 处理一批待处理事件
func runOutboxBatch() {
	events, err := mysql.ListPendingOutbox(50)
	if err != nil {
		zap.L().Error("outbox: list pending failed", zap.Error(err))
		return
	}
	for _, e := range events {
		claimed, err := ClaimOutboxEvent(e.ID)
		if err != nil {
			zap.L().Error("outbox: claim failed", zap.Int64("id", e.ID), zap.Error(err))
			continue
		}
		if !claimed {
			continue // 被其他 worker 认领
		}
		pe := &mysql.PendingOutboxEvent{ID: e.ID, EventID: e.EventID, EventType: e.EventType, AggregateID: e.AggregateID, Payload: e.Payload, Attempts: e.Attempts}
		if err := processOutboxEvent(pe); err != nil {
			zap.L().Error("outbox: process failed", zap.Int64("id", e.ID), zap.Error(err))
			if err := mysql.MarkOutboxFailed(e.ID, err.Error()); err != nil {
				zap.L().Error("outbox: mark failed", zap.Error(err))
			}
			continue
		}
		if err := mysql.MarkOutboxDone(e.ID); err != nil {
			zap.L().Error("outbox: mark done failed", zap.Error(err))
		}
	}
}

// StartRetentionWorker 启动保留期清理 worker（90 天事件 + 过期证据）
func StartRetentionWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := mysql.DeleteOldRecommendationEvents(90); err != nil {
				zap.L().Error("retention: delete old events failed", zap.Error(err))
			} else if n > 0 {
				zap.L().Info("retention: old events deleted", zap.Int64("count", n))
			}
			if n, err := mysql.DeleteExpiredEvidence(); err != nil {
				zap.L().Error("retention: delete expired evidence failed", zap.Error(err))
			} else if n > 0 {
				zap.L().Info("retention: expired evidence deleted", zap.Int64("count", n))
			}
		}
	}
}

