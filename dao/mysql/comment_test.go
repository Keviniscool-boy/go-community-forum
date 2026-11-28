package mysql

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"

	"bluebell/models"
)

// 评论体系集成测试：用真实迁移链建库（保证与生产结构一致，而非手写最小表）。
// 覆盖：两级回复与计数、回复/点赞通知、点赞并发幂等、软删除后的计数回算与占位保留、游标分页。

// setupCommentSchema 建库并写入三个用户（1 = 内容作者）与一问一答（均为用户 1 所发）
func setupCommentSchema(t *testing.T) {
	t.Helper()
	root := requireMySQL(t)
	switchToTempDB(t, createTempDB(t, root, "comment"))
	if err := RunMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	for _, uid := range []int64{1, 2, 3} {
		mustExec(t, "INSERT INTO user (user_id, username, email, password) VALUES (?, ?, ?, 'x')",
			uid, fmt.Sprintf("user%d", uid), fmt.Sprintf("user%d@t.local", uid))
	}
	mustExec(t, "INSERT INTO question (question_id, title, description, author_id) VALUES (10, 'q', 'd', 1)")
	mustExec(t, "INSERT INTO answer (answer_id, question_id, author_id, content) VALUES (20, 10, 1, 'ans')")
}

// mustCreateComment 在回答 20 下写入一条评论（parentID=0 为一级评论）
func mustCreateComment(t *testing.T, id, userID, parentID int64, content string) *models.Comment {
	t.Helper()
	cm := &models.Comment{
		CommentID:  id,
		UserID:     userID,
		TargetType: models.TargetAnswer,
		TargetID:   20,
		ParentID:   parentID,
		Content:    content,
	}
	if err := CreateComment(cm); err != nil {
		t.Fatalf("create comment %d: %v", id, err)
	}
	return cm
}

// queryCount 查询单个计数值
func queryCount(t *testing.T, query string, args ...interface{}) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q failed: %v", firstLine(query), err)
	}
	return n
}

