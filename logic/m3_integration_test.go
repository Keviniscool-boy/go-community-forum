package logic

// 第五阶段 M3 集成测试（PRD F7 通知偏好 / F8 问题重定向）

import (
	"errors"
	"strconv"
	"testing"

	"bluebell/dao/mysql"
)

// TestNotifyPreferences 通知偏好（PRD F7）：关闭 like 后点赞不产生通知；DM 同理；缺省全开
func TestNotifyPreferences(t *testing.T) {
	aID, bID, _ := phase5Env(t)
	d := mysql.GetGlobalDB()
	questionID := phase5Question(t, d, aID, "偏好问题")
	ansID := phase5Answer(t, d, aID, questionID, "回答")

	// 缺省全开：b 点赞 → a 收到通知
	if err := mysql.VoteAnswer(bID, ansID, 1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := d.Get(&n, "SELECT COUNT(*) FROM notification WHERE user_id = ? AND type = 1", aID); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("default like notification = %d (want 1)", n)
	}

	// a 关闭 like 偏好
	if err := SetNotifySettings(aID, map[string]bool{"like": false}); err != nil {
		t.Fatal(err)
	}
	// 取消旧赞再重赞（重新触发"首次赞同"通知语义）
	if err := mysql.VoteAnswer(bID, ansID, 0); err != nil {
		t.Fatal(err)
	}
	if err := mysql.VoteAnswer(bID, ansID, 1); err != nil {
		t.Fatal(err)
	}
	if err := d.Get(&n, "SELECT COUNT(*) FROM notification WHERE user_id = ? AND type = 1", aID); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("like notification with pref off = %d (want still 1, suppressed)", n)
	}

	// GET 返回 like=false
	pref, err := GetNotifySettings(aID)
	if err != nil {
		t.Fatal(err)
	}
	if pref["like"] {
		t.Fatal("like pref should be false")
	}
	if !pref["comment"] {
		t.Fatal("comment pref should default true")
	}

	// 非法键拒绝
	if err := SetNotifySettings(aID, map[string]bool{"hacker": false}); !errors.Is(err, ErrInvalidNotifySetting) {
		t.Fatalf("invalid key: want ErrInvalidNotifySetting, got %v", err)
	}

	// 恢复开启 → 通知恢复
	if err := SetNotifySettings(aID, map[string]bool{"like": true}); err != nil {
		t.Fatal(err)
	}
	if err := mysql.VoteAnswer(bID, ansID, 0); err != nil {
		t.Fatal(err)
	}
	if err := mysql.VoteAnswer(bID, ansID, 1); err != nil {
		t.Fatal(err)
	}
	if err := d.Get(&n, "SELECT COUNT(*) FROM notification WHERE user_id = ? AND type = 1", aID); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("like notification after re-enable = %d (want 2)", n)
	}
}

// TestQuestionRedirect 问题重定向（PRD F8）：治理动作可逆、防环、非公开目标拒绝
func TestQuestionRedirect(t *testing.T) {
	aID, _, _ := phase5Env(t)
	d := mysql.GetGlobalDB()
	qA := phase5Question(t, d, aID, "重复问题A")
	qB := phase5Question(t, d, aID, "目标问题B")
	qC := phase5Question(t, d, aID, "目标问题C")

	operator := int64(9500)
	mustExecP5(t, d, "INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'admin5', 'ad5@t', 'admin', 'x')", operator)

	// 自环拒绝
	if err := SetQuestionRedirect(operator, qA, qA, ""); !errors.Is(err, mysql.ErrInvalidParams) {
		t.Fatalf("self redirect: want ErrInvalidParams, got %v", err)
	}
	// 正常重定向 A→B
	if err := SetQuestionRedirect(operator, qA, qB, "重复合并"); err != nil {
		t.Fatalf("redirect: %v", err)
	}
	target, redirected, err := mysql.QuestionRedirected(qA)
	if err != nil || !redirected || target != qB {
		t.Fatalf("redirected = %v target = %d (want true, %d)", redirected, target, qB)
	}
	// 防环：B→A 应被拒（A 已指向 B，形成环）
	if err := SetQuestionRedirect(operator, qB, qA, ""); !errors.Is(err, mysql.ErrInvalidParams) {
		t.Fatalf("cycle redirect: want ErrInvalidParams, got %v", err)
	}
	// 不存在目标
	if err := SetQuestionRedirect(operator, qA, 99999999, ""); !errors.Is(err, mysql.ErrQuestionNotExist) {
		t.Fatalf("missing target: want ErrQuestionNotExist, got %v", err)
	}
	// 审计记录
	var auditCount int
	if err := d.Get(&auditCount, "SELECT COUNT(*) FROM audit_log WHERE action = 'question_redirect' AND target_id = ?",
		strconv.FormatInt(qA, 10)); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("redirect audits = %d (want 1)", auditCount)
	}

	// 链式重定向：B→C 后 A→B→C（沿链不回环时合法）
	if err := SetQuestionRedirect(operator, qB, qC, ""); err != nil {
		t.Fatal(err)
	}
	// 此时 A→B→C 会构成环，A 改指 C 合法
	if err := SetQuestionRedirect(operator, qA, qC, ""); err != nil {
		t.Fatalf("redirect A→C after chain: %v", err)
	}

	// 可逆：清除
	if err := ClearQuestionRedirect(operator, qA); err != nil {
		t.Fatal(err)
	}
	_, redirected, _ = mysql.QuestionRedirected(qA)
	if redirected {
		t.Fatal("redirect should be cleared")
	}
	// 清除审计
	if err := d.Get(&auditCount, "SELECT COUNT(*) FROM audit_log WHERE action = 'question_redirect_clear' AND actor_id = ?", operator); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("clear audits = %d (want 1)", auditCount)
	}
}
