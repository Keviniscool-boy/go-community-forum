package logic

// 用户主页可见性集成测试（验收 P1-1）：被隔离/隐藏的内容不得从个人主页旁路读到，
// 统计计数与列表口径一致，被禁用作者主页统一不可枚举。

import (
	"errors"
	"testing"

	"bluebell/dao/mysql"
	"bluebell/pkg/snowflake"
)

func TestUserProfileVisibilityIsolation(t *testing.T) {
	d := resolutionTempDB(t)
	authorID := int64(9101)
	responderID := int64(9102)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'p_author', 'pa@t', 'user', 'x'), (?, 'p_answerer', 'pb@t', 'user', 'x')",
		authorID, responderID)

	// 3 个问题：公开、隔离(moderation=3)、隐藏(visibility=0)
	pubQ := snowflake.GenerateID()
	quarantineQ := snowflake.GenerateID()
	hiddenQ := snowflake.GenerateID()
	mustExecFlow(t, d, "INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '公开问题', '', ?, 1)", pubQ, authorID)
	mustExecFlow(t, d, "INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '被隔离问题', '', ?, 3)", quarantineQ, authorID)
	mustExecFlow(t, d, "INSERT INTO question (question_id, title, description, author_id, moderation_status, visibility) VALUES (?, '被隐藏问题', '', ?, 1, 0)", hiddenQ, authorID)

	// 回答：公开问题的公开回答 + 隐藏问题下的回答（应不可见）
	pubA := snowflake.GenerateID()
	hiddenParentA := snowflake.GenerateID()
	mustExecFlow(t, d, "INSERT INTO answer (answer_id, question_id, author_id, content, moderation_status) VALUES (?, ?, ?, '公开回答', 1)", pubA, pubQ, responderID)
	mustExecFlow(t, d, "INSERT INTO answer (answer_id, question_id, author_id, content, moderation_status) VALUES (?, ?, ?, '隐藏问题下的回答', 1)", hiddenParentA, hiddenQ, responderID)
	mustExecFlow(t, d, "UPDATE question SET answer_count = 1 WHERE question_id = ?", pubQ)

	// 列表：只有公开问题
	qs, err := ListUserQuestions(authorID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) != 1 || qs[0].ID != pubQ {
		t.Fatalf("questions list = %d items (want only public), got %+v", len(qs), qs)
	}
	// 回答列表：隐藏问题下的回答不可见
	as, err := ListUserAnswers(responderID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(as) != 1 || as[0].ID != pubA {
		t.Fatalf("answers list = %d items (want only public)", len(as))
	}
	// 计数与列表口径一致
	qc, err := mysql.CountUserQuestions(authorID)
	if err != nil || qc != 1 {
		t.Fatalf("CountUserQuestions = %d err=%v (want 1)", qc, err)
	}
	ac, err := mysql.CountUserAnswers(responderID)
	if err != nil || ac != 1 {
		t.Fatalf("CountUserAnswers = %d err=%v (want 1)", ac, err)
	}

	// 禁用作者：主页统一不存在（防枚举）
	mustExecFlow(t, d, "UPDATE user SET status = 0 WHERE user_id = ?", authorID)
	if _, err = GetUserProfile(authorID, 0); !errors.Is(err, mysql.ErrUserNotExist) {
		t.Fatalf("disabled author profile: want ErrUserNotExist, got %v", err)
	}
}