// TestCommentThreadLifecycle 两级回复、通知、点赞、删除与计数回算
func TestCommentThreadLifecycle(t *testing.T) {
	setupCommentSchema(t)

	// 一级评论：目标评论数 +1，内容作者收到「评论」通知
	mustCreateComment(t, 101, 2, 0, "一级评论")
	if n := queryCount(t, "SELECT comment_count FROM answer WHERE answer_id = 20"); n != 1 {
		t.Fatalf("answer comment_count = %d, want 1", n)
	}
	if n := queryCount(t, "SELECT COUNT(*) FROM notification WHERE user_id = 1 AND actor_id = 2 AND type = 2"); n != 1 {
		t.Fatalf("comment notification to content author = %d, want 1", n)
	}

	// 回复：父评论回复数 +1；被回复人收到「回复」通知，内容作者（不同于被回复人）收到「评论」通知
	mustCreateComment(t, 102, 3, 101, "回复")
	if n := queryCount(t, "SELECT reply_count FROM comment WHERE comment_id = 101"); n != 1 {
		t.Fatalf("parent reply_count = %d, want 1", n)
	}
	if n := queryCount(t, "SELECT COUNT(*) FROM notification WHERE user_id = 2 AND actor_id = 3 AND type = 13"); n != 1 {
		t.Fatalf("reply notification to parent author = %d, want 1", n)
	}
	if n := queryCount(t, "SELECT COUNT(*) FROM notification WHERE user_id = 1 AND actor_id = 3 AND type = 2"); n != 1 {
		t.Fatalf("comment notification to content author on reply = %d, want 1", n)
	}
	if n := queryCount(t, "SELECT comment_count FROM answer WHERE answer_id = 20"); n != 2 {
		t.Fatalf("answer comment_count = %d, want 2", n)
	}

	// 自己回复自己：不产生通知
	mustCreateComment(t, 103, 2, 101, "自回复")
	if n := queryCount(t, "SELECT COUNT(*) FROM notification WHERE user_id = 2"); n != 1 {
		t.Fatalf("self reply must not notify, user 2 notifications = %d, want 1", n)
	}

	// 回复另一条回复：挂到所属一级评论 101 下并记录被回复人；被回复人与一级评论作者各收到一条「回复」通知
	if err := CreateComment(&models.Comment{
		CommentID: 104, UserID: 1, TargetType: models.TargetAnswer, TargetID: 20,
		ParentID: 101, ReplyToUserID: 3, Content: "回复的回复",
	}); err != nil {
		t.Fatal(err)
	}
	if n := queryCount(t, "SELECT COUNT(*) FROM notification WHERE user_id = 3 AND actor_id = 1 AND type = 13"); n != 1 {
		t.Fatalf("reply-to-reply notification to replied user = %d, want 1", n)
	}
	if n := queryCount(t, "SELECT COUNT(*) FROM notification WHERE user_id = 2 AND actor_id = 1 AND type = 13"); n != 1 {
		t.Fatalf("reply-to-reply notification to root author = %d, want 1", n)
	}
	replies, err := ListCommentReplies(101, 0, sql.NullTime{}, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(replies) != 3 || replies[0].CommentID != 102 || replies[1].CommentID != 103 || replies[2].CommentID != 104 {
		t.Fatalf("replies order wrong: got %d rows", len(replies))
	}
	if replies[2].ReplyToUserID != 3 {
		t.Fatalf("reply-to-reply should carry reply_to_user_id=3, got %d", replies[2].ReplyToUserID)
	}
	if replies[0].ReplyToUserID != 0 {
		t.Fatalf("reply to top-level comment must not carry reply_to_user_id")
	}

	// 点赞：并发 20 次同一用户点赞，关系表与计数都只有 1；首次点赞只通知一次
	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := SetCommentLike(1, 101, true); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent like failed: %v", err)
	}
	if n := queryCount(t, "SELECT like_count FROM comment WHERE comment_id = 101"); n != 1 {
		t.Fatalf("like_count = %d, want 1", n)
	}
	if n := queryCount(t, "SELECT COUNT(*) FROM notification WHERE user_id = 2 AND actor_id = 1 AND type = 12"); n != 1 {
		t.Fatalf("comment like notification = %d, want 1", n)
	}
	if err := SetCommentLike(3, 101, true); err != nil {
		t.Fatal(err)
	}
	liked, err := UserLikedCommentIDs(3, []int64{101, 102})
	if err != nil {
		t.Fatal(err)
	}
	if len(liked) != 1 || !liked[101] {
		t.Fatalf("UserLikedCommentIDs = %v, want {101}", liked)
	}
	// 重复取消点赞：计数不为负
	for i := 0; i < 2; i++ {
		if err := SetCommentLike(3, 101, false); err != nil {
			t.Fatal(err)
		}
	}
	if n := queryCount(t, "SELECT like_count FROM comment WHERE comment_id = 101"); n != 1 {
		t.Fatalf("like_count after double unlike = %d, want 1", n)
	}
	if err := SetCommentLike(2, 999, true); err != ErrCommentNotExist {
		t.Fatalf("like missing comment: got %v, want ErrCommentNotExist", err)
	}

	// 删除：非作者且非管理员被拒；管理员可删；已删评论保留占位（下方仍有回复）
	if err := SoftDeleteComment(101, 3, false); !isCommentForbidden(err) {
		t.Fatalf("delete by other user: got %v, want ErrCommentForbidden", err)
	}
	if err := SoftDeleteComment(101, 3, true); err != nil {
		t.Fatalf("privileged delete: %v", err)
	}
	if n := queryCount(t, "SELECT comment_count FROM answer WHERE answer_id = 20"); n != 3 {
		t.Fatalf("answer comment_count after delete = %d, want 3", n)
	}
	top, err := ListComments(&CommentPageParams{TargetType: models.TargetAnswer, TargetID: 20, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].CommentID != 101 || !top[0].DeletedAt.Valid || top[0].ReplyCount != 3 {
		t.Fatalf("deleted comment with replies should be kept as placeholder, got %+v", top)
	}
	if replies, err = ListCommentReplies(101, 0, sql.NullTime{}, 0, 20); err != nil || len(replies) != 3 {
		t.Fatalf("replies under deleted comment must remain, got %d err=%v", len(replies), err)
	}

	// 删除回复：父评论回复数与目标评论数回算
	if err := SoftDeleteComment(102, 3, false); err != nil {
		t.Fatal(err)
	}
	if n := queryCount(t, "SELECT reply_count FROM comment WHERE comment_id = 101"); n != 2 {
		t.Fatalf("parent reply_count after deleting reply = %d, want 2", n)
	}
	if n := queryCount(t, "SELECT comment_count FROM answer WHERE answer_id = 20"); n != 2 {
		t.Fatalf("answer comment_count = %d, want 2", n)
	}
	// 已删除的评论不能被回复、不能被点赞、不能重复删除
	cm := &models.Comment{CommentID: 105, UserID: 3, TargetType: models.TargetAnswer, TargetID: 20, ParentID: 101, Content: "x"}
	if err := CreateComment(cm); err != ErrCommentNotExist {
		t.Fatalf("reply to deleted comment: got %v, want ErrCommentNotExist", err)
	}
	if err := SetCommentLike(1, 101, true); err != ErrCommentNotExist {
		t.Fatalf("like deleted comment: got %v, want ErrCommentNotExist", err)
	}
	if err := SoftDeleteComment(102, 3, false); err != ErrCommentNotExist {
		t.Fatalf("double delete: got %v, want ErrCommentNotExist", err)
	}

	// 回复全部删除后，占位不再返回
	if err := SoftDeleteComment(104, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := SoftDeleteComment(103, 2, false); err != nil {
		t.Fatal(err)
	}
	if n := queryCount(t, "SELECT reply_count FROM comment WHERE comment_id = 101"); n != 0 {
		t.Fatalf("parent reply_count = %d, want 0", n)
	}
	if n := queryCount(t, "SELECT comment_count FROM answer WHERE answer_id = 20"); n != 0 {
		t.Fatalf("answer comment_count = %d, want 0", n)
	}
	top, err = ListComments(&CommentPageParams{TargetType: models.TargetAnswer, TargetID: 20, Limit: 20})
	if err != nil || len(top) != 0 {
		t.Fatalf("placeholder without visible replies must be hidden, got %d err=%v", len(top), err)
	}

	// 被封禁作者的评论不对外展示，解封后恢复
	mustCreateComment(t, 106, 3, 0, "封禁用户的评论")
	mustExec(t, "UPDATE user SET status = 0 WHERE user_id = 3")
	top, err = ListComments(&CommentPageParams{TargetType: models.TargetAnswer, TargetID: 20, Limit: 20})
	if err != nil || len(top) != 0 {
		t.Fatalf("banned author's comment must be hidden, got %d err=%v", len(top), err)
	}
	mustExec(t, "UPDATE user SET status = 1 WHERE user_id = 3")
	top, err = ListComments(&CommentPageParams{TargetType: models.TargetAnswer, TargetID: 20, Limit: 20})
	if err != nil || len(top) != 1 || top[0].CommentID != 106 {
		t.Fatalf("comment should be visible after unban, got %+v err=%v", top, err)
	}
}

// isCommentForbidden 判定是否为无权操作错误
func isCommentForbidden(err error) bool { return err == ErrCommentForbidden }

// TestCommentCursorPagination 一级评论（最新在前）与回复（最早在前）的游标分页互不重叠、不丢项
func TestCommentCursorPagination(t *testing.T) {
	setupCommentSchema(t)

	stamps := []string{"2026-01-01 00:00:01", "2026-01-01 00:00:02", "2026-01-01 00:00:03", "2026-01-01 00:00:04", "2026-01-01 00:00:05"}
	for i, ts := range stamps {
		mustExec(t, "INSERT INTO comment (comment_id, user_id, target_type, target_id, parent_id, content, create_time) VALUES (?, 2, 1, 20, 0, 'c', ?)",
			int64(200+i), ts)
	}
	// 回复挂在 200 下，时间递增
	for i, ts := range stamps {
		mustExec(t, "INSERT INTO comment (comment_id, user_id, target_type, target_id, parent_id, content, create_time) VALUES (?, 3, 1, 20, 200, 'r', ?)",
			int64(300+i), ts)
	}

	seen := map[int64]bool{}
	cursorID := int64(0)
	var cursorTime sql.NullTime
	pages := 0
	for {
		rows, err := ListComments(&CommentPageParams{TargetType: models.TargetAnswer, TargetID: 20, CursorTime: cursorTime, CursorID: cursorID, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		hasMore := len(rows) > 2
		if hasMore {
			rows = rows[:2]
		}
		for i := 1; i < len(rows); i++ { // 新的在前
			if rows[i-1].CommentID <= rows[i].CommentID {
				t.Fatalf("top-level comments must be newest first, got %d then %d", rows[i-1].CommentID, rows[i].CommentID)
			}
		}
		for _, r := range rows {
			if seen[r.CommentID] {
				t.Fatalf("comment %d returned twice across pages", r.CommentID)
			}
			seen[r.CommentID] = true
		}
		pages++
		if !hasMore {
			break
		}
		cursorTime.Time, cursorTime.Valid = rows[len(rows)-1].CreateTime, true
		cursorID = rows[len(rows)-1].CommentID
	}
	if len(seen) != 5 || pages != 3 {
		t.Fatalf("top-level pagination: seen=%d pages=%d, want 5 / 3", len(seen), pages)
	}

	// 回复：最早在前
	seen = map[int64]bool{}
	cursorID = 0
	cursorTime = sql.NullTime{}
	for {
		rows, err := ListCommentReplies(200, 0, cursorTime, cursorID, 2)
		if err != nil {
			t.Fatal(err)
		}
		hasMore := len(rows) > 2
		if hasMore {
			rows = rows[:2]
		}
		for i := 1; i < len(rows); i++ {
			if rows[i-1].CommentID >= rows[i].CommentID {
				t.Fatalf("replies must be oldest first, got %d then %d", rows[i-1].CommentID, rows[i].CommentID)
			}
		}
		for _, r := range rows {
			seen[r.CommentID] = true
		}
		if !hasMore {
			break
		}
		cursorTime.Time, cursorTime.Valid = rows[len(rows)-1].CreateTime, true
		cursorID = rows[len(rows)-1].CommentID
	}
	if len(seen) != 5 {
		t.Fatalf("reply pagination: seen=%d, want 5", len(seen))
	}
}
