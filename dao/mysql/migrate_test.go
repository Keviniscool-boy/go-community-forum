package mysql

import (
	"fmt"
	"os"
	"testing"

	"bluebell/pkg/snowflake"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// 迁移链集成测试：需要真实 MySQL，通过环境变量 BB_TEST_MYSQL_DSN 启用。
// DSN 只写到根路径、不带查询串，例如：
//
//	BB_TEST_MYSQL_DSN='root:pass@tcp(127.0.0.1:33061)/'
//
// 测试在临时数据库上执行（每个用例独立建库/删库），不触碰现有数据。
var testDSN = os.Getenv("BB_TEST_MYSQL_DSN")

// tempDBDSN 拼接临时库 DSN（库名后追加连接参数）
func tempDBDSN(name string) string {
	return testDSN + name + "?charset=utf8mb4&parseTime=True&loc=UTC"
}

func requireMySQL(t *testing.T) *sqlx.DB {
	t.Helper()
	if testDSN == "" {
		t.Skip("BB_TEST_MYSQL_DSN not set, skip integration test")
	}
	// 通知等 dao 层写操作使用雪花 ID，集成测试环境统一初始化
	if err := snowflake.Init("2026-07-24 00:00:00", 1); err != nil {
		t.Fatalf("init snowflake failed: %v", err)
	}
	root, err := sqlx.Connect("mysql", testDSN)
	if err != nil {
		t.Fatalf("connect test mysql failed: %v", err)
	}
	t.Cleanup(func() { root.Close() })
	return root
}

// createTempDB 创建一个全新的临时库并返回
func createTempDB(t *testing.T, db *sqlx.DB, name string) string {
	t.Helper()
	fullName := fmt.Sprintf("bb_migration_test_%s", name)
	db.MustExec("DROP DATABASE IF EXISTS " + fullName)
	db.MustExec("CREATE DATABASE " + fullName + " DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci")
	t.Cleanup(func() { db.MustExec("DROP DATABASE IF EXISTS " + fullName) })
	return fullName
}

// switchToTempDB 把全局 db 指向临时库（迁移器使用全局 db 执行）
func switchToTempDB(t *testing.T, fullName string) {
	t.Helper()
	temp, err := sqlx.Connect("mysql", tempDBDSN(fullName))
	if err != nil {
		t.Fatalf("connect temp db failed: %v", err)
	}
	t.Cleanup(func() { temp.Close() })
	setGlobalDB(temp)
}

// columnExists 检查列是否存在
func columnExists(t *testing.T, d *sqlx.DB, table, column string) bool {
	var n int
	err := d.Get(&n, `SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, column)
	if err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// TestMigrationsFreshDB 空库跑完整迁移链：结构完整、重复执行幂等
func TestMigrationsFreshDB(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "fresh")
	switchToTempDB(t, fullName)

	if err := RunMigrations(); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := RunMigrations(); err != nil {
		t.Fatalf("second run (idempotent): %v", err)
	}

	// 关键列与表
	for _, tc := range []struct{ table, column string }{
		{"user", "email"}, {"user", "email_verified"}, {"user", "token_version"},
		{"user", "avatar_url"}, {"user", "status"},
	} {
		if !columnExists(t, db, tc.table, tc.column) {
			t.Fatalf("column %s.%s missing", tc.table, tc.column)
		}
	}
	for _, table := range []string{"user", "community", "post", "topic", "question", "answer",
		"question_topic", "user_follow", "topic_follow", "question_follow",
		"answer_vote", "answer_collect", "comment", "comment_like", "notification", "schema_migrations"} {
		var n int
		if err := db.Get(&n, `SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table); err != nil || n != 1 {
			t.Fatalf("table %s missing (err=%v)", table, err)
		}
	}

	// 版本记录 000-008（新增迁移后同步更新数量断言）
	var versions []int
	if err := db.Select(&versions, "SELECT version FROM schema_migrations ORDER BY version"); err != nil {
		t.Fatal(err)
	}
	if len(versions) != 11 {
		t.Fatalf("expected 11 migrations recorded, got %v", versions)
	}

	// 基线社区种子存在
	var seedCount int
	if err := db.Get(&seedCount, "SELECT COUNT(community_id) FROM community"); err != nil || seedCount != 4 {
		t.Fatalf("community seed missing: count=%d err=%v", seedCount, err)
	}
}

