package logic

// 问答解决闭环集成测试（产品审计 PRD §4 / A2 验收）。
// 每个用例在独立临时库上执行真实迁移链 000-014，验证：
// 采纳/撤销原子性、并发采纳唯一、关闭/重开状态机、采纳答案删除/下架/作者禁用自动恢复 open、
// 采纳置顶排序分页不重复、GET /me/content 状态过滤。

import (
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"

	"github.com/jmoiron/sqlx"
	_ "github.com/go-sql-driver/mysql"
)

// resolutionTempDB 建临时库并执行完整迁移链，全局 DB 指向它
func resolutionTempDB(t *testing.T) *sqlx.DB {
	t.Helper()
	if resDSN == "" {
		t.Skip("BB_TEST_MYSQL_DSN not set, skip resolution integration test")
	}
	requireFlowEnv(t) // viper/Redis/雪花初始化
	root := rootDB(t)
	fullName := "bb_resolution_test_" + t.Name()
	mustExecFlow(t, root, "DROP DATABASE IF EXISTS "+fullName)
	mustExecFlow(t, root, "CREATE DATABASE "+fullName+" DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci")
	t.Cleanup(func() {
		mustExecFlow(t, root, "DROP DATABASE IF EXISTS "+fullName)
	})
	temp, err := sqlx.Connect("mysql", resDSN+fullName+"?charset=utf8mb4&parseTime=True&loc=UTC")
	if err != nil {
		t.Fatalf("connect temp db failed: %v", err)
	}
	t.Cleanup(func() { temp.Close() })
	mysql.SetGlobalDB(temp)
	if err = mysql.RunMigrations(); err != nil {
		t.Fatalf("run migrations failed: %v", err)
	}
	return temp
}

var resDSN = os.Getenv("BB_TEST_MYSQL_DSN")

// seedQA 建用户/问题/回答，返回各 id
func seedQA(t *testing.T, d *sqlx.DB, nAnswers int) (authorID, questionID int64, answerIDs []int64) {
	t.Helper()
	authorID = 9001
	responderID := int64(9002)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'author', 'a@t', 'user', 'x'), (?, 'answerer', 'b@t', 'user', 'x')",
		authorID, responderID)
	questionID = snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '如何测试采纳闭环', 'desc', ?, 1)",
		questionID, authorID)
	answerIDs = make([]int64, 0, nAnswers)
	for i := 0; i < nAnswers; i++ {
		aid := snowflake.GenerateID()
		mustExecFlow(t, d,
			"INSERT INTO answer (answer_id, question_id, author_id, content, moderation_status) VALUES (?, ?, ?, ?, 1)",
			aid, questionID, responderID, "回答内容 "+strconv.Itoa(i))
		mustExecFlow(t, d, "UPDATE question SET answer_count = answer_count + 1 WHERE question_id = ?", questionID)
		answerIDs = append(answerIDs, aid)
	}
	return
}

// requireQuestionResolution 读回问题的解决状态字段
func requireQuestionResolution(t *testing.T, d *sqlx.DB, questionID int64) (resolution int32, acceptedID int64) {
	t.Helper()
	row := d.QueryRow("SELECT resolution_status, COALESCE(accepted_answer_id, 0) FROM question WHERE question_id = ?", questionID)
	if err := row.Scan(&resolution, &acceptedID); err != nil {
		t.Fatalf("read question resolution: %v", err)
	}
	return
}

