package logic

// 回答邀请闭环集成测试（PRD §12 验收：并发唯一/频控/状态竞争/可见性/联动钩子）

import (
	"errors"
	"strconv"
	"sync"
	"testing"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// TestInviteSendPermissions 批量邀请：权限/逐人原因/频控/通知
func TestInviteSendPermissions(t *testing.T) {
	d := resolutionTempDB(t)
	authorID := int64(9301)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'inv_author', 'ia@t', 'user', 'x')", authorID)
	questionID := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '待邀请问题', '', ?, 1)",
		questionID, authorID)

	// 5 个可邀请用户 + 1 个关闭偏好 + 1 个屏蔽作者 + 1 个不存在
	inviteeIDs := make([]int64, 0, 5)
	for i := 0; i < 5; i++ {
		uid := int64(9400 + i)
		inviteeIDs = append(inviteeIDs, uid)
		mustExecFlow(t, d,
			"INSERT INTO user (user_id, username, email, role, password, answer_invite_enabled) VALUES (?, ?, ?, 'user', 'x', 1)",
			uid, "inv_u"+strconv.Itoa(i), "iu"+strconv.Itoa(i)+"@t")
	}
	prefOff := int64(9500)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password, answer_invite_enabled) VALUES (?, 'pref_off', 'po@t', 'user', 'x', 0)", prefOff)
	blocked := int64(9501)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'blocked', 'bl@t', 'user', 'x')", blocked)
	mustExecFlow(t, d, "INSERT INTO user_block (blocker_id, blocked_id) VALUES (?, ?)", blocked, authorID)

	// 非作者邀请 → ErrForbidden
	ids := []string{strconv.FormatInt(inviteeIDs[0], 10)}
	p := &models.ParamSendAnswerInvitations{InviteeIDs: ids}
	if _, err := SendAnswerInvitations(9999, questionID, p, "r1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-author invite: want ErrForbidden, got %v", err)
	}

	// 先验证逐人原因（占用配额前）：偏好关闭 / 屏蔽 / 不存在
	reasons, err := SendAnswerInvitations(authorID, questionID,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{
			strconv.FormatInt(prefOff, 10), strconv.FormatInt(blocked, 10), "99999999",
		}}, "r2a")
	if err != nil {
		t.Fatalf("batch reasons: %v", err)
	}
	wantReasons := []string{"对方未开放回答邀请", "无法邀请该用户", "用户不存在或不可邀请"}
	for i, r := range reasons {
		if r.OK || r.Reason != wantReasons[i] {
			t.Fatalf("reason[%d] = %q ok=%v (want %q)", i, r.Reason, r.OK, wantReasons[i])
		}
	}

	// 批量邀请 5 个有效用户 → 全部成功
	all := []string{
		strconv.FormatInt(inviteeIDs[0], 10), strconv.FormatInt(inviteeIDs[1], 10),
		strconv.FormatInt(inviteeIDs[2], 10), strconv.FormatInt(inviteeIDs[3], 10),
		strconv.FormatInt(inviteeIDs[4], 10),
	}
	results, err := SendAnswerInvitations(authorID, questionID, &models.ParamSendAnswerInvitations{InviteeIDs: all}, "r2")
	if err != nil {
		t.Fatalf("batch invite: %v", err)
	}
	okCount, failCount := 0, 0
	for _, r := range results {
		if r.OK {
			okCount++
		} else {
			failCount++
			if r.Reason == "" {
				t.Fatalf("failed result without reason: %+v", r)
			}
		}
	}
	if okCount != 5 || failCount != 0 {
		t.Fatalf("batch invite ok=%d fail=%d (want 5/0)", okCount, failCount)
	}

	// 每个成功邀请都有 type=9 通知
	var notifyCount int
	if err := d.Get(&notifyCount,
		"SELECT COUNT(*) FROM notification WHERE type = ?", models.NotifyTypeAnswerInvite); err != nil {
		t.Fatal(err)
	}
	if notifyCount != 5 {
		t.Fatalf("type 9 notifications = %d (want 5)", notifyCount)
	}

	// 频控：每问题每 24h 最多 5 个不同用户 → 第 6 人返回上限原因
	extra := int64(9600)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'extra', 'ex@t', 'user', 'x')", extra)
	results, err = SendAnswerInvitations(authorID, questionID,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{strconv.FormatInt(extra, 10)}}, "r3")
	if err != nil {
		t.Fatal(err)
	}
	if results[0].OK || results[0].Reason != "该问题今日邀请数已达上限" {
		t.Fatalf("quota result: %+v", results[0])
	}

	// 重复邀请同一人（已有有效邀请）→ 幂等原因
	results, err = SendAnswerInvitations(authorID, questionID,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{strconv.FormatInt(inviteeIDs[0], 10)}}, "r4")
	if err != nil {
		t.Fatal(err)
	}
	if results[0].OK || results[0].Reason != "已有进行中的邀请" {
		t.Fatalf("duplicate active invite: %+v", results[0])
	}
}

