package logic

import (
	"testing"
	"time"

	"bluebell/models"
)

// TestTimeCursorRoundTrip 时间游标编解码往返一致性
func TestTimeCursorRoundTrip(t *testing.T) {
	now := time.Now()
	id := int64(1234567890123)

	encoded := encodeTimeCursor(now, id)
	c, err := parseTimeCursor(encoded)
	if err != nil {
		t.Fatalf("parseTimeCursor failed: %v", err)
	}
	if !c.Time.Equal(now) {
		t.Fatalf("time mismatch: want %v got %v", now, c.Time)
	}
	if c.ID != id {
		t.Fatalf("id mismatch: want %d got %d", id, c.ID)
	}
}

// TestTimeCursorInvalid 非法游标必须报错而不是panic
func TestTimeCursorInvalid(t *testing.T) {
	if _, err := parseTimeCursor("not-a-valid-cursor!!"); err == nil {
		t.Fatal("expected error for invalid cursor")
	}
}

// TestParseOffsetCursor offset 游标边界
func TestParseOffsetCursor(t *testing.T) {
	if offset, version, err := parseOffsetCursor(""); err != nil || offset != 0 || version != 0 {
		t.Fatalf("empty cursor should be offset 0, got %d err %v", offset, err)
	}
	encoded := encodeOffsetCursor(40, 12345)
	offset, version, err := parseOffsetCursor(encoded)
	if err != nil || offset != 40 || version != 12345 {
		t.Fatalf("offset mismatch: got %d v%d err %v", offset, version, err)
	}
	if _, _, err := parseOffsetCursor("###bad###"); err == nil {
		t.Fatal("expected error for invalid offset cursor")
	}
}

// TestFeedBaseScore 推荐分：互动与新鲜度方向正确
func TestFeedBaseScore(t *testing.T) {
	now := time.Now()
	hot := &models.FeedItem{
		CreateTime: now,
		Stats:      models.FeedStats{VoteCount: 10, CommentCount: 5, CollectCount: 2},
	}
	old := &models.FeedItem{
		CreateTime: now.Add(-72 * time.Hour),
		Stats:      models.FeedStats{VoteCount: 10, CommentCount: 5, CollectCount: 2},
	}
	scoreHot := feedBaseScore(hot)
	scoreOld := feedBaseScore(old)
	if scoreHot <= scoreOld {
		t.Fatalf("fresh item should score higher: hot=%.2f old=%.2f", scoreHot, scoreOld)
	}

	// 互动权重：收藏(4) > 评论(3) > 赞同(2)
	moreCollect := &models.FeedItem{CreateTime: now, Stats: models.FeedStats{CollectCount: 1}}
	moreVote := &models.FeedItem{CreateTime: now, Stats: models.FeedStats{VoteCount: 1}}
	if feedBaseScore(moreCollect) <= feedBaseScore(moreVote) {
		t.Fatal("collect should weigh more than vote")
	}
}

// TestTruncateExcerpt 摘要截断不产生半个中文
func TestTruncateExcerpt(t *testing.T) {
	long := "这是一段很长的中文摘要内容，用来测试截断逻辑是否正确。"
	got := truncateExcerpt(long, 5)
	if got != "这是一段很…" {
		t.Fatalf("unexpected excerpt: %q", got)
	}
	if truncateExcerpt("short", 10) != "short" {
		t.Fatal("short excerpt should be unchanged")
	}
}
