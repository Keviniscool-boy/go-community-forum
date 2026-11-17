package logic

// 流程级集成测试（P0 修复验收：B01/B02/B03/B06）。
// 环境：viper 指向测试 MySQL/Redis；每个用例独立临时库（SetGlobalDB 指向）。

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/snowflake"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/viper"
	_ "github.com/go-sql-driver/mysql"
)

var (
	flowDSN       = os.Getenv("BB_TEST_MYSQL_DSN")
	flowRedisAddr = os.Getenv("BB_TEST_REDIS_ADDR")
	flowInitOnce  sync.Once
	flowInitErr   error
)

// requireFlowEnv 初始化 viper/Redis 全局客户端与 snowflake（logic 函数依赖）
func requireFlowEnv(t *testing.T) {
	t.Helper()
	if flowDSN == "" || flowRedisAddr == "" {
		t.Skip("BB_TEST_MYSQL_DSN/BB_TEST_REDIS_ADDR not set, skip flow integration test")
	}
	flowInitOnce.Do(func() {
		// flowRedisAddr 形如 127.0.0.1:26379；拆出 host 与 port
		host, port := "127.0.0.1", "26379"
		if idx := strings.LastIndex(flowRedisAddr, ":"); idx >= 0 {
			host = flowRedisAddr[:idx]
			port = flowRedisAddr[idx+1:]
		}
		viper.Set("redis.host", host)
		viper.Set("redis.port", port)
		viper.Set("redis.db", 0)
		flowInitErr = redis.Init()
		if flowInitErr != nil {
			return
		}
		// 测试隔离：清空共享 Redis 实例（限流键跨运行残留会导致假阳性）
		if err := redis.GetClient().FlushDB(context.Background()).Err(); err != nil {
			flowInitErr = err
			return
		}
		flowInitErr = snowflake.Init("2026-07-24 00:00:00", 1)
	})
	if flowInitErr != nil {
		t.Fatalf("init flow env failed: %v", flowInitErr)
	}
}

// rootDB 根连接（建临时库用）
func rootDB(t *testing.T) *sqlx.DB {
	t.Helper()
	root, err := sqlx.Connect("mysql", flowDSN)
	if err != nil {
		t.Fatalf("connect root failed: %v", err)
	}
	t.Cleanup(func() { root.Close() })
	return root
}

// flowTempDB 建临时库、把全局 DB 指向它，并返回该连接
func flowTempDB(t *testing.T, name string) *sqlx.DB {
	t.Helper()
	root := rootDB(t)
	fullName := "bb_flow_test_" + name
	mustExecFlow(t, root, "DROP DATABASE IF EXISTS "+fullName)
	mustExecFlow(t, root, "CREATE DATABASE "+fullName+" DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci")
	t.Cleanup(func() {
		mustExecFlow(t, root, "DROP DATABASE IF EXISTS "+fullName)
	})
	temp, err := sqlx.Connect("mysql", flowDSN+fullName+"?charset=utf8mb4&parseTime=True&loc=UTC")
	if err != nil {
		t.Fatalf("connect temp db failed: %v", err)
	}
	t.Cleanup(func() { temp.Close() })
	mysql.SetGlobalDB(temp)
	return temp
}