// TestInviteQuotaConcurrency 并发邀请同一人：恰好一条有效邀请（生成列唯一键）
func TestInviteQuotaConcurrency(t *testing.T) {
	d := resolutionTempDB(t)
	authorID := int64(9310)
	inviteeID := int64(9311)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'c_author', 'ca@t', 'user', 'x'), (?, 'c_invitee', 'cb@t', 'user', 'x')",
		authorID, inviteeID)
	questionID := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '并发邀请问题', '', ?, 1)",
		questionID, authorID)

	idStr := strconv.FormatInt(inviteeID, 10)
	var wg sync.WaitGroup
	resultsCh := make(chan InviteeResult, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rs, err := SendAnswerInvitations(authorID, questionID,
				&models.ParamSendAnswerInvitations{InviteeIDs: []string{idStr}}, "r-conc")
			if err != nil {
				resultsCh <- InviteeResult{OK: false, Reason: err.Error()}
				return
			}
			resultsCh <- rs[0]
		}()
	}
	wg.Wait()
	close(resultsCh)
	okCount := 0
	for r := range resultsCh {
		if r.OK {
			okCount++
		}
	}
	if okCount != 1 {
		t.Fatalf("concurrent invite: ok=%d (want exactly 1)", okCount)
	}
}

// TestInviteStateMachine 接受/婉拒：幂等/越权/过期/问题失效
func TestInviteStateMachine(t *testing.T) {
	d := resolutionTempDB(t)
	authorID := int64(9320)
	inviteeID := int64(9321)
	otherID := int64(9322)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'sm_author', 'sa@t', 'user', 'x'), (?, 'sm_invitee', 'sb@t', 'user', 'x'), (?, 'sm_other', 'sc@t', 'user', 'x')",
		authorID, inviteeID, otherID)
	questionID := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '状态机问题', '', ?, 1)",
		questionID, authorID)

	results, err := SendAnswerInvitations(authorID, questionID,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{strconv.FormatInt(inviteeID, 10)}}, "r1")
	if err != nil {
		t.Fatal(err)
	}
	invID, err := strconv.ParseInt(results[0].InvitationID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}

	// 非接收者操作 → ErrInviteReceiver
	if err := RespondAnswerInvitation(otherID, invID, "accept", "r2"); !errors.Is(err, ErrInviteReceiver) {
		t.Fatalf("non-receiver accept: want ErrInviteReceiver, got %v", err)
	}
	// 接受
	if err := RespondAnswerInvitation(inviteeID, invID, "accept", "r3"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	// 重复接受幂等成功
	if err := RespondAnswerInvitation(inviteeID, invID, "accept", "r4"); err != nil {
		t.Fatalf("idempotent accept: %v", err)
	}
	// 接受后婉拒 → 状态冲突
	if err := RespondAnswerInvitation(inviteeID, invID, "decline", "r5"); !errors.Is(err, ErrInviteStateOp) {
		t.Fatalf("decline after accept: want ErrInviteStateOp, got %v", err)
	}
	// 提问者收到 type=10 通知
	var notifyCount int
	if err := d.Get(&notifyCount, "SELECT COUNT(*) FROM notification WHERE user_id = ? AND type = ?", authorID, models.NotifyTypeInviteAccepted); err != nil {
		t.Fatal(err)
	}
	if notifyCount != 1 {
		t.Fatalf("invite-accepted notifications = %d (want 1)", notifyCount)
	}

	// 过期邀请：接受返回状态错误且转为 expired
	// （用独立问题：同问题+同人已有 accepted 有效邀请，生成列唯一键不允许再插 pending）
	qExpired := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '过期邀请问题', '', ?, 1)",
		qExpired, authorID)
	expiredInv := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question_answer_invitation (invitation_id, question_id, inviter_id, invitee_id, status, expires_at)"+
			" VALUES (?, ?, ?, ?, 0, NOW() - INTERVAL 1 DAY)", expiredInv, qExpired, authorID, inviteeID)
	if err := RespondAnswerInvitation(inviteeID, expiredInv, "accept", "r6"); !errors.Is(err, ErrInviteStateOp) {
		t.Fatalf("expired accept: want ErrInviteStateOp, got %v", err)
	}
	var expiredStatus int32
	if err := d.Get(&expiredStatus,
		"SELECT status FROM question_answer_invitation WHERE invitation_id = ?", expiredInv); err != nil {
		t.Fatal(err)
	}
	if expiredStatus != models.InviteStatusExpired {
		t.Fatalf("expired invitation status = %d (want 5)", expiredStatus)
	}

	// 婉拒后 7 天内重新邀请 → 冷却原因
	results, err = SendAnswerInvitations(authorID, questionID,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{strconv.FormatInt(inviteeID, 10)}}, "r7")
	// invitee 当前状态是 accepted（上面接受了），活跃邀请仍在 → 原因是已有进行中的邀请
	if err != nil {
		t.Fatal(err)
	}
	if results[0].OK || results[0].Reason != "已有进行中的邀请" {
		t.Fatalf("re-invite with active: %+v", results[0])
	}

	// 问题关闭 → pending 失效 + 新邀请被拒
	q2 := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '关闭问题邀请', '', ?, 1)",
		q2, authorID)
	r2, err := SendAnswerInvitations(authorID, q2,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{strconv.FormatInt(inviteeID, 10)}}, "r8")
	if err != nil {
		t.Fatal(err)
	}
	inv2, err := strconv.ParseInt(r2[0].InvitationID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetQuestionStatus(authorID, q2, "closed", "r9"); err != nil {
		t.Fatalf("close question: %v", err)
	}
	if err := RespondAnswerInvitation(inviteeID, inv2, "accept", "r10"); !errors.Is(err, ErrInviteStateOp) {
		t.Fatalf("accept after close: want ErrInviteStateOp, got %v", err)
	}
	var inv2Status int32
	if err := d.Get(&inv2Status, "SELECT status FROM question_answer_invitation WHERE invitation_id = ?", inv2); err != nil {
		t.Fatal(err)
	}
	if inv2Status != models.InviteStatusExpired {
		t.Fatalf("invitation after close = %d (want expired)", inv2Status)
	}
}

