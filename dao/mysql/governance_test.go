package mysql

import (
	"errors"
	"testing"
	"time"

	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// TestModerationVisibilityGovernance 治理状态过滤：隔离/隐藏内容不出现在 Feed（规格 15.1 搜索泄露）
func TestModerationVisibilityGovernance(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "governance")
	switchToTempDB(t, fullName)

	mustExec(t, `CREATE TABLE user (
		id bigint NOT NULL AUTO_INCREMENT, user_id bigint NOT NULL, username varchar(64) NOT NULL,
		email varchar(255) NOT NULL, status tinyint NOT NULL DEFAULT 1, role varchar(16) NOT NULL DEFAULT 'user',
		PRIMARY KEY (id), UNIQUE KEY uk_user_id (user_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE question (
		id bigint NOT NULL AUTO_INCREMENT, question_id bigint NOT NULL,
		title varchar(200) NOT NULL, description text NOT NULL, author_id bigint NOT NULL,
		answer_count int NOT NULL DEFAULT 0, comment_count int NOT NULL DEFAULT 0,
		follower_count int NOT NULL DEFAULT 0, view_count int NOT NULL DEFAULT 0,
		status tinyint NOT NULL DEFAULT 1, moderation_status tinyint NOT NULL DEFAULT 1,
		visibility tinyint NOT NULL DEFAULT 1, deleted_at timestamp NULL DEFAULT NULL,
		updated_by bigint NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY uk_question_id (question_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE answer (
		id bigint NOT NULL AUTO_INCREMENT, answer_id bigint NOT NULL, question_id bigint NOT NULL,
		author_id bigint NOT NULL, content mediumtext NOT NULL,
		vote_count int NOT NULL DEFAULT 0, comment_count int NOT NULL DEFAULT 0,
		collect_count int NOT NULL DEFAULT 0, status tinyint NOT NULL DEFAULT 1,
		moderation_status tinyint NOT NULL DEFAULT 1, visibility tinyint NOT NULL DEFAULT 1,
		deleted_at timestamp NULL DEFAULT NULL, updated_by bigint NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY uk_answer_id (answer_id)) ENGINE=InnoDB`)

	uid := int64(50)
	mustExec(t, "INSERT INTO user (user_id, username, email) VALUES (?, 'author', 'a@t.local')", uid)
	mustExec(t, "INSERT INTO question (question_id, title, description, author_id, moderation_status, visibility) VALUES (1, 'approved-q', 'd', ?, 1, 1)", uid)
	mustExec(t, "INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (2, 'quarantined-q', 'd', ?, 3)", uid)
	mustExec(t, "INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (3, 'rejected-q', 'd', ?, 2)", uid)
	mustExec(t, "INSERT INTO question (question_id, title, description, author_id, visibility) VALUES (4, 'hidden-q', 'd', ?, 0)", uid)

	// Feed 只含 approved 公开内容
	rows, err := LatestFeedPage(time.Time{}, "", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ItemID != 1 {
		t.Fatalf("feed should only contain approved public question, got %d rows", len(rows))
	}

	// 作者被禁用后内容从 Feed 消失
	mustExec(t, "UPDATE user SET status = 0 WHERE user_id = ?", uid)
	rows, err = LatestFeedPage(time.Time{}, "", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("disabled author content must be excluded from feed, got %d rows", len(rows))
	}
	mustExec(t, "UPDATE user SET status = 1 WHERE user_id = ?", uid)
}

// TestReportQueueAndAudit 举报幂等、队列、乐观锁处理与审计
func TestReportQueueAndAudit(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "report")
	switchToTempDB(t, fullName)

	mustExec(t, `CREATE TABLE report (
		id bigint NOT NULL AUTO_INCREMENT, report_id bigint NOT NULL, reporter_id bigint NOT NULL,
		target_type tinyint NOT NULL, target_id bigint NOT NULL, reason_code varchar(32) NOT NULL,
		description varchar(1000) NOT NULL DEFAULT '', status tinyint NOT NULL DEFAULT 0,
		reviewer_id bigint NULL, review_note varchar(1000) NOT NULL DEFAULT '',
		created_at timestamp NULL DEFAULT CURRENT_TIMESTAMP, reviewed_at timestamp NULL DEFAULT NULL,
		PRIMARY KEY (id), UNIQUE KEY uk_report_id (report_id),
		UNIQUE KEY uk_reporter_target (reporter_id, target_type, target_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE audit_log (
		id bigint NOT NULL AUTO_INCREMENT, audit_id bigint NOT NULL, actor_id bigint NULL,
		action varchar(64) NOT NULL, target_type varchar(32) NOT NULL, target_id varchar(64) NOT NULL,
		request_id varchar(64) NOT NULL, ip_hash varchar(128) NOT NULL DEFAULT '',
		metadata_json json NOT NULL, created_at timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY uk_audit_id (audit_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE question (
		id bigint NOT NULL AUTO_INCREMENT, question_id bigint NOT NULL, title varchar(200) NOT NULL,
		description text NOT NULL, author_id bigint NOT NULL, answer_count int NOT NULL DEFAULT 0,
		comment_count int NOT NULL DEFAULT 0, follower_count int NOT NULL DEFAULT 0, view_count int NOT NULL DEFAULT 0,
		status tinyint NOT NULL DEFAULT 1, moderation_status tinyint NOT NULL DEFAULT 1,
		visibility tinyint NOT NULL DEFAULT 1, deleted_at timestamp NULL DEFAULT NULL, updated_by bigint NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id),
		UNIQUE KEY uk_question_id (question_id)) ENGINE=InnoDB`)

	r := &models.Report{
		ReportID:   snowflake.GenerateID(),
		ReporterID: 7,
		TargetType: models.ReportTargetQuestion,
		TargetID:   1,
		ReasonCode: "spam",
	}
	dup, err := CreateReport(r)
	if err != nil {
		t.Fatal(err)
	}
	if dup {
		t.Fatal("first report must not be duplicate")
	}
	// 同用户同目标重复提交（不同 report_id）应幂等去重
	r2 := &models.Report{
		ReportID:   snowflake.GenerateID(),
		ReporterID: 7,
		TargetType: models.ReportTargetQuestion,
		TargetID:   1,
		ReasonCode: "spam",
	}
	dup2, err := CreateReport(r2)
	if err != nil {
		t.Fatal(err)
	}
	if !dup2 {
		t.Fatal("same reporter+target second report should be deduplicated")
	}

	// 审核处理（乐观锁）：第一次成功，第二次并发失败
	ok, err := ResolveReport(r.ReportID, 100, models.ReportStatusResolved, "确认垃圾内容")
	if err != nil || !ok {
		t.Fatalf("first resolve should win: ok=%v err=%v", ok, err)
	}
	ok, err = ResolveReport(r.ReportID, 101, models.ReportStatusDismissed, "")
	if err != nil || ok {
		t.Fatalf("second resolve should lose optimistic lock: ok=%v err=%v", ok, err)
	}

	// 被举报目标内容
	mustExec(t, "INSERT INTO question (question_id, title, description, author_id) VALUES (1, 'q1', 'd', 50)")

	// 管理员隔离内容
	if err := SetModeration(models.ReportTargetQuestion, 1, models.ModerationQuarantined, 100); err != nil {
		t.Fatal(err)
	}
	var mod int32
	mustQueryRow(t, db, "SELECT moderation_status FROM question WHERE question_id = 1", &mod)
	if mod != models.ModerationQuarantined {
		t.Fatalf("moderation_status=%d", mod)
	}

	// 审计日志查询
	if err := CreateAuditLog(&models.AuditLog{
		AuditID: snowflake.GenerateID(), ActorID: 100, Action: "content_moderate",
		TargetType: "question", TargetID: "1", RequestID: "req-1", Metadata: "{}",
	}); err != nil {
		t.Fatalf("create audit log: %v", err)
	}
	logs, err := ListAuditLogs(&AuditLogPageParams{Limit: 10})
	if err != nil || len(logs) == 0 {
		t.Fatalf("audit logs missing: %v", err)
	}
}

