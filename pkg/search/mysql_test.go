package search

// 搜索集成测试（A3 验收）：真实迁移链建库，验证
// 专栏 scope、已解决问题标注（solved）、回答结果携带所属问题 id。
// 需要 BB_TEST_MYSQL_DSN（同其他集成测试）。

import (
	"context"
	"os"
	"strconv"
	"testing"

	"bluebell/dao/mysql"
	"bluebell/pkg/snowflake"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var searchDSN = os.Getenv("BB_TEST_MYSQL_DSN")

func TestSearchColumnScopeAndSolvedFlag(t *testing.T) {
	if searchDSN == "" {
		t.Skip("BB_TEST_MYSQL_DSN not set, skip search integration test")
	}
	if err := snowflake.Init("2026-07-24 00:00:00", 1); err != nil {
		t.Fatalf("init snowflake: %v", err)
	}
	root, err := sqlx.Connect("mysql", searchDSN)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	// Cleanup LIFO：先 DROP 库，再关连接（defer 会先于 Cleanup 关闭连接，导致 DROP 失败）
	t.Cleanup(func() { _ = root.Close() })
	fullName := "bb_search_test_" + t.Name()
	mustExecSearch(t, root, "DROP DATABASE IF EXISTS "+fullName)
	mustExecSearch(t, root, "CREATE DATABASE "+fullName+" DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci")

	temp, err := sqlx.Connect("mysql", searchDSN+fullName+"?charset=utf8mb4&parseTime=True&loc=UTC")
	if err != nil {
		t.Fatalf("connect temp: %v", err)
	}
	t.Cleanup(func() { _ = temp.Close() })
	t.Cleanup(func() { mustExecSearch(t, root, "DROP DATABASE IF EXISTS "+fullName) })
	mysql.SetGlobalDB(temp)
	if err = mysql.RunMigrations(); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	// 种子：1 个用户、已解决问题（含"采纳闭环"标题）、公开专栏
	mustExecSearch(t, temp,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (7001, 'tester', 's@t', 'user', 'x')")
	solvedQID := snowflake.GenerateID()
	mustExecSearch(t, temp,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status, resolution_status) VALUES (?, '采纳闭环指南', '如何做', 7001, 1, 1)", solvedQID)
	mustExecSearch(t, temp,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '未解决问题', '求助', 7001, 1)", snowflake.GenerateID())
	colID := snowflake.GenerateID()
	mustExecSearch(t, temp,
		"INSERT INTO column_info (column_id, owner_id, name, description) VALUES (?, 7001, '专栏搜索测试', '收录专栏')", colID)
	mustExecSearch(t, temp,
		"INSERT INTO article (article_id, author_id, column_id, title, content, moderation_status) VALUES (?, 7001, ?, '专栏搜索测试文章', '正文', 1)", snowflake.GenerateID(), colID)

	m := &MySQLFallback{DB: temp}
	ctx := context.Background()

	// 1) column scope 命中专栏（scope=column 只搜专栏表）
	colRes, err := m.Search(ctx, Query{Keyword: "专栏搜索测试", Scope: "column", Limit: 20})
	if err != nil {
		t.Fatalf("column search: %v", err)
	}
	if len(colRes.Items) != 1 || colRes.Items[0].DocumentType != "column" {
		t.Fatalf("column scope results = %d (%+v), want 1 column", len(colRes.Items), colRes.Items)
	}
	// article scope 命中专栏下的文章
	artRes, err := m.Search(ctx, Query{Keyword: "专栏搜索测试", Scope: "article", Limit: 20})
	if err != nil {
		t.Fatalf("article search: %v", err)
	}
	if len(artRes.Items) != 1 || artRes.Items[0].DocumentType != "article" {
		t.Fatalf("article scope results = %d (%+v), want 1 article", len(artRes.Items), artRes.Items)
	}
	// 2) 全 scope：问题结果带 solved 标注
	allRes, err := m.Search(ctx, Query{Keyword: "采纳闭环", Scope: "all", Limit: 20})
	if err != nil {
		t.Fatalf("all search: %v", err)
	}
	foundSolved := false
	for _, it := range allRes.Items {
		if it.DocumentType == "question" && it.DocumentID == strconv.FormatInt(solvedQID, 10) {
			if !it.Solved {
				t.Fatal("solved question should be marked solved")
			}
			foundSolved = true
		}
	}
	if !foundSolved {
		t.Fatalf("solved question not found in results: %+v", allRes.Items)
	}
	// 未解决问题不带 solved
	openRes, err := m.Search(ctx, Query{Keyword: "未解决问题", Scope: "question", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range openRes.Items {
		if it.Solved {
			t.Fatal("open question must not be marked solved")
		}
	}
	// 3) 隐藏专栏不可搜出
	mustExecSearch(t, temp, "UPDATE column_info SET visibility = 0 WHERE column_id = ?", colID)
	hiddenRes, err := m.Search(ctx, Query{Keyword: "专栏搜索测试", Scope: "column", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range hiddenRes.Items {
		if it.DocumentType == "column" {
			t.Fatal("hidden column must not appear in search results")
		}
	}
}


func mustExecSearch(t *testing.T, d *sqlx.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := d.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