// TestInviteAnsweredTransition 公开回答落地 → 邀请 answered；隔离回答不触发
func TestInviteAnsweredTransition(t *testing.T) {
	d := resolutionTempDB(t)
	authorID := int64(9330)
	inviteeID := int64(9331)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 't_author', 'ta@t', 'user', 'x'), (?, 't_invitee', 'tb@t', 'user', 'x')",
		authorID, inviteeID)
	questionID := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '完成联动问题', '', ?, 1)",
		questionID, authorID)
	results, err := SendAnswerInvitations(authorID, questionID,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{strconv.FormatInt(inviteeID, 10)}}, "r1")
	if err != nil {
		t.Fatal(err)
	}
	invID, _ := strconv.ParseInt(results[0].InvitationID, 10, 64)

	// 被邀请人接受
	if err := RespondAnswerInvitation(inviteeID, invID, "accept", "r2"); err != nil {
		t.Fatal(err)
	}
	// 被邀请人发布公开回答（CreateAnswer moderation=1 → 同事务完成邀请 + type 8 通知提问者）
	ansID := snowflake.GenerateID()
	if err := mysql.CreateAnswer(&models.Answer{
		ID: ansID, QuestionID: questionID, AuthorID: inviteeID, Content: "公开回答",
	}, 1); err != nil {
		t.Fatalf("create answer: %v", err)
	}
	var invStatus int32
	if err := d.Get(&invStatus, "SELECT status FROM question_answer_invitation WHERE invitation_id = ?", invID); err != nil {
		t.Fatal(err)
	}
	if invStatus != models.InviteStatusAnswered {
		t.Fatalf("invitation status after public answer = %d (want answered)", invStatus)
	}
	// 提问者收到 type=8 新回答通知
	var notifyCount int
	if err := d.Get(&notifyCount, "SELECT COUNT(*) FROM notification WHERE user_id = ? AND type = ?", authorID, models.NotifyTypeNewAnswer); err != nil {
		t.Fatal(err)
	}
	if notifyCount != 1 {
		t.Fatalf("type 8 notifications = %d (want 1)", notifyCount)
	}

	// 隔离回答不改变已完成邀请、且审核下架后重算
	quarAns := snowflake.GenerateID()
	if err := mysql.CreateAnswer(&models.Answer{
		ID: quarAns, QuestionID: questionID, AuthorID: inviteeID, Content: "隔离回答",
	}, 3); err != nil {
		t.Fatal(err)
	}
	if err := d.Get(&invStatus, "SELECT status FROM question_answer_invitation WHERE invitation_id = ?", invID); err != nil {
		t.Fatal(err)
	}
	if invStatus != models.InviteStatusAnswered {
		t.Fatalf("invitation status after quarantined answer = %d (still answered)", invStatus)
	}

	// 管理员批准待审回答（moderation 0→1）→ 邀请完成 + type 8（changed=true 才触发）
	authorID2 := int64(9332)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 't_author2', 'tc@t', 'user', 'x')", authorID2)
	questionID2 := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '待审回答联动', '', ?, 1)",
		questionID2, authorID2)
	results2, err := SendAnswerInvitations(authorID2, questionID2,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{strconv.FormatInt(inviteeID, 10)}}, "r3")
	if err != nil {
		t.Fatal(err)
	}
	invID2, _ := strconv.ParseInt(results2[0].InvitationID, 10, 64)
	if err := RespondAnswerInvitation(inviteeID, invID2, "accept", "r4"); err != nil {
		t.Fatal(err)
	}
	pendingAns := snowflake.GenerateID()
	if err := mysql.CreateAnswer(&models.Answer{
		ID: pendingAns, QuestionID: questionID2, AuthorID: inviteeID, Content: "待审回答",
	}, 0); err != nil {
		t.Fatal(err)
	}
	// 待审状态：邀请未完成
	if err := d.Get(&invStatus, "SELECT status FROM question_answer_invitation WHERE invitation_id = ?", invID2); err != nil {
		t.Fatal(err)
	}
	if invStatus != models.InviteStatusAccepted {
		t.Fatalf("invitation while pending answer = %d (want accepted)", invStatus)
	}
	// 审核批准
	if err := ModerateContent(8888, "answer", "approve", strconv.FormatInt(pendingAns, 10), "r5"); err != nil {
		t.Fatalf("moderate approve: %v", err)
	}
	if err := d.Get(&invStatus, "SELECT status FROM question_answer_invitation WHERE invitation_id = ?", invID2); err != nil {
		t.Fatal(err)
	}
	if invStatus != models.InviteStatusAnswered {
		t.Fatalf("invitation after approve = %d (want answered)", invStatus)
	}
	// 重复 approve 幂等：不再产生重复通知
	var type8Count int
	if err := d.Get(&type8Count, "SELECT COUNT(*) FROM notification WHERE user_id = ? AND type = ?", authorID2, models.NotifyTypeNewAnswer); err != nil {
		t.Fatal(err)
	}
	if err := ModerateContent(8888, "answer", "approve", strconv.FormatInt(pendingAns, 10), "r6"); err != nil {
		t.Fatal(err)
	}
	if err := d.Get(&type8Count, "SELECT COUNT(*) FROM notification WHERE user_id = ? AND type = ?", authorID2, models.NotifyTypeNewAnswer); err != nil {
		t.Fatal(err)
	}
	if type8Count != 1 {
		t.Fatalf("type 8 after re-approve = %d (want 1, idempotent)", type8Count)
	}
}