// TestEventDedup 推荐事件幂等（同 event_id 重复上报不产生重复行）
func TestEventDedup(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "events")
	switchToTempDB(t, fullName)

	mustExec(t, `CREATE TABLE recommendation_event (
		id bigint NOT NULL AUTO_INCREMENT, event_id bigint NOT NULL, anonymous_id_hash char(64) NOT NULL DEFAULT '',
		user_id bigint NULL, session_id_hash char(64) NOT NULL DEFAULT '',
		event_type varchar(32) NOT NULL, item_type varchar(32) NOT NULL, item_id bigint NOT NULL,
		feed_type varchar(32) NOT NULL DEFAULT '', position int NULL, request_id varchar(64) NOT NULL,
		occurred_at timestamp NULL DEFAULT CURRENT_TIMESTAMP, metadata_json json NOT NULL,
		PRIMARY KEY (id), UNIQUE KEY uk_event_id (event_id)) ENGINE=InnoDB`)

	pos := 3
	events := []*models.RecommendationEvent{{
		EventID: 42, UserID: 1, EventType: "feed_click", ItemType: "question",
		ItemID: 9, FeedType: "latest", Position: &pos, MetadataStr: "{}",
	}}
	if err := CreateRecommendationEvents(events); err != nil {
		t.Fatal(err)
	}
	if err := CreateRecommendationEvents(events); err != nil {
		t.Fatal(err)
	}
	var n int64
	mustQueryRow(t, db, "SELECT COUNT(*) FROM recommendation_event WHERE event_id = 42", &n)
	if n != 1 {
		t.Fatalf("event dedup broken: %d rows", n)
	}
}

var _ = errors.Is
var _ = time.Now
