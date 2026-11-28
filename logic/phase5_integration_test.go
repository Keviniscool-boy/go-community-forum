package logic

// 第五阶段 M1 集成测试（PRD-第五阶段 §3 验收标准）
// F1 关注/粉丝列表、F2 拉黑全链路、F3 单条已读、F4 评论编辑
// 迁移链临时库 + 自包含种子；BB_TEST_MYSQL_DSN / BB_TEST_REDIS_ADDR 启用。

import (
	"context"
	"errors"
	"time"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/snowflake"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/viper"
)

var phase5DSN = os.Getenv("BB_TEST_MYSQL_DSN")

var (
	redisOnce    sync.Once
	redisInitErr error
)

// phase5Env 迁移链临时库 + 全局 DB 指向 + 种子用户（a/b/c）
func phase5Env(t *testing.T) (aID, bID, cID int64) {
	t.Helper()
	if phase5DSN == "" {
		t.Skip("BB_TEST_MYSQL_DSN not set")
	}
	if err := snowflake.Init("2026-07-24 00:00:00", 1); err != nil {
		t.Fatalf("init snowflake: %v", err)
	}
	// Redis（DM 限流计数等）：进程内一次性初始化，指向测试实例
	redisOnce.Do(func() {
		viper.Set("redis.host", "127.0.0.1")
		viper.Set("redis.port", 26379)
		viper.Set("redis.db", 0)
		redisInitErr = redis.Init()
		if redisInitErr == nil {
			// 清空共享 Redis 实例（限流计数跨运行残留会导致假阳性）
			if ferr := redis.GetClient().FlushDB(context.Background()).Err(); ferr != nil {
				redisInitErr = ferr
			}
		}
	})
	if redisInitErr != nil {
		t.Fatalf("init redis: %v", redisInitErr)
	}
	root, err := sqlx.Connect("mysql", phase5DSN)
	if err != nil {
		t.Fatalf("connect root: %v", err)
	}
	t.Cleanup(func() { _ = root.Close() })
	fullName := fmt.Sprintf("bb_phase5_test_%s", t.Name())
	_, err = root.Exec("DROP DATABASE IF EXISTS " + fullName)
	if err != nil {
		t.Fatal(err)
	}
	_, err = root.Exec("CREATE DATABASE " + fullName + " DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = root.Exec("DROP DATABASE IF EXISTS " + fullName)
	})
	temp, err := sqlx.Connect("mysql", phase5DSN+fullName+"?charset=utf8mb4&parseTime=True&loc=UTC")
	if err != nil {
		t.Fatalf("connect temp: %v", err)
	}
	t.Cleanup(func() { _ = temp.Close() })
	mysql.SetGlobalDB(temp)
	if err = mysql.RunMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	aID, bID, cID = 9401, 9402, 9403
	mustExecP5(t, temp,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'p5_a', 'a5@t', 'user', 'x'), (?, 'p5_b', 'b5@t', 'user', 'x'), (?, 'p5_c', 'c5@t', 'user', 'x')",
		aID, bID, cID)
	return
}

func mustExecP5(t *testing.T, d *sqlx.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := d.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func phase5Question(t *testing.T, d *sqlx.DB, authorID int64, title string) int64 {
	t.Helper()
	id := snowflake.GenerateID()
	mustExecP5(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, ?, '', ?, 1)",
		id, title, authorID)
	return id
}

func phase5Answer(t *testing.T, d *sqlx.DB, authorID, questionID int64, content string) int64 {
	t.Helper()
	id := snowflake.GenerateID()
	mustExecP5(t, d,
		"INSERT INTO answer (answer_id, question_id, author_id, content, moderation_status) VALUES (?, ?, ?, ?, 1)",
		id, questionID, authorID, content)
	return id
}

func phase5Comment(t *testing.T, d *sqlx.DB, userID, targetType, targetID, parentID int64) int64 {
	t.Helper()
	id := snowflake.GenerateID()
	mustExecP5(t, d,
		"INSERT INTO comment (comment_id, user_id, target_type, target_id, parent_id, content, status, moderation_status) VALUES (?, ?, ?, ?, ?, ?, 1, 1)",
		id, userID, targetType, targetID, parentID, "评论内容")
	return id
}

