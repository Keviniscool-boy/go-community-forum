package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"bluebell/pkg/snowflake"

	"github.com/redis/go-redis/v9"
)

// uniqueEmail 每次运行生成唯一测试邮箱，避免限发/尝试计数键跨次运行残留
func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s_%d@test.local", prefix, time.Now().UnixNano())
}

// 验证码集成测试：需要真实 Redis，BB_TEST_REDIS_ADDR 启用（例如 127.0.0.1:26379）
var testRedisAddr = os.Getenv("BB_TEST_REDIS_ADDR")

func requireRedis(t *testing.T) {
	t.Helper()
	if testRedisAddr == "" {
		t.Skip("BB_TEST_REDIS_ADDR not set, skip redis integration test")
	}
	// 测试客户端：直接按环境变量地址构造，不走 viper 配置
	rdb = redis.NewClient(&redis.Options{Addr: testRedisAddr, DB: 0})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect test redis failed: %v", err)
	}
	if err := snowflake.Init("2026-07-24 00:00:00", 1); err != nil {
		t.Fatalf("init snowflake failed: %v", err)
	}
}

// TestVerifyCodeAtomic 并发提交同一验证码恰好一次成功（验收点）
func TestVerifyCodeAtomic(t *testing.T) {
	requireRedis(t)
	email := uniqueEmail("atomic")
	code, err := SendAuthCode(email, "register", "127.0.0.1")
	if err != nil {
		t.Fatalf("send code: %v", err)
	}

	// 30 个 goroutine 同时用同一个码校验
	const concurrency = 30
	var wg sync.WaitGroup
	successCh := make(chan struct{}, concurrency)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := VerifyAuthCode(email, "register", code); err == nil {
				successCh <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(successCh)
	if n := len(successCh); n != 1 {
		t.Fatalf("expected exactly 1 success out of %d concurrent verifies, got %d", concurrency, n)
	}
}

// TestVerifyCodeAttempts 连续错误达到上限后锁定
func TestVerifyCodeAttempts(t *testing.T) {
	requireRedis(t)
	email := uniqueEmail("attempts")
	if _, err := SendAuthCode(email, "register", "127.0.0.1"); err != nil {
		t.Fatalf("send code: %v", err)
	}

	// 连续错 5 次
	for i := 0; i < authMaxAttempts; i++ {
		err := VerifyAuthCode(email, "register", "000000")
		if err == nil {
			t.Fatal("wrong code should fail")
		}
		if !errors.Is(err, ErrVerifyCodeInvalid) {
			t.Fatalf("attempt %d should be ErrVerifyCodeInvalid, got %v", i+1, err)
		}
	}
	// 第 6 次应锁定
	err := VerifyAuthCode(email, "register", "000000")
	if !errors.Is(err, ErrVerifyCodeLocked) {
		t.Fatalf("expected ErrVerifyCodeLocked after max attempts, got %v", err)
	}
	// 锁定后即使码正确也拒绝（码已被清除）
	if err := VerifyAuthCode(email, "register", "000000"); !errors.Is(err, ErrVerifyCodeInvalid) {
		t.Fatalf("locked email should reject, got %v", err)
	}
}

// TestSendCodeRateLimit 同邮箱 60 秒限发
func TestSendCodeRateLimit(t *testing.T) {
	requireRedis(t)
	email := uniqueEmail("ratelimit")
	if _, err := SendAuthCode(email, "register", "9.9.9.9"); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if _, err := SendAuthCode(email, "register", "9.9.9.9"); !errors.Is(err, ErrCodeSendTooFrequent) {
		t.Fatalf("second send should be rate limited, got %v", err)
	}
}
