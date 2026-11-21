package mysql

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// ---------- 消息举报证据（AES-256-GCM 加密快照；密钥与数据库分离） ----------

var ErrEvidenceKeyNotConfigured = errors.New("证据加密密钥未配置")

// evidenceKey 加密密钥（hex 32 字节 = AES-256）；来自 message.evidence_key 配置（部署环境注入）
func evidenceKey() ([]byte, error) {
	keyHex := viper.GetString("message.evidence_key")
	if len(keyHex) < 64 {
		return nil, ErrEvidenceKeyNotConfigured
	}
	return hexDecodeKey(keyHex[:64])
}

func hexDecodeKey(hexStr string) ([]byte, error) {
	b := make([]byte, 32)
	for i := 0; i < 32; i++ {
		hi := hexNibble(hexStr[i*2])
		lo := hexNibble(hexStr[i*2+1])
		if hi < 0 || lo < 0 {
			return nil, ErrEvidenceKeyNotConfigured
		}
		b[i] = byte(hi<<4 | lo)
	}
	return b, nil
}

func hexNibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// encryptEvidence AES-256-GCM 加密（输出 nonce||ciphertext）
func encryptEvidence(plain []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

// SaveMessageEvidence 举报时保存加密消息快照（密钥版本 1；快照不含举报人身份）
func SaveMessageEvidence(reportID, messageID int64, body string, retainUntil time.Time) error {
	key, err := evidenceKey()
	if err != nil {
		// 密钥未配置：证据不落库（举报本身仍成立），记录告警由运维配置后补录
		zap.L().Error("evidence key not configured, skip evidence", zap.Error(err))
		return nil
	}
	enc, err := encryptEvidence([]byte(body), key)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		"INSERT IGNORE INTO message_report_evidence (report_id, message_id, encrypted_snapshot, key_version, retain_until) VALUES (?, ?, ?, 1, ?)",
		reportID, messageID, enc, retainUntil)
	return err
}

// ---------- 事务性 outbox ----------

// OutboxEvent outbox 事件
type OutboxEvent struct {
	EventID     int64
	EventType   string
	AggregateID string
	Payload     string // 合法 JSON；不含私信正文、token、验证码
}

// PendingOutboxEvent 待处理事件（db 标签使 sqlx 正确映射 snake_case 列）
type PendingOutboxEvent struct {
	ID          int64  `db:"id"`
	EventID     int64  `db:"event_id"`
	EventType   string `db:"event_type"`
	AggregateID string `db:"aggregate_id"`
	Payload     string `db:"payload"`
	Attempts    int    `db:"attempts"`
}

// ListPendingOutbox 取待处理事件（到期，按尝试次数上限排除 dead）
func ListPendingOutbox(limit int) ([]PendingOutboxEvent, error) {
	events := make([]PendingOutboxEvent, 0, limit)
	err := db.Select(&events, `SELECT id, event_id, event_type, aggregate_id, payload, attempts
		FROM outbox WHERE status = 0 AND attempts < 5 AND next_attempt_at <= NOW()
		ORDER BY id LIMIT ?`, limit)
	return events, err
}

// MarkOutboxDone 标记完成（幂等）
func MarkOutboxDone(id int64) error {
	_, err := db.Exec("UPDATE outbox SET status = 1 WHERE id = ?", id)
	return err
}

// MarkOutboxFailed 指数退避（attempts+1；attempts>=5 标记 dead）
func MarkOutboxFailed(id int64, lastErr string) error {
	_, err := db.Exec(`UPDATE outbox SET attempts = attempts + 1, last_error = ?,
		next_attempt_at = NOW() + INTERVAL (POW(2, attempts) * 60) SECOND,
		status = IF(attempts + 1 >= 5, 2, 0) WHERE id = ?`, lastErr, id)
	return err
}

// ---------- 内容逐日指标聚合（幂等重算） ----------