// TestMigrationsLegacyDB 旧库升级：旧结构 + NULL 邮箱用户 → 升级后可用且不误标验证
func TestMigrationsLegacyDB(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "legacy")
	switchToTempDB(t, fullName)

	// 模拟旧库：只建旧结构（迁移链之前的状态）
	mustExec(t, `CREATE TABLE user (
		id bigint(20) NOT NULL AUTO_INCREMENT,
		username varchar(64) NOT NULL,
		email varchar(255) DEFAULT NULL,
		password varchar(64) NOT NULL,
		user_id bigint(20) NOT NULL,
		gender tinyint(4) NOT NULL DEFAULT 0,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		update_time timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY idx_username (username), UNIQUE KEY idx_user_id (user_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	mustExec(t, `INSERT INTO user (user_id, username, email, password) VALUES
		(101, 'alice', NULL, '$2a$10$dummyhashdummyhashdummyhashdummyhashdummyhashdummy'),
		(102, 'bob', 'bob@real.example', '$2a$10$dummyhashdummyhashdummyhashdummyhashdummy')`)

	if err := RunMigrations(); err != nil {
		t.Fatalf("legacy upgrade: %v", err)
	}

	// NULL 邮箱被清洗为占位地址且未标记验证；真实邮箱保持不变
	var email string
	var verified int
	mustQueryRow(t, db, "SELECT email, email_verified FROM user WHERE user_id = 101", &email, &verified)
	if email != "legacy_101@placeholder.invalid" || verified != 0 {
		t.Fatalf("legacy backfill wrong: email=%q verified=%d", email, verified)
	}
	mustQueryRow(t, db, "SELECT email, email_verified FROM user WHERE user_id = 102", &email, &verified)
	if email != "bob@real.example" {
		t.Fatalf("real email should be untouched, got %q", email)
	}

	// token_version 列就绪
	if !columnExists(t, db, "user", "token_version") {
		t.Fatal("token_version missing after upgrade")
	}
}

// TestMigrationsComposeBaselineDB Compose 初始化库（基线 + 种子）升级路径
func TestMigrationsComposeBaselineDB(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "compose")
	switchToTempDB(t, fullName)

	// 模拟 Compose：只执行 create_table.sql（基线）——读取嵌入文件保证与 000 一致
	files, err := listMigrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	var baselineSQL string
	for _, f := range files {
		if f.Version == 0 {
			baselineSQL = f.SQL
		}
	}
	for _, stmt := range splitSQLStatements(baselineSQL) {
		mustExec(t, stmt)
	}
	// 手工插入一条旧版用户（模拟已有数据）
	mustExec(t, "INSERT INTO user (user_id, username, password) VALUES (201, 'carol', 'x')")

	if err := RunMigrations(); err != nil {
		t.Fatalf("compose baseline upgrade: %v", err)
	}
	if !columnExists(t, db, "user", "token_version") {
		t.Fatal("upgrade incomplete")
	}
	// 已有用户被清洗且数据未丢
	var n int
	mustQueryRow(t, db, "SELECT COUNT(*) FROM user WHERE username = 'carol'", &n)
	if n != 1 {
		t.Fatal("existing user lost during upgrade")
	}
}

// TestMigrationsCommentEnhanceUpgrade 旧库（已有评论）升级到 008：reply_count 按存量回复关系回填，点赞从 0 起算
func TestMigrationsCommentEnhanceUpgrade(t *testing.T) {
	root := requireMySQL(t)
	switchToTempDB(t, createTempDB(t, root, "comment_upgrade"))

	// 先升级到 007（评论增强之前的状态），写入存量评论：一级评论 1 下挂回复 2、3，另有已删除回复 4 与已禁用回复 5
	if err := ensureMigrationTable(); err != nil {
		t.Fatal(err)
	}
	files, err := listMigrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Version > 7 {
			break
		}
		if err := applyMigration(f); err != nil {
			t.Fatalf("apply %03d: %v", f.Version, err)
		}
	}
	mustExec(t, "INSERT INTO comment (comment_id, user_id, target_type, target_id, parent_id, content) VALUES "+
		"(1, 1, 1, 10, 0, 'top'), (2, 2, 1, 10, 1, 'r1'), (3, 3, 1, 10, 1, 'r2'), (6, 4, 1, 10, 2, 'nested')")
	mustExec(t, "INSERT INTO comment (comment_id, user_id, target_type, target_id, parent_id, content, deleted_at) VALUES "+
		"(4, 2, 1, 10, 1, 'deleted', NOW())")
	mustExec(t, "INSERT INTO comment (comment_id, user_id, target_type, target_id, parent_id, content, status) VALUES "+
		"(5, 2, 1, 10, 1, 'disabled', 0)")

	if err := RunMigrations(); err != nil {
		t.Fatalf("upgrade to 008: %v", err)
	}
	if n := queryCount(t, "SELECT reply_count FROM comment WHERE comment_id = 1"); n != 3 {
		t.Fatalf("reply_count backfill = %d, want 3 (two replies + one flattened, only visible)", n)
	}
	// 深层回复上提到一级评论下，并记录被回复人
	var parentID, replyTo int64
	mustQueryRow(t, db, "SELECT parent_id, reply_to_user_id FROM comment WHERE comment_id = 6", &parentID, &replyTo)
	if parentID != 1 || replyTo != 2 {
		t.Fatalf("nested reply should be flattened under 1 replying to user 2, got parent=%d reply_to=%d", parentID, replyTo)
	}
	if n := queryCount(t, "SELECT like_count FROM comment WHERE comment_id = 1"); n != 0 {
		t.Fatalf("like_count should start at 0, got %d", n)
	}
	if n := queryCount(t, "SELECT COUNT(*) FROM comment WHERE reply_count <> 0"); n != 1 {
		t.Fatalf("only the top-level comment should have reply_count, got %d rows", n)
	}
	if !columnExists(t, db, "comment", "like_count") {
		t.Fatal("comment.like_count missing after upgrade")
	}
	// 评论可见性列表在升级后的结构上可正常执行
	rows, err := ListComments(&CommentPageParams{TargetType: 1, TargetID: 10, Limit: 20})
	if err != nil {
		t.Fatalf("list comments after upgrade: %v", err)
	}
	// 作者表为空（未建用户）：无活跃作者的评论不返回，但查询本身不应报错
	if len(rows) != 0 {
		t.Fatalf("comments without active author must be hidden, got %d", len(rows))
	}
}

func mustExec(t *testing.T, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %s: %v", firstLine(query), err)
	}
}

func mustQueryRow(t *testing.T, d *sqlx.DB, query string, dest ...interface{}) {
	t.Helper()
	if err := d.QueryRow(query).Scan(dest...); err != nil {
		t.Fatalf("query %q failed: %v", firstLine(query), err)
	}
}