// TestInviteExpiryHooks 屏蔽/禁用/删除问题 → pending 失效
func TestInviteExpiryHooks(t *testing.T) {
	d := resolutionTempDB(t)
	authorID := int64(9340)
	inviteeID := int64(9341)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'e_author', 'ea@t', 'user', 'x'), (?, 'e_invitee', 'eb@t', 'user', 'x')",
		authorID, inviteeID)
	questionID := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '失效钩子问题', '', ?, 1)",
		questionID, authorID)
	results, err := SendAnswerInvitations(authorID, questionID,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{strconv.FormatInt(inviteeID, 10)}}, "r1")
	if err != nil {
		t.Fatal(err)
	}
	invID, _ := strconv.ParseInt(results[0].InvitationID, 10, 64)

	// 屏蔽 → pending 失效
	if err := mysql.BlockUser(authorID, inviteeID); err != nil {
		t.Fatal(err)
	}
	var status int32
	if err := d.Get(&status, "SELECT status FROM question_answer_invitation WHERE invitation_id = ?", invID); err != nil {
		t.Fatal(err)
	}
	if status != models.InviteStatusExpired {
		t.Fatalf("after block = %d (want expired)", status)
	}
	// 屏蔽后新邀请 → 被拒
	results, err = SendAnswerInvitations(authorID, questionID,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{strconv.FormatInt(inviteeID, 10)}}, "r2")
	if err != nil {
		t.Fatal(err)
	}
	if results[0].OK || results[0].Reason != "无法邀请该用户" {
		t.Fatalf("invite after block: %+v", results[0])
	}
	mustExecFlow(t, d, "DELETE FROM user_block WHERE blocker_id = ? AND blocked_id = ?", authorID, inviteeID)

	// 删除问题 → pending 失效
	// （原问题+同人已被 7 天冷却覆盖（PRD §8），换新问题验证删除钩子）
	qDelete := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '删除失效问题', '', ?, 1)",
		qDelete, authorID)
	results, err = SendAnswerInvitations(authorID, qDelete,
		&models.ParamSendAnswerInvitations{InviteeIDs: []string{strconv.FormatInt(inviteeID, 10)}}, "r3")
	if err != nil {
		t.Fatal(err)
	}
	if !results[0].OK || results[0].InvitationID == "" {
		t.Fatalf("invite before delete: %+v", results[0])
	}
	invID, _ = strconv.ParseInt(results[0].InvitationID, 10, 64)
	if ok, err := mysql.SoftDeleteQuestion(qDelete, authorID); err != nil || !ok {
		t.Fatalf("delete question: ok=%v err=%v", ok, err)
	}
	if err := d.Get(&status, "SELECT status FROM question_answer_invitation WHERE invitation_id = ?", invID); err != nil {
		t.Fatal(err)
	}
	if status != models.InviteStatusExpired {
		t.Fatalf("after delete = %d (want expired)", status)
	}
}

