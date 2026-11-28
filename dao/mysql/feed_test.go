package mysql

import (
	"fmt"
	"testing"
	"time"
)

// TestFollowingFeedPagination 关注流 keyset 分页正确性（需要真实 MySQL，BB_TEST_MYSQL_DSN 启用）
// 验证点：第二页不重复、不漏数据（回归修复：游标参数曾与作者 ID 参数错位）
func TestFollowingFeedPagination(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "following")
	switchToTempDB(t, fullName)
	// 独立库，建 question/answer 最小结构（与迁移链列一致的可空性由 DEFAULT 兜底）
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
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY uk_question_id (question_id)) ENGINE=InnoDB`)
	mustExec(t, `CREATE TABLE answer (
		id bigint NOT NULL AUTO_INCREMENT, answer_id bigint NOT NULL,
		question_id bigint NOT NULL, author_id bigint NOT NULL, content mediumtext NOT NULL,
		vote_count int NOT NULL DEFAULT 0, comment_count int NOT NULL DEFAULT 0,
		collect_count int NOT NULL DEFAULT 0, status tinyint NOT NULL DEFAULT 1,
		moderation_status tinyint NOT NULL DEFAULT 1, visibility tinyint NOT NULL DEFAULT 1,
		deleted_at timestamp NULL DEFAULT NULL,
		create_time timestamp NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id), UNIQUE KEY uk_answer_id (answer_id)) ENGINE=InnoDB`)
	t.Cleanup(func() {
		mustExec(t, "DROP TABLE IF EXISTS answer")
		mustExec(t, "DROP TABLE IF EXISTS question")
	})

	// 造数据：2 个关注作者，各发 5 条内容（i%3==0 为问题，其余为回答挂在 q9000 下）
	// 时间交错：每两条同秒（验证 tie-breaker）
	authorA, authorB, outsider := int64(1), int64(2), int64(999)
	base := time.Now().Add(-time.Hour)
	mustExec(t, "INSERT INTO user (user_id, username, email) VALUES (?, 'a', 'a@t'), (?, 'b', 'b@t'), (?, 'o', 'o@t')", authorA, authorB, outsider)
	mustExec(t, "INSERT INTO question (question_id, title, author_id, description, create_time) VALUES (9000, 'q', ?, '', ?)", authorA, base)
	for i := 0; i < 10; i++ {
		author := authorA
		if i%2 == 1 {
			author = authorB
		}
		created := base.Add(time.Duration(i/2) * time.Minute)
		id := int64(1000 + i)
		if i%3 == 0 {
			mustExec(t, "INSERT INTO question (question_id, title, author_id, description, create_time) VALUES (?, ?, ?, '', ?)",
				id, fmt.Sprintf("t%d", i), author, created)
		} else {
			mustExec(t, "INSERT INTO answer (answer_id, question_id, author_id, content, create_time) VALUES (?, 9000, ?, ?, ?)",
				id, author, fmt.Sprintf("c%d", i), created)
		}
	}
	// 外人内容，不应出现在关注流
	mustExec(t, "INSERT INTO question (question_id, title, author_id, description, create_time) VALUES (7777, 'outsider', ?, '', ?)", outsider, base)

	// 翻页遍历：page size 4，直到取完，比对全集
	seen := make(map[int64]bool)
	cursorTime, cursorType, cursorID, total := time.Time{}, "", int64(0), 0
	for page := 0; page < 10; page++ {
		rows, err := FollowingFeedPage([]int64{authorA, authorB}, cursorTime, cursorType, cursorID, 4, 0)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if r.AuthorID == outsider {
				t.Fatal("outsider content leaked into following feed")
			}
			if seen[r.ItemID] {
				t.Fatalf("duplicate item %d across pages", r.ItemID)
			}
			seen[r.ItemID] = true
			total++
			cursorTime, cursorType, cursorID = r.CreateTime, r.ItemType, r.ItemID
		}
		if len(rows) < 4 {
			break
		}
	}
	// 关注作者内容：10 条 + 挂回答的问题 9000（作者 A）= 11 条
	if total != 11 {
		t.Fatalf("following feed items mismatch: got %d, want 11", total)
	}

	// 排序检查：create_time 降序，同秒内 item_id 严格降序（total order，不漏不重）
	rows, err := FollowingFeedPage([]int64{authorA, authorB}, time.Time{}, "", 0, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(rows); i++ {
		prev, cur := rows[i-1], rows[i]
		if cur.CreateTime.After(prev.CreateTime) {
			t.Fatal("rows not in descending time order")
		}
		if !cur.CreateTime.Equal(prev.CreateTime) {
			continue
		}
		// 同时间：item_type 降序（question > answer），同类型内 item_id 降序
		if cur.ItemType > prev.ItemType {
			t.Fatalf("item_type tie-breaker broken: %s after %s", cur.ItemType, prev.ItemType)
		}
		if cur.ItemType == prev.ItemType && cur.ItemID >= prev.ItemID {
			t.Fatalf("id tie-breaker broken: ids %d then %d at same time and type", prev.ItemID, cur.ItemID)
		}
	}
}

// TestFeedQueriesExplain EXPLAIN 验证时间索引用于 keyset 查询（记录样本：单表探针）
func TestFeedQueriesExplain(t *testing.T) {
	root := requireMySQL(t)
	fullName := createTempDB(t, root, "explain")
	switchToTempDB(t, fullName)
	mustExec(t, `CREATE TABLE IF NOT EXISTS explain_probe (
		id bigint NOT NULL AUTO_INCREMENT PRIMARY KEY,
		item_id bigint NOT NULL, author_id bigint NOT NULL, create_time timestamp NULL,
		KEY idx_time (create_time)) ENGINE=InnoDB`)
	t.Cleanup(func() { mustExec(t, "DROP TABLE IF EXISTS explain_probe") })

	rows, err := db.Query("EXPLAIN SELECT * FROM explain_probe WHERE create_time > NOW() - INTERVAL 1 DAY ORDER BY create_time DESC LIMIT 5")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	vals := make([]interface{}, len(cols))
	found := false
	for rows.Next() {
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		for i, c := range cols {
			if c == "key" && vals[i] != nil {
				if key, ok := vals[i].([]byte); ok && string(key) == "idx_time" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("EXPLAIN did not use idx_time index for time-range keyset query")
	}
}