// TestFollowLists 关注/粉丝列表（PRD F1）：计数一致、viewer 视角、封禁用户排除
func TestFollowLists(t *testing.T) {
	aID, bID, cID := phase5Env(t)
	d := mysql.GetGlobalDB()
	// b、c 关注 a；a 关注 b
	if err := mysql.FollowUser(bID, aID); err != nil {
		t.Fatal(err)
	}
	if err := mysql.FollowUser(cID, aID); err != nil {
		t.Fatal(err)
	}
	if err := mysql.FollowUser(aID, bID); err != nil {
		t.Fatal(err)
	}

	followers, err := ListFollowers(aID, aID, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(followers.Items) != 2 || !followers.HasMore == false {
		t.Fatalf("followers = %d hasMore=%v (want 2, false)", len(followers.Items), followers.HasMore)
	}
	for _, it := range followers.Items {
		if it.UserID == strconv.FormatInt(bID, 10) && !it.IsFollowing {
			t.Fatal("a should be following b")
		}
		if it.UserID == strconv.FormatInt(cID, 10) && it.IsFollowing {
			t.Fatal("a should NOT be following c")
		}
	}

	following, err := ListFollowing(bID, aID, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(following.Items) != 1 || following.Items[0].UserID != strconv.FormatInt(aID, 10) {
		t.Fatalf("following list wrong: %+v", following.Items)
	}

	// 游客视角 is_following 全 false
	guest, err := ListFollowers(aID, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range guest.Items {
		if it.IsFollowing {
			t.Fatal("guest is_following must be false")
		}
	}

	// 封禁用户排除
	mustExecP5(t, d, "UPDATE user SET status = 0 WHERE user_id = ?", cID)
	banned, err := ListFollowers(aID, aID, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(banned.Items) != 1 {
		t.Fatalf("followers after ban = %d (want 1)", len(banned.Items))
	}
}

// TestBlockFullChain 拉黑全链路（PRD F2 验收）
func TestBlockFullChain(t *testing.T) {
	aID, bID, _ := phase5Env(t)
	d := mysql.GetGlobalDB()
	if err := mysql.FollowUser(aID, bID); err != nil {
		t.Fatal(err)
	}
	if err := mysql.FollowUser(bID, aID); err != nil {
		t.Fatal(err)
	}
	questionID := phase5Question(t, d, aID, "拉黑链路问题")
	mustExecP5(t, d, "INSERT INTO question_follow (user_id, question_id) VALUES (?, ?)", bID, questionID)
	_ = phase5Answer(t, d, bID, questionID, "被拉黑者的回答")
	phase5Comment(t, d, bID, 2, questionID, 0)

	// a 拉黑 b：自动解除双向关注；问题关注不受影响
	if err := mysql.BlockUser(aID, bID); err != nil {
		t.Fatal(err)
	}
	var followCount int
	if err := d.Get(&followCount,
		"SELECT COUNT(*) FROM user_follow WHERE (follower_id=? AND following_id=?) OR (follower_id=? AND following_id=?)",
		aID, bID, bID, aID); err != nil {
		t.Fatal(err)
	}
	if followCount != 0 {
		t.Fatalf("follow rows after block = %d (want 0)", followCount)
	}
	var qFollow int
	if err := d.Get(&qFollow, "SELECT COUNT(*) FROM question_follow WHERE user_id = ? AND question_id = ?", bID, questionID); err != nil {
		t.Fatal(err)
	}
	if qFollow != 1 {
		t.Fatal("question follow must survive block")
	}

	// b 再关注 a → 统一文案拒绝
	if err := FollowUser(bID, aID); !errors.Is(err, ErrFollowBlocked) {
		t.Fatalf("follow after block: want ErrFollowBlocked, got %v", err)
	}

	// a 的最新 Feed 不再出现 b 的内容
	feed, err := GetLatestFeed(aID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range feed.Items {
		if it.Author.UserID == strconv.FormatInt(bID, 10) {
			t.Fatal("blocked author leaked into latest feed")
		}
	}

	// a 视角：b 的评论不出现
	rows, err := mysql.ListComments(&mysql.CommentPageParams{
		TargetType: models.TargetQuestion, TargetID: questionID, Limit: 20, ViewerID: aID,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.UserID == bID {
			t.Fatal("blocked user comment leaked into list")
		}
	}
	// 游客视角仍可见（屏蔽是 viewer 维度）
	guestRows, err := mysql.ListComments(&mysql.CommentPageParams{
		TargetType: models.TargetQuestion, TargetID: questionID, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(guestRows) == 0 {
		t.Fatal("guest should still see blocked user's comments")
	}

	// 拉黑期间 b 的互动不产生新通知（历史通知保留是正确行为——只看块后增量）
	ansID := phase5Answer(t, d, aID, questionID, "a 的回答")
	var base int
	if err := d.Get(&base, "SELECT COUNT(*) FROM notification WHERE user_id = ? AND actor_id = ?", aID, bID); err != nil {
		t.Fatal(err)
	}
	if err := mysql.VoteAnswer(bID, ansID, 1); err != nil {
		t.Fatal(err)
	}
	var n1 int
	if err := d.Get(&n1, "SELECT COUNT(*) FROM notification WHERE user_id = ? AND actor_id = ?", aID, bID); err != nil {
		t.Fatal(err)
	}
	if n1 != base {
		t.Fatalf("blocked actor produced %d new notifications (want 0)", n1-base)
	}
	// 对照：c 点赞 → 有通知
	if err := mysql.VoteAnswer(9403, ansID, 1); err != nil {
		t.Fatal(err)
	}
	var n2 int
	if err := d.Get(&n2, "SELECT COUNT(*) FROM notification WHERE user_id = ? AND actor_id = ?", aID, int64(9403)); err != nil {
		t.Fatal(err)
	}
	if n2 != 1 {
		t.Fatalf("normal actor notification = %d (want 1)", n2)
	}

	// a 看不到 b 的主页
	if _, err := GetUserProfile(bID, aID); !errors.Is(err, mysql.ErrUserNotExist) {
		t.Fatalf("blocked profile: want ErrUserNotExist, got %v", err)
	}

	// 取消拉黑 → 可再关注
	if err := mysql.UnblockUser(aID, bID); err != nil {
		t.Fatal(err)
	}
	if err := FollowUser(bID, aID); err != nil {
		t.Fatalf("follow after unblock: %v", err)
	}
}

// TestNotificationSingleRead 单条已读（PRD F3）
func TestNotificationSingleRead(t *testing.T) {
	aID, bID, _ := phase5Env(t)
	d := mysql.GetGlobalDB()
	questionID := phase5Question(t, d, aID, "单条已读问题")
	ansID := phase5Answer(t, d, aID, questionID, "回答")
	if err := mysql.VoteAnswer(bID, ansID, 1); err != nil {
		t.Fatal(err)
	}
	var nid int64
	if err := d.Get(&nid, "SELECT notification_id FROM notification WHERE user_id = ? LIMIT 1", aID); err != nil {
		t.Fatalf("no notification created: %v", err)
	}
	cnt, _ := mysql.CountUnreadNotifications(aID)
	if cnt != 1 {
		t.Fatalf("unread before = %d", cnt)
	}
	if err := MarkNotificationRead(aID, strconv.FormatInt(nid, 10)); err != nil {
		t.Fatal(err)
	}
	if err := MarkNotificationRead(aID, strconv.FormatInt(nid, 10)); err != nil {
		t.Fatal(err) // 幂等
	}
	cnt, _ = mysql.CountUnreadNotifications(aID)
	if cnt != 0 {
		t.Fatalf("unread after = %d (want 0)", cnt)
	}
	if err := MarkNotificationRead(bID, strconv.FormatInt(nid, 10)); !errors.Is(err, ErrNotificationForbidden) {
		t.Fatalf("cross-user read: want ErrNotificationForbidden, got %v", err)
	}
}

// TestCommentEdit 评论编辑（PRD F4）
func TestCommentEdit(t *testing.T) {
	aID, bID, _ := phase5Env(t)
	d := mysql.GetGlobalDB()
	questionID := phase5Question(t, d, aID, "评论编辑问题")
	cmtID := phase5Comment(t, d, bID, 2, questionID, 0)

	// 非作者
	if _, err := EditComment(aID, strconv.FormatInt(cmtID, 10), "非作者编辑"); !errors.Is(err, ErrCommentNotOwner) {
		t.Fatalf("non-author edit: want ErrCommentNotOwner, got %v", err)
	}
	// 作者窗口内编辑（等 1.1s 保证毫秒级 update_time > create_time；同毫秒编辑视为未编辑）
	time.Sleep(1100 * time.Millisecond)
	detail, err := EditComment(bID, strconv.FormatInt(cmtID, 10), "编辑后的内容")
	if err != nil {
		t.Fatalf("author edit: %v", err)
	}
	if detail.Content != "编辑后的内容" {
		t.Fatalf("edited content: %q", detail.Content)
	}
	var edited bool
	if err := d.Get(&edited,
		"SELECT update_time > create_time FROM comment WHERE comment_id = ?", cmtID); err != nil {
		t.Fatal(err)
	}
	if !edited {
		t.Fatal("comment should be marked edited")
	}
	// 超时
	mustExecP5(t, d, "UPDATE comment SET create_time = NOW() - INTERVAL 10 MINUTE WHERE comment_id = ?", cmtID)
	if _, err := EditComment(bID, strconv.FormatInt(cmtID, 10), "超时编辑"); !errors.Is(err, ErrCommentEditWindow) {
		t.Fatalf("timeout edit: want ErrCommentEditWindow, got %v", err)
	}
	// 空内容
	if _, err := EditComment(bID, strconv.FormatInt(cmtID, 10), ""); err == nil {
		t.Fatal("empty content should fail")
	}

	// 普通词隔离：moderation_status 变为隔离且列表不可见
	if err := mysql.AddSensitiveWord("隔离测试词", 2, aID); err != nil {
		t.Fatal(err)
	}
	InvalidateSensitiveWords() // 进程级词表缓存失效（管理端路径会调用；测试直插需手动）
	mustExecP5(t, d, "UPDATE comment SET create_time = NOW() WHERE comment_id = ?", cmtID)
	if _, err := EditComment(bID, strconv.FormatInt(cmtID, 10), "包含隔离测试词的内容"); err != nil {
		t.Fatalf("quarantine edit: %v", err)
	}
	var modStatus int32
	if err := d.Get(&modStatus, "SELECT moderation_status FROM comment WHERE comment_id = ?", cmtID); err != nil {
		t.Fatal(err)
	}
	if modStatus != models.ModerationQuarantined {
		t.Fatalf("moderation after edit = %d (want quarantined)", modStatus)
	}
	rows, err := mysql.ListComments(&mysql.CommentPageParams{
		TargetType: models.TargetQuestion, TargetID: questionID, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.CommentID == cmtID {
			t.Fatal("quarantined comment visible in list")
		}
	}
}
