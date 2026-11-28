package logic

// 私信集成测试（第五阶段 PRD F6 验收标准）

import (
	"errors"
	"strconv"
	"sync"
	"testing"

	"bluebell/dao/mysql"
	"bluebell/models"
)

func TestDMConcurrencyAndReadWatermark(t *testing.T) {
	aID, bID, _ := phase5Env(t)
	d := mysql.GetGlobalDB()

	// 并发开聊同一对用户 → 只产生一个会话（唯一键 + 事务重试语义）
	var wg sync.WaitGroup
	threadIDs := make(chan int64, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(sender int64) {
			defer wg.Done()
			_, tid, err := OpenDirectMessage(sender, strconv.FormatInt(aID, 10), &models.ParamSendMessage{Content: "hi"})
			if err == nil {
				threadIDs <- tid
			} else {
				threadIDs <- -1
			}
		}(bID)
	}
	wg.Wait()
	close(threadIDs)
	okThreads := make(map[int64]int)
	for tid := range threadIDs {
		if tid > 0 {
			okThreads[tid]++
		}
	}
	if len(okThreads) != 1 {
		t.Fatalf("concurrent open chat produced %d threads (want 1)", len(okThreads))
	}

	// 未读水位：b→a 发 3 条，a 打开会话前未读 3，打开后归零
	tid := func() int64 {
		for k := range okThreads {
			return k
		}
		return 0
	}()
	if _, _, err := ReplyInThread(bID, strconv.FormatInt(tid, 10), &models.ParamSendMessage{Content: "第二条"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReplyInThread(bID, strconv.FormatInt(tid, 10), &models.ParamSendMessage{Content: "第三条"}); err != nil {
		t.Fatal(err)
	}
	unread, err := mysql.CountThreadUnread(aID)
	if err != nil {
		t.Fatal(err)
	}
	if unread != 3 {
		t.Fatalf("unread = %d (want 3)", unread)
	}
	// a 打开会话（更新已读水位）
	if _, err := GetThreadMessages(aID, strconv.FormatInt(tid, 10), "", 30); err != nil {
		t.Fatal(err)
	}
	unread, _ = mysql.CountThreadUnread(aID)
	if unread != 0 {
		t.Fatalf("unread after open = %d (want 0)", unread)
	}
	// b 自己的视角：自己发的消息不算未读
	unreadB, _ := mysql.CountThreadUnread(bID)
	if unreadB != 0 {
		t.Fatalf("sender unread = %d (want 0)", unreadB)
	}
	// 通知聚合：连发多条只 1 条未读通知（10 分钟窗）
	var dmNotifs int64
	if err := d.Get(&dmNotifs,
		"SELECT COUNT(*) FROM notification WHERE user_id = ? AND type = ?", aID, models.NotifyTypeDM); err != nil {
		t.Fatal(err)
	}
	if dmNotifs != 1 {
		t.Fatalf("DM notifications = %d (want 1, 10min window)", dmNotifs)
	}
}

func TestDMBlocksLimitsWords(t *testing.T) {
	aID, bID, _ := phase5Env(t)
	d := mysql.GetGlobalDB()
	questionID := phase5Question(t, d, aID, "私信限流问题")

	// 拉黑：b 拉黑 a → a 不能开聊
	if err := mysql.BlockUser(bID, aID); err != nil {
		t.Fatal(err)
	}
	_, _, err := OpenDirectMessage(aID, strconv.FormatInt(bID, 10), &models.ParamSendMessage{Content: "hi"})
	if !errors.Is(err, ErrMessageBlocked) {
		t.Fatalf("blocked DM: want ErrMessageBlocked, got %v", err)
	}
	if err := mysql.UnblockUser(bID, aID); err != nil {
		t.Fatal(err)
	}

	// 敏感词拒绝词：命中直接拒绝发送
	if err := mysql.AddSensitiveWord("拒绝测试词", 1, aID); err != nil {
		t.Fatal(err)
	}
	InvalidateSensitiveWords()
	_, _, err = OpenDirectMessage(aID, strconv.FormatInt(bID, 10), &models.ParamSendMessage{Content: "包含拒绝测试词的消息"})
	if !errors.Is(err, ErrMessageRejected) {
		t.Fatalf("reject-word DM: want ErrMessageRejected, got %v", err)
	}

	// 非互关每日 20 条（Redis 计数）：连发 20 条成功，第 21 条限流
	sent := 0
	for i := 0; i < 21; i++ {
		_, _, err := OpenDirectMessage(aID, strconv.FormatInt(bID, 10), &models.ParamSendMessage{
			Content: "普通消息 " + strconv.Itoa(i) + " " + strconv.FormatInt(questionID, 10),
		})
		if err == nil {
			sent++
		} else if errors.Is(err, ErrMessageRateLimit) {
			break
		} else {
			t.Fatalf("unexpected err at %d: %v", i, err)
		}
	}
	if sent != 20 {
		t.Fatalf("sent = %d (want 20 before rate limit)", sent)
	}

	// 自己给自己发 → 拒绝
	if _, _, err := OpenDirectMessage(aID, strconv.FormatInt(aID, 10), &models.ParamSendMessage{Content: "self"}); !errors.Is(err, ErrMessageSelf) {
		t.Fatalf("self DM: want ErrMessageSelf, got %v", err)
	}

	// 越权会话：c 访问 a/b 的会话 → ErrMessageNotPeer
	threads, err := ListMyThreads(aID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads.Items) == 0 {
		t.Fatal("no thread for a")
	}
	// c 不是参与者：越权访问
	if _, err := GetThreadMessages(9403, strconv.FormatInt(threads.Items[0].ThreadID, 10), "", 30); !errors.Is(err, ErrMessageNotPeer) {
		t.Fatalf("cross-user thread: want ErrMessageNotPeer, got %v", err)
	}
}
