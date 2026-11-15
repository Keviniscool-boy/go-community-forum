package redis

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

// 邮箱验证码相关 key（在 keys.go 常量基础上拼接）
const (
	keyAuthCodePrefix     = "auth:code:"        // auth:code:{purpose}:{email} -> code, TTL 5 分钟
	keyAuthSendLimit      = "auth:send-limit:"  // auth:send-limit:{purpose}:{email} -> 1, TTL 60 秒
	keyAuthSendLimitIP    = "auth:send-limit-ip:" // auth:send-limit-ip:{ip} -> 计数, TTL 1 小时
	authCodeTTL           = 5 * time.Minute
	authSendInterval      = 60 * time.Second
	authIPHourlyLimit     = 20
)

// 业务错误（logic 层用 errors.Is 区分）
var (
	ErrCodeSendTooFrequent = fmt.Errorf("发送过于频繁，请 60 秒后再试")
	ErrCodeSendIPLimited   = fmt.Errorf("今日发送次数已达上限，请稍后再试")
	ErrVerifyCodeInvalid   = fmt.Errorf("验证码错误或已过期")
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

// SendAuthCode 生成验证码并写入 Redis；同一邮箱 60 秒内只能发送一次，单 IP 每小时有上限
// 返回验证码内容供调用方发送邮件
func SendAuthCode(email, purpose, ip string) (string, error) {
	ctx := context.Background()

	// 单 IP 每小时上限
	ipKey := getRedisKey(keyAuthSendLimitIP + ip)
	cnt, err := rdb.Incr(ctx, ipKey).Result()
	if err != nil {
		return "", err
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

// VerifyAuthCode 校验验证码，成功后立即删除（一次性使用）
func VerifyAuthCode(email, purpose, code string) error {
	ctx := context.Background()
	codeKey := getRedisKey(keyAuthCodePrefix + purpose + ":" + email)
	saved, err := rdb.Get(ctx, codeKey).Result()
	if err != nil {
		// key 不存在视为验证码过期
		return ErrVerifyCodeInvalid
	}
	if saved != code {
		return ErrVerifyCodeInvalid
	}
	rdb.Del(ctx, codeKey)
	return nil
}