func mustExecFlow(t *testing.T, d *sqlx.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := d.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// createDMTables 私信域最小表
func createDMTables(t *testing.T, d *sqlx.DB) {
	mustExecFlow(t, d, `CREATE TABLE user (
		id bigint NOT NULL AUTO_INCREMENT, user_id bigint NOT NULL, username varchar(64) NOT NULL,
		email varchar(255) NOT NULL, status tinyint NOT NULL DEFAULT 1, role varchar(16) NOT NULL DEFAULT 'user',
		PRIMARY KEY (id), UNIQUE KEY uk_user_id (user_id)) ENGINE=InnoDB`)
	mustExecFlow(t, d, `CREATE TABLE user_follow (
		id bigint NOT NULL AUTO_INCREMENT, follower_id bigint NOT NULL, following_id bigint NOT NULL,
		PRIMARY KEY (id), UNIQUE KEY uk_user_follow (follower_id, following_id)) ENGINE=InnoDB`)
	mustExecFlow(t, d, `CREATE TABLE user_block (
		id bigint NOT NULL AUTO_INCREMENT, blocker_id bigint NOT NULL, blocked_id bigint NOT NULL,
		PRIMARY KEY (id), UNIQUE KEY uk_user_block (blocker_id, blocked_id)) ENGINE=InnoDB`)
	mustExecFlow(t, d, `CREATE TABLE message_privacy_setting (
		id bigint NOT NULL AUTO_INCREMENT, user_id bigint NOT NULL, incoming_policy varchar(16) NOT NULL DEFAULT 'following',
		read_receipts_enabled tinyint NOT NULL DEFAULT 1, version bigint NOT NULL DEFAULT 1,
		PRIMARY KEY (id), UNIQUE KEY uk_privacy_user (user_id)) ENGINE=InnoDB`)
	mustExecFlow(t, d, `CREATE TABLE conversation (
		id bigint NOT NULL AUTO_INCREMENT, conversation_id bigint NOT NULL, user_low_id bigint NOT NULL,
		user_high_id bigint NOT NULL, initiator_id bigint NOT NULL, state tinyint NOT NULL DEFAULT 0,
		version bigint NOT NULL DEFAULT 1, declined_at timestamp NULL DEFAULT NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		update_time timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY uk_pair (user_low_id, user_high_id)) ENGINE=InnoDB`)
	mustExecFlow(t, d, `CREATE TABLE conversation_member (
		id bigint NOT NULL AUTO_INCREMENT, conversation_id bigint NOT NULL, user_id bigint NOT NULL,
		last_read_message_id bigint NOT NULL DEFAULT 0, PRIMARY KEY (id),
		UNIQUE KEY uk_conv_member (conversation_id, user_id)) ENGINE=InnoDB`)
	mustExecFlow(t, d, `CREATE TABLE direct_message (
		id bigint NOT NULL AUTO_INCREMENT, message_id bigint NOT NULL, conversation_id bigint NOT NULL,
		sender_id bigint NOT NULL, client_message_id varchar(64) NOT NULL, body varchar(2000) NOT NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id),
		UNIQUE KEY uk_client_msg (conversation_id, sender_id, client_message_id)) ENGINE=InnoDB`)
}

// TestB01BlockedFailClosed B01：屏蔽后发起被拦截（ErrCannotContact）
func TestB01BlockedFailClosed(t *testing.T) {
	requireFlowEnv(t)
	temp := flowTempDB(t, "b01")
	createDMTables(t, temp)
	mustExecFlow(t, temp, "INSERT INTO user (user_id, username, email) VALUES (1,'a','a@t'),(2,'b','b@t')")
	mustExecFlow(t, temp, "INSERT INTO user_block (blocker_id, blocked_id) VALUES (2, 1)")

	_, _, err := StartConversation(1, &models.ParamStartConversation{
		RecipientID: "2", Body: "hi", ClientMessageID: "c1",
	})
	if !errors.Is(err, ErrCannotContact) {
		t.Fatalf("expected ErrCannotContact, got %v", err)
	}
}

// TestB02PendingSingleFirstMessage B02：pending 会话只允许一条首信；
// 同 client_message_id 幂等返回原消息；换键重发被拒；拒绝后 7 天冷却
func TestB02PendingSingleFirstMessage(t *testing.T) {
	requireFlowEnv(t)
	temp := flowTempDB(t, "b02pending")
	createDMTables(t, temp)
	mustExecFlow(t, temp, "INSERT INTO user (user_id, username, email) VALUES (1,'a','a@t'),(2,'b','b@t')")
	mustExecFlow(t, temp, "INSERT INTO user_follow (follower_id, following_id) VALUES (2, 1)")

	convID, msg, err := StartConversation(1, &models.ParamStartConversation{
		RecipientID: "2", Body: "first", ClientMessageID: "m1"})
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	// 同 client 键重试：幂等返回原消息
	convID2, msg2, err := StartConversation(1, &models.ParamStartConversation{
		RecipientID: "2", Body: "first", ClientMessageID: "m1"})
	if err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if convID2 != convID || msg2.MessageID != msg.MessageID {
		t.Fatal("idempotent retry should return original message")
	}
	// 换 client 键再发：pending 状态下被拒
	_, _, err = StartConversation(1, &models.ParamStartConversation{
		RecipientID: "2", Body: "second", ClientMessageID: "m2"})
	if !errors.Is(err, ErrDMWaitingForAccept) {
		t.Fatalf("second first-message should be ErrDMWaitingForAccept, got %v", err)
	}

	// 接受后正常发送
	if err := AcceptConversation(2, convID); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, err := SendMessageToConversation(2, convID, &models.ParamSendMessage{
		Body: "reply", ClientMessageID: "r1"}); err != nil {
		t.Fatalf("reply after accept: %v", err)
	}

	// 拒绝冷却：新建会话 → 接收方拒绝 → 立即重发起被冷却拒绝
	mustExecFlow(t, temp, "INSERT INTO user (user_id, username, email) VALUES (3,'c','c@t')")
	mustExecFlow(t, temp, "INSERT INTO user_follow (follower_id, following_id) VALUES (3, 1), (1, 3)")
	conv3, _, err := StartConversation(3, &models.ParamStartConversation{
		RecipientID: "3", Body: "hello", ClientMessageID: "m3"})
	if err != nil {
		t.Fatalf("conv3 start: %v", err)
	}
	if err := DeclineConversation(1, conv3); err != nil {
		t.Fatalf("decline: %v", err)
	}
	_, _, err = StartConversation(3, &models.ParamStartConversation{
		RecipientID: "1", Body: "again", ClientMessageID: "again1"})
	if !errors.Is(err, ErrDMCooldown) {
		t.Fatalf("cooldown expected, got %v", err)
	}
	// 冷却过期（declined_at 拨回 8 天前）后可重新发起
	mustExecFlow(t, temp,
		"UPDATE conversation SET declined_at = UTC_TIMESTAMP() - INTERVAL 8 DAY WHERE conversation_id = ?", conv3)
	_, _, err = StartConversation(3, &models.ParamStartConversation{
		RecipientID: "1", Body: "after cooldown", ClientMessageID: "after1"})
	if err != nil {
		t.Fatalf("re-initiate after cooldown: %v", err)
	}
}

// TestB03BaseVersionConflict B03：恢复历史版本 baseVersion 不匹配时冲突
func TestB03BaseVersionConflict(t *testing.T) {
	requireFlowEnv(t)
	temp := flowTempDB(t, "b03")
	createArticleTables(t, temp)
	mustExecFlow(t, temp, "INSERT INTO user (user_id, username, email) VALUES (1,'author','a@t')")
	mustExecFlow(t, temp, "INSERT INTO article (article_id, author_id, title, content) VALUES (10, 1, 't', 'v1')")

	rev1, err := SaveRevision(1, 10, "t", "v1")
	if err != nil {
		t.Fatal(err)
	}
	rev2, err := SaveRevision(1, 10, "t", "v2")
	if err != nil {
		t.Fatal(err)
	}
	// baseVersion 错填 1（当前最新是 2）→ 冲突
	_, err = RestoreRevision(1, 10, rev1.RevisionID, 1)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected ErrRevisionConflict, got %v", err)
	}
	// baseVersion 正确（2）→ 恢复成功产生 v3
	rev3, err := RestoreRevision(1, 10, rev2.RevisionID, 2)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if rev3.RevisionNo != 3 {
		t.Fatalf("restore should create revision 3, got %d", rev3.RevisionNo)
	}
}

