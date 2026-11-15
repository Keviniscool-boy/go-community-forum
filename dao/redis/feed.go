package redis

import (
	"context"
	"time"
)

// Feed 缓存 key
const (
	keyFeedHotCache = "feed:hot:v1" // 热榜/推荐流缓存：装配好的条目 JSON，TTL 10 分钟
	feedCacheTTL    = 10 * time.Minute
)

// GetFeedHotCache 读取热榜缓存原始 JSON（不存在返回 nil, nil）
func GetFeedHotCache() ([]byte, error) {
	ctx := context.Background()
	data, err := rdb.Get(ctx, getRedisKey(keyFeedHotCache)).Bytes()
	if err != nil {
		return nil, nil // 缓存未命中视为空
	}
	return data, nil
}

// SetFeedHotCache 写入热榜缓存
func SetFeedHotCache(data []byte) error {
	ctx := context.Background()
	return rdb.Set(ctx, getRedisKey(keyFeedHotCache), data, feedCacheTTL).Err()
}

// SetFeedRefreshLock 热榜刷新锁，避免并发重复计算（拿到锁才刷新）
func SetFeedRefreshLock() (bool, error) {
	ctx := context.Background()
	return rdb.SetNX(ctx, getRedisKey(keyFeedHotCache+":lock"), 1, 30*time.Second).Result()
}

// ReleaseFeedRefreshLock 释放刷新锁
func ReleaseFeedRefreshLock() {
	ctx := context.Background()
	rdb.Del(ctx, getRedisKey(keyFeedHotCache+":lock"))
}
