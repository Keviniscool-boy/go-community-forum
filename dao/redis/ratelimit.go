package redis

import (
	"context"
	"time"
)

// IncrRateLimitTTL 通用限流计数（自定义 key 与 TTL 秒），返回当前计数
func IncrRateLimitTTLX(key string, ttlSeconds int) (int64, error) {
	ctx := context.Background()
	cnt, err := rdb.Incr(ctx, getRedisKey(key)).Result()
	if err != nil {
		return 0, err
	}
	if cnt == 1 {
		rdb.Expire(ctx, getRedisKey(key), time.Duration(ttlSeconds)*time.Second)
	}
	return cnt, nil
}
