package redis

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
)

// 邮箱验证码相关 key（在 keys.go 常量基础上拼接）
const (
	keyAuthCodePrefix  = "auth:code:"           // auth:code:{purpose}:{email} -> code, TTL 5 分钟
	keyAuthSendLimit   = "auth:send-limit:"     // auth:send-limit:{purpose}:{email} -> 1, TTL 60 秒
	keyAuthSendLimitIP = "auth:send-limit-ip:"  // auth:send-limit-ip:{ip} -> 计数, TTL 1 小时
	keyAuthAttempts    = "auth:attempts:"       // auth:attempts:{purpose}:{email} -> 错误尝试次数, TTL 5 分钟
	authCodeTTL        = 5 * time.Minute
	authSendInterval   = 60 * time.Second
	authIPHourlyLimit  = 20
	authMaxAttempts    = 5 // 验证码连续错误上限，超过后必须重新获取验证码
)

// 业务错误（logic 层用 errors.Is 区分）
var (
	ErrCodeSendTooFrequent = fmt.Errorf("发送过于频繁，请 60 秒后再试")
	ErrCodeSendIPLimited   = fmt.Errorf("发送次数已达上限，请稍后再试")
	ErrVerifyCodeInvalid   = fmt.Errorf("验证码错误或已过期")
	ErrVerifyCodeLocked    = fmt.Errorf("错误次数过多，请重新获取验证码")
)

// genVerifyCode 生成 6 位数字验证码
func genVerifyCode() (string, error) {
	max := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// verifyScript 原子校验：比较、消费、失败计数一次完成
// 返回 0 成功（码已删除）、1 码无效、2 错误次数超限
// GET+DEL 两步操作存在并发窗口（多个请求同时读到同一个码），
// Lua 脚本在 Redis 单线程内执行，保证同一验证码并发提交只有一次成功
var verifyScript = redis.NewScript(`
local saved = redis.call('GET', KEYS[1])
if not saved then
  return 1
end
if saved ~= ARGV[1] then
  local tries = redis.call('INCR', KEYS[2])
  if tries == 1 then
    redis.call('EXPIRE', KEYS[2], ARGV[2])
  end
  if tries > tonumber(ARGV[1]) then
    redis.call('DEL', KEYS[1])
    return 2
  end
  return 1
end
redis.call('DEL', KEYS[1])
redis.call('DEL', KEYS[2])
return 0
`)

// SendAuthCode 生成验证码并写入 Redis；同一邮箱 60 秒内只能发送一次，单 IP 每小时有上限
// 返回验证码内容供调用方发送邮件；邮件发送失败时验证码保持有效，等待倒计时后可重发
func SendAuthCode(email, purpose, ip string) (string, error) {
	ctx := context.Background()

	// 单 IP 每小时上限
	ipKey := getRedisKey(keyAuthSendLimitIP + ip)
	cnt, err := rdb.Incr(ctx, ipKey).Result()
	if err != nil {
		return "", err // Redis 不可用按系统错误上抛，不伪装成验证码错误
	}
	if cnt == 1 {
		rdb.Expire(ctx, ipKey, time.Hour)
	}
	if cnt > authIPHourlyLimit {
		return "", ErrCodeSendIPLimited
	}

	// 同一邮箱发送间隔限制（60 秒）
	limitKey := getRedisKey(keyAuthSendLimit + purpose + ":" + email)
	ok, err := rdb.SetNX(ctx, limitKey, 1, authSendInterval).Result()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrCodeSendTooFrequent
	}

	// 生成验证码并存储，TTL 5 分钟
	code, err := genVerifyCode()
	if err != nil {
		return "", err
	}
	codeKey := getRedisKey(keyAuthCodePrefix + purpose + ":" + email)
	if err = rdb.Set(ctx, codeKey, code, authCodeTTL).Err(); err != nil {
		return "", err
	}
	return code, nil
}

// VerifyAuthCode 原子校验并消费验证码
// 返回 ErrVerifyCodeInvalid / ErrVerifyCodeLocked（业务错误）；
// Redis 连接故障以原生 error 返回，调用方必须区分处理（不能提示"验证码错误"）
func VerifyAuthCode(email, purpose, code string) error {
	ctx := context.Background()
	codeKey := getRedisKey(keyAuthCodePrefix + purpose + ":" + email)
	attemptKey := getRedisKey(keyAuthAttempts + purpose + ":" + email)
	res, err := verifyScript.Run(ctx, rdb, []string{codeKey, attemptKey},
		authMaxAttempts, int(authCodeTTL.Seconds())).Int()
	if err != nil {
		// Redis 脚本执行失败：明确上抛系统错误
		return fmt.Errorf("verify code script failed: %w", err)
	}
	switch res {
	case 0:
		return nil
	case 2:
		return ErrVerifyCodeLocked
	default:
		return ErrVerifyCodeInvalid
	}
}

// token_version 缓存：受保护请求每次都要校验 JWT 版本，
// 缓存 5 分钟避免每个请求查库；密码重置时主动失效
const keyAuthTokenVer = "auth:tokenver:" // auth:tokenver:{uid} -> int, TTL 5 分钟

// GetTokenVersionCached 读取缓存的 token 版本；缓存未命中返回 (-1, nil)
func GetTokenVersionCached(userID int64) (int, error) {
	ctx := context.Background()
	v, err := rdb.Get(ctx, getRedisKey(fmt.Sprintf("%s%d", keyAuthTokenVer, userID))).Int()
	if err == redis.Nil {
		return -1, nil
	}
	return v, err
}

// SetTokenVersionCache 写入 token 版本缓存
func SetTokenVersionCache(userID int64, version int) {
	ctx := context.Background()
	rdb.Set(ctx, getRedisKey(fmt.Sprintf("%s%d", keyAuthTokenVer, userID)), version, 5*time.Minute)
}

// InvalidateTokenVersionCache 使 token 版本缓存失效（密码重置后调用）
func InvalidateTokenVersionCache(userID int64) {
	ctx := context.Background()
	rdb.Del(ctx, getRedisKey(fmt.Sprintf("%s%d", keyAuthTokenVer, userID)))
}