// TestB06ConcurrentReviewPending B06：同文章同版本并发提交审阅，只有一个 pending
func TestB06ConcurrentReviewPending(t *testing.T) {
	requireFlowEnv(t)
	temp := flowTempDB(t, "b06")
	createArticleTables(t, temp)
	mustExecFlow(t, temp, `CREATE TABLE review_request (
		id bigint NOT NULL AUTO_INCREMENT, review_id bigint NOT NULL, article_id bigint NOT NULL,
		revision_id bigint NOT NULL, state tinyint NOT NULL DEFAULT 0, initiator_id bigint NOT NULL,
		PRIMARY KEY (id), UNIQUE KEY uk_review_id (review_id),
		UNIQUE KEY uk_review_pending (article_id, revision_id, state)) ENGINE=InnoDB`)
	mustExecFlow(t, temp, `CREATE TABLE article_review_pending (
		article_id bigint NOT NULL, review_id bigint NOT NULL, revision_id bigint NOT NULL,
		PRIMARY KEY (article_id)) ENGINE=InnoDB`)
	mustExecFlow(t, temp, "INSERT INTO user (user_id, username, email) VALUES (1,'a','a@t'),(2,'e','e@t')")
	mustExecFlow(t, temp, "INSERT INTO article (article_id, author_id, title, content) VALUES (10, 1, 't', 'c')")
	mustExecFlow(t, temp, "INSERT INTO column_info (column_id, owner_id, name) VALUES (5, 1, 'col')")
	mustExecFlow(t, temp, "INSERT INTO column_member (column_id, user_id, role) VALUES (5, 1, 'owner'), (5, 2, 'editor')")
	rev, err := SaveRevision(1, 10, "t", "v1")
	if err != nil {
		t.Fatal(err)
	}

	// 两个 goroutine 并发提交同一版本审阅
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := SubmitReview(1, 10, strconv.FormatInt(rev.RevisionID, 10))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	successes, pendings := 0, 0
	for err := range errs {
		if err == nil {
			successes++
		} else if errors.Is(err, mysql.ErrReviewPending) {
			pendings++
		}
	}
	if successes != 1 || pendings != 1 {
		t.Fatalf("concurrent submit: successes=%d pendings=%d (want 1/1)", successes, pendings)
	}
}