// TestAcceptAnswerFlow 采纳主流程：作者采纳 → solved + 通知 + 审计；幂等；换答案冲突；越权拒绝
func TestAcceptAnswerFlow(t *testing.T) {
	d := resolutionTempDB(t)
	authorID, questionID, answerIDs := seedQA(t, d, 2)

	// 非作者采纳 → ErrForbidden
	if err := AcceptAnswer(9999, questionID, answerIDs[0], "req1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-author accept: want ErrForbidden, got %v", err)
	}
	// 作者采纳回答 0
	if err := AcceptAnswer(authorID, questionID, answerIDs[0], "req1"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	resolution, accepted := requireQuestionResolution(t, d, questionID)
	if resolution != models.QuestionResolutionSolved || accepted != answerIDs[0] {
		t.Fatalf("after accept: resolution=%d accepted=%d", resolution, accepted)
	}
	// 通知回答作者（type=6 采纳）
	var notifyCount int
	if err := d.Get(&notifyCount,
		"SELECT COUNT(*) FROM notification WHERE user_id = 9002 AND type = ? AND target_id = ?",
		models.NotifyTypeAccept, questionID); err != nil {
		t.Fatal(err)
	}
	if notifyCount != 1 {
		t.Fatalf("want 1 accept notification, got %d", notifyCount)
	}
	// 审计记录
	var auditCount int
	if err := d.Get(&auditCount, "SELECT COUNT(*) FROM audit_log WHERE action = 'answer_accept' AND target_id = ?",
		strconv.FormatInt(answerIDs[0], 10)); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("want 1 audit log, got %d", auditCount)
	}
	// 幂等：重复采纳同一回答成功且不重复通知/审计
	if err := AcceptAnswer(authorID, questionID, answerIDs[0], "req2"); err != nil {
		t.Fatalf("idempotent accept: %v", err)
	}
	if err := d.Get(&notifyCount, "SELECT COUNT(*) FROM notification WHERE user_id = 9002 AND type = ?", models.NotifyTypeAccept); err != nil {
		t.Fatal(err)
	}
	if notifyCount != 1 {
		t.Fatalf("idempotent accept duplicated notification: %d", notifyCount)
	}
	// 换答案必须先撤销 → 冲突
	if err := AcceptAnswer(authorID, questionID, answerIDs[1], "req3"); !errors.Is(err, ErrQuestionStateConflict) {
		t.Fatalf("accept different answer: want conflict, got %v", err)
	}
}

// TestUnacceptAnswerFlow 撤销采纳：恢复 open + 通知；幂等
func TestUnacceptAnswerFlow(t *testing.T) {
	d := resolutionTempDB(t)
	authorID, questionID, answerIDs := seedQA(t, d, 1)
	if err := AcceptAnswer(authorID, questionID, answerIDs[0], "req1"); err != nil {
		t.Fatal(err)
	}
	if err := UnacceptAnswer(authorID, questionID, answerIDs[0], "req2"); err != nil {
		t.Fatalf("unaccept: %v", err)
	}
	resolution, accepted := requireQuestionResolution(t, d, questionID)
	if resolution != models.QuestionResolutionOpen || accepted != 0 {
		t.Fatalf("after unaccept: resolution=%d accepted=%d", resolution, accepted)
	}
	var notifyCount int
	if err := d.Get(&notifyCount, "SELECT COUNT(*) FROM notification WHERE user_id = 9002 AND type = ?", models.NotifyTypeUnaccept); err != nil {
		t.Fatal(err)
	}
	if notifyCount != 1 {
		t.Fatalf("want 1 unaccept notification, got %d", notifyCount)
	}
	// 幂等：再次撤销成功
	if err := UnacceptAnswer(authorID, questionID, answerIDs[0], "req3"); err != nil {
		t.Fatalf("idempotent unaccept: %v", err)
	}
}

// TestCloseReopenQuestion 关闭/重开状态机
func TestCloseReopenQuestion(t *testing.T) {
	d := resolutionTempDB(t)
	authorID, questionID, answerIDs := seedQA(t, d, 1)

	// 作者关闭
	if err := SetQuestionStatus(authorID, questionID, "closed", "req1"); err != nil {
		t.Fatalf("close: %v", err)
	}
	// 关闭后不能采纳
	if err := AcceptAnswer(authorID, questionID, answerIDs[0], "req2"); !errors.Is(err, ErrQuestionClosedOp) {
		t.Fatalf("accept on closed: want ErrQuestionClosedOp, got %v", err)
	}
	// 管理员重开他人问题
	if err := SetQuestionStatus(9999, questionID, "open", "req3"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("reopen by plain user: want ErrForbidden, got %v", err)
	}
	// 重开（author 本人）
	if err := SetQuestionStatus(authorID, questionID, "open", "req4"); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	resolution, _ := requireQuestionResolution(t, d, questionID)
	if resolution != models.QuestionResolutionOpen {
		t.Fatalf("after reopen: resolution=%d", resolution)
	}
	// solved 状态不允许直接关闭（须先撤销采纳）
	if err := AcceptAnswer(authorID, questionID, answerIDs[0], "req5"); err != nil {
		t.Fatal(err)
	}
	if err := SetQuestionStatus(authorID, questionID, "closed", "req6"); !errors.Is(err, ErrQuestionTransition) {
		t.Fatalf("close solved: want ErrQuestionTransition, got %v", err)
	}
}