// AggregateContentDailyMetrics 将近 N 天事件聚合进 content_daily_metric（metric_version 递增支持重算不翻倍）
// 返回本次使用的 metric_version
func AggregateContentDailyMetrics(days int) (int, error) {
	if days <= 0 || days > 90 {
		days = 30
	}
	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// 新 metric_version = 当前最大 + 1
	var newVersion int
	if err = tx.Get(&newVersion,
		"SELECT COALESCE(MAX(metric_version), 0) + 1 FROM content_daily_metric WHERE computed_at > NOW() - INTERVAL 1 DAY"); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`
		INSERT INTO content_daily_metric (date, content_type, content_id, metric_version, impressions, clicks, views)
		SELECT DATE(occurred_at) AS date, item_type AS content_type, item_id,
			?, 
			COALESCE(SUM(CASE WHEN event_type = 'feed_impression' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN event_type = 'feed_click' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN event_type = 'content_view' THEN 1 ELSE 0 END), 0)
		FROM recommendation_event
		WHERE occurred_at > UTC_TIMESTAMP() - INTERVAL ? DAY
		GROUP BY DATE(occurred_at), item_type, item_id`,
		newVersion, days); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return newVersion, nil
}

// DeleteOldRecommendationEvents 清理 90 天前的原始事件（规格 4.5 保留期）
func DeleteOldRecommendationEvents(days int) (int64, error) {
	result, err := db.Exec(
		"DELETE FROM recommendation_event WHERE occurred_at < UTC_TIMESTAMP() - INTERVAL ? DAY", days)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// DeleteExpiredEvidence 清理过期证据
func DeleteExpiredEvidence() (int64, error) {
	result, err := db.Exec("DELETE FROM message_report_evidence WHERE retain_until < NOW()")
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// ---------- 账号删除去标识化 ----------

// DeactivateAccountAnonymized 删除账号（去标识化）：事务内
//   - 禁用账号、撤销会话（token_version 大幅递增）
//   - 用户名/邮箱替换为不可逆占位（anonymous_<uid>@deleted.invalid）
//   - 内容保留（社区公共记录），作者展示名变为"已注销用户"
func DeactivateAccountAnonymized(userID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	anonName := "anonymous_" + strconv.FormatInt(userID, 10)
	anonEmail := "deleted_" + strconv.FormatInt(userID, 10) + "@deleted.invalid"
	result, err := tx.Exec(
		"UPDATE user SET status = 0, username = ?, email = ?, email_verified = 0, bio = '', avatar_url = '', token_version = token_version + 1000000 WHERE user_id = ?",
		anonName, anonEmail, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrUserNotExist
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// 清缓存
	redisInvalidateTokenVer(userID)
	return nil
}
// redisInvalidateTokenVer 包装 dao/redis 的缓存失效（无包循环：dao/redis 不依赖 dao/mysql）
var invalidateTokenVerCacheFn func(int64)

// SetTokenVerCacheInvalidator 由 main 注入（避免 dao/mysql 直接依赖 dao/redis 的初始化顺序问题）
func SetTokenVerCacheInvalidator(f func(int64)) {
	invalidateTokenVerCacheFn = f
}

func redisInvalidateTokenVer(userID int64) {
	if invalidateTokenVerCacheFn != nil {
		invalidateTokenVerCacheFn(userID)
	}
}

var _ = zap.L

// CreateOutboxEventTx 事务内写入（与业务同事务）
func CreateOutboxEventTx(tx *sqlx.Tx, e *OutboxEvent) error {
	payload := e.Payload
	if payload == "" {
		payload = "{}"
	}
	_, err := tx.Exec(
		"INSERT IGNORE INTO outbox (event_id, event_type, aggregate_id, payload) VALUES (?, ?, ?, CAST(? AS JSON))",
		e.EventID, e.EventType, e.AggregateID, payload)
	return err
}

// CreateOutboxEvent 写入 outbox 事件（幂等键 event_id；非事务路径）
func CreateOutboxEvent(e *OutboxEvent) error {
	payload := e.Payload
	if payload == "" {
		payload = "{}"
	}
	_, err := db.Exec(
		"INSERT IGNORE INTO outbox (event_id, event_type, aggregate_id, payload) VALUES (?, ?, ?, CAST(? AS JSON))",
		e.EventID, e.EventType, e.AggregateID, payload)
	return err
}