// createArticleTables 文章协作域最小表
func createArticleTables(t *testing.T, d *sqlx.DB) {
	mustExecFlow(t, d, `CREATE TABLE user (
		id bigint NOT NULL AUTO_INCREMENT, user_id bigint NOT NULL, username varchar(64) NOT NULL,
		email varchar(255) NOT NULL, status tinyint NOT NULL DEFAULT 1, role varchar(16) NOT NULL DEFAULT 'user',
		PRIMARY KEY (id), UNIQUE KEY uk_user_id (user_id)) ENGINE=InnoDB`)
	mustExecFlow(t, d, `CREATE TABLE article (
		id bigint NOT NULL AUTO_INCREMENT, article_id bigint NOT NULL, author_id bigint NOT NULL,
		column_id bigint NULL, title varchar(200) NOT NULL, content mediumtext NOT NULL,
		view_count int NOT NULL DEFAULT 0, status tinyint NOT NULL DEFAULT 1,
		moderation_status tinyint NOT NULL DEFAULT 1, visibility tinyint NOT NULL DEFAULT 1,
		deleted_at timestamp NULL DEFAULT NULL, collab_state tinyint NOT NULL DEFAULT 0,
		approved_revision_id bigint NULL, updated_by bigint NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		update_time timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY uk_article_id (article_id)) ENGINE=InnoDB`)
	mustExecFlow(t, d, `CREATE TABLE column_info (
		id bigint NOT NULL AUTO_INCREMENT, column_id bigint NOT NULL, owner_id bigint NOT NULL,
		name varchar(64) NOT NULL, description varchar(1000) NOT NULL DEFAULT '',
		status tinyint NOT NULL DEFAULT 1, moderation_status tinyint NOT NULL DEFAULT 1,
		visibility tinyint NOT NULL DEFAULT 1, deleted_at timestamp NULL DEFAULT NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		update_time timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY uk_column_id (column_id)) ENGINE=InnoDB`)
	mustExecFlow(t, d, `CREATE TABLE column_member (
		id bigint NOT NULL AUTO_INCREMENT, column_id bigint NOT NULL, user_id bigint NOT NULL,
		role varchar(16) NOT NULL, PRIMARY KEY (id),
		UNIQUE KEY uk_column_member (column_id, user_id)) ENGINE=InnoDB`)
	mustExecFlow(t, d, `CREATE TABLE article_revision (
		id bigint NOT NULL AUTO_INCREMENT, revision_id bigint NOT NULL, article_id bigint NOT NULL,
		revision_no bigint NOT NULL, title varchar(200) NOT NULL, body mediumtext NOT NULL,
		author_id bigint NOT NULL, content_hash char(64) NOT NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id),
		UNIQUE KEY uk_revision_no (article_id, revision_no)) ENGINE=InnoDB`)
}