// TestAnswerDeleteClearsAcceptance 采纳答案被作者删除 → 自动恢复 open + 审计
func TestAnswerDeleteClearsAcceptance(t *testing.T) {
	d := resolutionTempDB(t)
	authorID, questionID, answerIDs := seedQA(t, d, 1)
	if err := AcceptAnswer(authorID, questionID, answerIDs[0], "req1"); err != nil {
		t.Fatal(err)
	}
	// 回答作者软删除自己的采纳答案
	if ok, err := mysql.SoftDeleteAnswer(answerIDs[0], 9002); err != nil || !ok {
		t.Fatalf("delete answer: ok=%v err=%v", ok, err)
	}
	resolution, accepted := requireQuestionResolution(t, d, questionID)
	if resolution != models.QuestionResolutionOpen || accepted != 0 {
		t.Fatalf("after delete: resolution=%d accepted=%d (want open/0)", resolution, accepted)
	}
	var auditCount int
	if err := d.Get(&auditCount, "SELECT COUNT(*) FROM audit_log WHERE action = 'acceptance_auto_clear'"); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("want 1 auto-clear audit, got %d", auditCount)
	}
}

// TestModerationTakedownClearsAcceptance 回答被管理员下架 → 自动恢复 open
func TestModerationTakedownClearsAcceptance(t *testing.T) {
	d := resolutionTempDB(t)
	authorID, questionID, answerIDs := seedQA(t, d, 1)
	if err := AcceptAnswer(authorID, questionID, answerIDs[0], "req1"); err != nil {
		t.Fatal(err)
	}
	if err := ModerateContent(8001, "answer", "quarantine", strconv.FormatInt(answerIDs[0], 10), "req2"); err != nil {
		t.Fatalf("moderate: %v", err)
	}
	resolution, accepted := requireQuestionResolution(t, d, questionID)
	if resolution != models.QuestionResolutionOpen || accepted != 0 {
		t.Fatalf("after takedown: resolution=%d accepted=%d", resolution, accepted)
	}
}

// TestUserDisableClearsAcceptances 回答作者被禁用 → 自动清空采纳
func TestUserDisableClearsAcceptances(t *testing.T) {
	d := resolutionTempDB(t)
	authorID, questionID, answerIDs := seedQA(t, d, 1)
	if err := AcceptAnswer(authorID, questionID, answerIDs[0], "req1"); err != nil {
		t.Fatal(err)
	}
	if err := SetUserEnabled(8001, 9002, false, "req2"); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	resolution, accepted := requireQuestionResolution(t, d, questionID)
	if resolution != models.QuestionResolutionOpen || accepted != 0 {
		t.Fatalf("after disable: resolution=%d accepted=%d", resolution, accepted)
	}
}

// TestConcurrentAcceptDifferentAnswers 并发采纳不同回答：恰好一个成功一个冲突
func TestConcurrentAcceptDifferentAnswers(t *testing.T) {
	d := resolutionTempDB(t)
	authorID, questionID, answerIDs := seedQA(t, d, 2)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs <- AcceptAnswer(authorID, questionID, answerIDs[idx], "req-conc")
		}(i)
	}
	wg.Wait()
	close(errs)
	successes, conflicts := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrQuestionStateConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent accept: successes=%d conflicts=%d (want 1/1)", successes, conflicts)
	}
	// 恰好一条采纳通知
	var notifyCount int
	if err := d.Get(&notifyCount, "SELECT COUNT(*) FROM notification WHERE type = ?", models.NotifyTypeAccept); err != nil {
		t.Fatal(err)
	}
	if notifyCount != 1 {
		t.Fatalf("want exactly 1 notification, got %d", notifyCount)
	}
}

