package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Feed 缓存 key
const (
	keyFeedHotCache = "feed:hot:v3" // 热榜/推荐流缓存：JSON（含快照版本号），TTL 10 分钟
	keyFeedHotLock  = "feed:hot:v3:lock"
	feedCacheTTL    = 10 * time.Minute
	feedLockTTL     = 30 * time.Second
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

// SetFeedRefreshLock 获取热榜刷新锁（唯一持有者标识）
// 锁带随机 holder，释放时校验归属，避免误删他人的锁
func SetFeedRefreshLock(holder string) (bool, error) {
	ctx := context.Background()
	return rdb.SetNX(ctx, getRedisKey(keyFeedHotLock), holder, feedLockTTL).Result()
}

// releaseLockScript 持有者匹配才删除锁（原子）
var releaseLockScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`)

// ReleaseFeedRefreshLock 校验持有者后释放锁
func ReleaseFeedRefreshLock(holder string) {
	ctx := context.Background()
	releaseLockScript.Run(ctx, rdb, []string{getRedisKey(keyFeedHotLock)}, holder)
}