// TestUnansweredListing 待回答列表：可见性/已回答移除/话题过滤/游标
func TestUnansweredListing(t *testing.T) {
	d := resolutionTempDB(t)
	authorID := int64(9350)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'u_author', 'ua@t', 'user', 'x')", authorID)
	// 三个公开待回答 + 一个隔离问题 + 一个已解决
	q1 := snowflake.GenerateID()
	q2 := snowflake.GenerateID()
	q3 := snowflake.GenerateID()
	quar := snowflake.GenerateID()
	solved := snowflake.GenerateID()
	mustExecFlow(t, d, "INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '待回答甲', '', ?, 1)", q1, authorID)
	mustExecFlow(t, d, "INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '待回答乙', '', ?, 1)", q2, authorID)
	mustExecFlow(t, d, "INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '待回答丙', '', ?, 1)", q3, authorID)
	mustExecFlow(t, d, "INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '被隔离问题', '', ?, 3)", quar, authorID)
	mustExecFlow(t, d, "INSERT INTO question (question_id, title, description, author_id, moderation_status, resolution_status) VALUES (?, '已解决问题', '', ?, 1, 1)", solved, authorID)

	page1, err := ListUnansweredQuestions(0, "latest", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) != 2 || !page1.HasMore {
		t.Fatalf("page1: n=%d hasMore=%v", len(page1.Items), page1.HasMore)
	}
	seen := map[int64]bool{}
	for _, q := range page1.Items {
		seen[q.ID] = true
		if q.ResolutionStatus != models.QuestionResolutionOpen {
			t.Fatalf("non-open question in unanswered list: %d", q.ID)
		}
	}
	page2, err := ListUnansweredQuestions(0, "latest", page1.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range page2.Items {
		if seen[q.ID] {
			t.Fatalf("duplicate across pages: %d", q.ID)
		}
		seen[q.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("unanswered list seen = %d (want exactly 3 public open)", len(seen))
	}
	if seen[quar] || seen[solved] {
		t.Fatal("quarantined/solved question leaked into unanswered list")
	}

	// 公开回答落地后问题移出待回答列表
	if err := mysql.CreateAnswer(&models.Answer{
		ID: snowflake.GenerateID(), QuestionID: q1, AuthorID: 9351, Content: "x",
	}, 1); err != nil {
		t.Fatal(err)
	}
	page3, err := ListUnansweredQuestions(0, "latest", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range page3.Items {
		if q.ID == q1 {
			t.Fatal("answered question still in unanswered list")
		}
	}
	if len(page3.Items) != 2 {
		t.Fatalf("unanswered after answer = %d (want 2)", len(page3.Items))
	}
}

// TestInviteCandidatesAndPreference 候选排序/排除/理由/偏好开关
func TestInviteCandidatesAndPreference(t *testing.T) {
	d := resolutionTempDB(t)
	authorID := int64(9360)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password) VALUES (?, 'cand_author', 'cda@t', 'user', 'x')", authorID)
	topicID := int64(500)
	mustExecFlow(t, d, "INSERT INTO topic (topic_id, name) VALUES (?, '候选话题')", topicID)
	questionID := snowflake.GenerateID()
	mustExecFlow(t, d,
		"INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '候选测试问题', '', ?, 1)",
		questionID, authorID)
	mustExecFlow(t, d, "INSERT IGNORE INTO question_topic (question_id, topic_id) VALUES (?, ?)", questionID, topicID)

	// 用户 A：同话题 2 个公开回答；用户 B：只关注话题；用户 C：关闭偏好
	aID, bID, cID := int64(9361), int64(9362), int64(9363)
	mustExecFlow(t, d,
		"INSERT INTO user (user_id, username, email, role, password, answer_invite_enabled) VALUES (?, 'cand_a', 'ca2@t', 'user', 'x', 1), (?, 'cand_b', 'cb2@t', 'user', 'x', 1), (?, 'cand_c', 'cc2@t', 'user', 'x', 0)",
		aID, bID, cID)
	for i := 0; i < 2; i++ {
		qid := snowflake.GenerateID()
		mustExecFlow(t, d, "INSERT INTO question (question_id, title, description, author_id, moderation_status) VALUES (?, '话题问题', '', ?, 1)", qid, authorID)
		mustExecFlow(t, d, "INSERT IGNORE INTO question_topic (question_id, topic_id) VALUES (?, ?)", qid, topicID)
		mustExecFlow(t, d, "INSERT INTO answer (answer_id, question_id, author_id, content, moderation_status) VALUES (?, ?, ?, 'x', 1)", snowflake.GenerateID(), qid, aID)
	}
	mustExecFlow(t, d, "INSERT INTO topic_follow (user_id, topic_id) VALUES (?, ?)", bID, topicID)

	candidates, err := ListInviteCandidates(authorID, questionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %d (want 2: A + B, C 偏好关闭被排除)", len(candidates))
	}
	if candidates[0].UserID != strconv.FormatInt(aID, 10) || candidates[0].Reason != "回答过该话题" {
		t.Fatalf("top candidate: %+v", candidates[0])
	}
	if candidates[1].UserID != strconv.FormatInt(bID, 10) || candidates[1].Reason != "关注了该话题" {
		t.Fatalf("second candidate: %+v", candidates[1])
	}

	// 关键词过滤命中
	byName, err := ListInviteCandidates(authorID, questionID, "cand_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(byName) != 1 || byName[0].UserID != strconv.FormatInt(aID, 10) {
		t.Fatalf("keyword filter: %+v", byName)
	}

	// 偏好开关往返
	enabled, err := GetAnswerInvitePreference(cID)
	if err != nil || enabled {
		t.Fatalf("preference initial: enabled=%v err=%v", enabled, err)
	}
	if err := SetAnswerInvitePreference(cID, true); err != nil {
		t.Fatal(err)
	}
	enabled, _ = GetAnswerInvitePreference(cID)
	if !enabled {
		t.Fatal("preference should be enabled after set")
	}
	// 开启后 C 出现在候选
	candidates, err = ListInviteCandidates(authorID, questionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 3 {
		t.Fatalf("candidates after enabling C = %d (want 3)", len(candidates))
	}
}