// TestListAnswersAcceptedPinned 采纳置顶 + 翻页不重复
func TestListAnswersAcceptedPinned(t *testing.T) {
	d := resolutionTempDB(t)
	authorID, questionID, answerIDs := seedQA(t, d, 3)
	// 采纳最旧的回答（answerIDs[0]，create_time 最早）
	if err := AcceptAnswer(authorID, questionID, answerIDs[0], "req1"); err != nil {
		t.Fatal(err)
	}
	page1, err := ListAnswers(questionID, models.AnswerOrderLatest, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("page1 items=%d", len(page1.Items))
	}
	// 首位必须是采纳回答
	if page1.Items[0].ID != answerIDs[0] || !page1.Items[0].Accepted {
		t.Fatalf("accepted not pinned first: first=%v", page1.Items[0].ID)
	}
	seen := map[int64]bool{answerIDs[0]: true}
	// page1 其余条目计入已见集合
	for _, it := range page1.Items[1:] {
		seen[it.ID] = true
	}
	// 翻页直到取完：不允许重复
	for c := page1; c.HasMore; {
		next, err := ListAnswers(questionID, models.AnswerOrderLatest, c.NextCursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range next.Items {
			if seen[it.ID] {
				t.Fatalf("duplicate answer across pages: %d", it.ID)
			}
			seen[it.ID] = true
			if it.Accepted {
				t.Fatalf("accepted answer leaked into cursor stream: %d", it.ID)
			}
		}
		c = next
	}
	if len(seen) != 3 {
		t.Fatalf("want all 3 answers seen, got %d", len(seen))
	}
}

// TestMyContentFilter 我的内容管理：类型与状态过滤
func TestMyContentFilter(t *testing.T) {
	d := resolutionTempDB(t)
	authorID, questionID, answerIDs := seedQA(t, d, 1)
	// 一篇文章
	articleID := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO article (article_id, author_id, title, content, moderation_status) VALUES (?, ?, '文章标题', '正文', 1)",
		articleID, authorID)

	// published：问题+回答+文章
	pub, err := ListMyContent(authorID, &models.ParamMyContent{Type: models.MeContentQuestion, Status: models.MeContentPublished, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(pub.Items) != 1 || pub.Items[0].ContentID != questionID {
		t.Fatalf("published questions: %+v", pub.Items)
	}
	// 回答属于回答者 9002（seedQA 的 responder），列出其回答
	ans, err := ListMyContent(9002, &models.ParamMyContent{Type: models.MeContentAnswer, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(ans.Items) != 1 || ans.Items[0].ContentID != answerIDs[0] || ans.Items[0].Title != "如何测试采纳闭环" {
		t.Fatalf("answers list: %+v", ans.Items)
	}
	art, err := ListMyContent(authorID, &models.ParamMyContent{Type: models.MeContentArticle, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(art.Items) != 1 || art.Items[0].ContentID != articleID {
		t.Fatalf("articles list: %+v", art.Items)
	}
	// 下架文章后 hidden 过滤命中、published 不命中
	mustExecFlow(t, d, "UPDATE article SET moderation_status = 3 WHERE article_id = ?", articleID)
	hidden, err := ListMyContent(authorID, &models.ParamMyContent{Type: models.MeContentArticle, Status: models.MeContentHidden, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden.Items) != 1 {
		t.Fatalf("hidden articles: %+v", hidden.Items)
	}
	pub2, err := ListMyContent(authorID, &models.ParamMyContent{Type: models.MeContentArticle, Status: models.MeContentPublished, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(pub2.Items) != 0 {
		t.Fatalf("published after quarantine should be empty: %+v", pub2.Items)
	}
	// 非法类型参数拒绝
	if _, err = ListMyContent(authorID, &models.ParamMyContent{Type: "post"}); err == nil {
		t.Fatal("invalid type should fail")
	}
}
