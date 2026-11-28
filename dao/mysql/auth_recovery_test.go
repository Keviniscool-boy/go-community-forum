package mysql

import (
	"errors"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// TestResetTokenSingleUse 重置凭据单次消费（需真实 MySQL）
// 规格验收：凭据消费、改密、token_version+1 同事务；重复使用失败；过期失败
func TestResetTokenSingleUse(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "reset")
	switchToTempDB(t, fullName)

	mustExec(t, `CREATE TABLE user (
		id bigint NOT NULL AUTO_INCREMENT, user_id bigint NOT NULL, username varchar(64) NOT NULL,
		email varchar(255) NOT NULL, email_verified tinyint NOT NULL DEFAULT 0, password varchar(64) NOT NULL,
		avatar_url varchar(500) NOT NULL DEFAULT '', bio varchar(500) NOT NULL DEFAULT '',
		status tinyint NOT NULL DEFAULT 1, token_version int NOT NULL DEFAULT 0,
		notify_settings json NULL,
		PRIMARY KEY (id), UNIQUE KEY uk_user_email (email)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE password_reset_token (
		id bigint NOT NULL AUTO_INCREMENT, email varchar(255) NOT NULL, token_hash char(64) NOT NULL,
		expires_at timestamp NOT NULL, used tinyint NOT NULL DEFAULT 0,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (id),
		KEY idx_prt_email_used (email, used, expires_at), KEY idx_prt_token_hash (token_hash)) ENGINE=InnoDB`)

	mustExec(t, "INSERT INTO user (user_id, username, email, email_verified, password, token_version) VALUES (5, 'u', 'r@test.local', 1, 'oldhash', 3)")

	// 写入凭据
	if err := UpsertResetToken("r@test.local", "hash123", time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// 首次消费成功：返回 email，密码更新，token_version 递增
	email, err := ConsumeResetToken("hash123", "$2a$10$newhash")
	if err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if email != "r@test.local" {
		t.Fatalf("email mismatch: %q", email)
	}
	var version int
	var pwd string
	mustQueryRow(t, db, "SELECT token_version FROM user WHERE user_id = 5", &version)
	mustQueryRow(t, db, "SELECT password FROM user WHERE user_id = 5", &pwd)
	if version != 4 || pwd != "$2a$10$newhash" {
		t.Fatalf("password reset incomplete: version=%d pwd=%q", version, pwd)
	}
	// 重复消费失败
	if _, err = ConsumeResetToken("hash123", "x"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("second consume should be invalid, got %v", err)
	}
	// 覆盖写入后旧 hash 失效（同 email 只保留最新）
	if err = UpsertResetToken("r@test.local", "hash456", time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = ConsumeResetToken("hash123", "x"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("old hash should be overwritten, got %v", err)
	}
	if _, err = ConsumeResetToken("hash456", "y"); err != nil {
		t.Fatalf("new hash should work: %v", err)
	}
	// 过期凭据失败
	if err = UpsertResetToken("r@test.local", "expired", time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = ConsumeResetToken("expired", "z"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("expired token should be invalid, got %v", err)
	}
	// 未验证邮箱不可消费改密（email_verified=0 不更新用户行）
	mustExec(t, "INSERT INTO user (user_id, username, email, email_verified, password) VALUES (6, 'legacy', 'legacy_6@placeholder.invalid', 0, 'h')")
	if err = UpsertResetToken("legacy_6@placeholder.invalid", "legacyhash", time.Now().UTC().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = ConsumeResetToken("legacyhash", "w"); err == nil {
		t.Fatal("unverified email should not consume reset")
	}
}

// TestBindEmailConflict 绑定邮箱：唯一约束冲突返回明确错误；成功后 email_verified=1
func TestBindEmailConflict(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "bind")
	switchToTempDB(t, fullName)

	mustExec(t, `CREATE TABLE user (
		id bigint NOT NULL AUTO_INCREMENT, user_id bigint NOT NULL, username varchar(64) NOT NULL,
		email varchar(255) NOT NULL, email_verified tinyint NOT NULL DEFAULT 0, password varchar(64) NOT NULL,
		avatar_url varchar(500) NOT NULL DEFAULT '', bio varchar(500) NOT NULL DEFAULT '',
		status tinyint NOT NULL DEFAULT 1, token_version int NOT NULL DEFAULT 0,
		notify_settings json NULL,
		PRIMARY KEY (id), UNIQUE KEY uk_user_email (email)) ENGINE=InnoDB`)
	mustExec(t, "INSERT INTO user (user_id, username, email, email_verified, password) VALUES "+
		"(1, 'legacy', 'legacy_1@placeholder.invalid', 0, 'h'), (2, 'other', 'taken@test.local', 1, 'h')")

	// 正常绑定
	if err := BindEmail(1, "real@test.local"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	var verified int
	var email string
	mustQueryRow(t, db, "SELECT email_verified, email FROM user WHERE user_id = 1", &verified, &email)
	if verified != 1 || email != "real@test.local" {
		t.Fatalf("bind failed: verified=%d email=%q", verified, email)
	}
	// 绑定被占用邮箱 → ErrEmailTaken
	if err := BindEmail(1, "taken@test.local"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}
