package logic

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/jwt"
	"bluebell/pkg/mail"
	"bluebell/pkg/snowflake"

	"golang.org/x/crypto/bcrypt"
	"go.uber.org/zap"
)

// 邮箱认证业务错误
// 注意：对外响应不区分"邮箱已注册/未注册"，避免账号存在性枚举；
// 存在性相关错误只在内部流转，controller 统一映射为模糊错误码
var (
	ErrEmailExist       = errors.New("邮箱已被注册")
	ErrEmailNotFound    = errors.New("该邮箱尚未注册")
	ErrAccountBanned    = errors.New("账号已被禁用")
	ErrRegisterConflict = errors.New("注册失败：邮箱或用户名已被使用")
)

// normalizeEmail 服务端统一规范化：去除首尾空白；域名转小写；
// local-part 不做任何供应商特定的转换（保留原样）。
// MySQL 唯一索引使用 utf8mb4_general_ci（大小写不敏感），与应用比较语义一致
func normalizeEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return email
	}
	return email[:at] + strings.ToLower(email[at:])
}

// validatePasswordStrength 密码 8-64 字节（UTF-8 字节长度，先于 bcrypt 校验，
// 避免 bcrypt 72 字节静默截断）；binding 的 min/max 按 rune 计数，这里补字节口径
func validatePasswordStrength(password string) error {
	if n := len(password); n < 8 || n > 64 || !utf8.ValidString(password) {
		return errors.New("密码长度需为 8-64 字节")
	}
	return nil
}

// SendCode 发送邮箱验证码
// 防枚举：不再预先检查邮箱是否存在，统一发码（注册用不了的码在最终注册时被统一错误拦截）
// purpose=bind_email 时由路由层强制鉴权，只对已登录用户放行
func SendCode(rawEmail, purpose, ip string) error {
	email := normalizeEmail(rawEmail)
	code, err := redis.SendAuthCode(email, purpose, ip)
	if err != nil {
		return err
	}
	// 邮件发送失败：立即失效对应验证码，防止无效码残留被重试使用
	if err = mail.SendCodeMail(email, code); err != nil {
		redis.InvalidateAuthCode(email, purpose)
		return err
	}
	return nil
}

// RegisterByEmail 邮箱验证码注册，成功后返回 token 和用户信息
func RegisterByEmail(p *models.ParamEmailRegister) (token string, user *models.AuthUser, err error) {
	if err = validatePasswordStrength(p.Password); err != nil {
		return "", nil, err
	}
	email := normalizeEmail(p.Email)
	// 校验验证码（Lua 原子：比较、消费、失败计数；Redis 故障返回系统错误）
	if err = redis.VerifyAuthCode(email, models.CodePurposeRegister, p.Code); err != nil {
		return "", nil, err
	}
	// 唯一性检查：命中冲突走统一的模糊错误，避免暴露具体原因
	emailExist, err := mysql.CheckEmailExist(email)
	if err != nil {
		return "", nil, err
	}
	usernameExist, err := mysql.CheckUsernameExist(p.Username)
	if err != nil {
		return "", nil, err
	}
	if emailExist || usernameExist {
		return "", nil, ErrRegisterConflict
	}
	// 生成 UID 并入库（密码 bcrypt 哈希在 dao 层处理）
	newUser := &models.User{
		UserID:   snowflake.GenerateID(),
		Username: p.Username,
		Email:    email,
		Password: p.Password,
	}
	if err = mysql.CreateEmailUser(newUser); err != nil {
		// 并发下撞唯一索引同样收敛为模糊错误
		if errors.Is(err, mysql.ErrUserExist) {
			return "", nil, ErrRegisterConflict
		}
		return "", nil, err
	}
	// 注册成功直接签发 token
	token, err = jwt.GenToken(newUser.UserID, newUser.Username, 0)
	if err != nil {
		return "", nil, err
	}
	return token, toAuthUser(newUser), nil
}

// LoginByEmail 邮箱密码登录，成功后返回 token 和用户信息
// 响应统一：邮箱不存在与密码错误返回同一个 ErrInvalidCredentials
func LoginByEmail(p *models.ParamEmailLogin) (token string, user *models.AuthUser, err error) {
	email := normalizeEmail(p.Email)
	loginUser := &models.User{
		Email:    email,
		Password: p.Password,
	}
	if err = mysql.LoginByEmail(loginUser); err != nil {
		switch {
		case errors.Is(err, mysql.ErrInvalidCredentials):
			return "", nil, ErrInvalidCredentials
		case errors.Is(err, mysql.ErrAccountBanned):
			return "", nil, ErrAccountBanned
		default:
			return "", nil, err
		}
	}
	// 更新最后登录时间（失败不影响登录）
	if err := mysql.UpdateLastLogin(loginUser.UserID); err != nil {
		zap.L().Warn("update last_login_at failed", zap.Error(err))
	}
	token, err = jwt.GenToken(loginUser.UserID, loginUser.Username, loginUser.TokenVersion)
	if err != nil {
		return "", nil, err
	}
	return token, toAuthUser(loginUser), nil
}

// ---------- 两段式密码重置 ----------

// VerifyResetCode 找回密码第一步：验证码（原子消费）换一次性重置凭据
// 凭据明文只出现在本次响应；服务端只存 sha256 摘要与过期时间
func VerifyResetCode(p *models.ParamVerifyResetCode) (resetToken string, expiresIn int, err error) {
	email := normalizeEmail(p.Email)
	// 验证码校验：仅 email_verified=1 的真实邮箱用户能收到重置验证码，此处码错误/过期即拒绝
	if err = redis.VerifyAuthCode(email, models.CodePurposeResetPassword, p.Code); err != nil {
		return "", 0, err
	}
	// 生成 256 位随机凭据
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", 0, err
	}
	resetToken = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(resetToken))
	tokenHash := hex.EncodeToString(sum[:])
	expiresAt := time.Now().UTC().Add(models.ResetTokenTTLSeconds * time.Second)
	if err = mysql.UpsertResetToken(email, tokenHash, expiresAt); err != nil {
		// 凭据写入失败：验证码已消费，用户需重新获取验证码；错误上抛为服务错误
		return "", 0, err
	}
	return resetToken, models.ResetTokenTTLSeconds, nil
}

// ResetPasswordWithToken 找回密码第二步：凭据单次消费 + 改密 + 撤销所有旧会话（同事务）
// 统一响应：凭据无效与邮箱不存在返回同一个模糊错误，防枚举
func ResetPasswordWithToken(p *models.ParamResetWithToken) error {
	if err := validatePasswordStrength(p.Password); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(p.ResetToken))
	tokenHash := hex.EncodeToString(sum[:])
	// dao 层要求传入 bcrypt 哈希（与其他写入路径一致）
	hashed, err := bcrypt.GenerateFromPassword([]byte(p.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	email, err := mysql.ConsumeResetToken(tokenHash, string(hashed))
	if err != nil {
		return err
	}
	// 清除该用户 token 版本缓存，旧 JWT 在下一次受保护请求即失效
	if u, err := mysql.GetUserByEmail(email); err == nil {
		redis.InvalidateTokenVersionCache(u.UserID)
	}
	return nil
}

// ---------- 邮箱绑定与全局登出 ----------

// BindEmail 旧账号绑定真实邮箱：验证码（bind_email purpose，原子消费）+ 唯一约束兜底
// 占位邮箱（legacy，未验证）绑定真实邮箱后即完成账号迁移闭环
func BindEmail(userID int64, p *models.ParamBindEmail) error {
	email := normalizeEmail(p.Email)
	if err := redis.VerifyAuthCode(email, models.CodePurposeBindEmail, p.Code); err != nil {
		return err
	}
	if err := mysql.BindEmail(userID, email); err != nil {
		return err
	}
	// 邮箱是身份字段，绑定后撤销所有旧会话（保守做法：重新登录）
	if _, err := mysql.IncrementTokenVersion(userID); err != nil {
		return err
	}
	redis.InvalidateTokenVersionCache(userID)
	return nil
}

// LogoutAll 全部设备登出：递增 token_version 使该用户所有已签发 token 失效
func LogoutAll(userID int64) error {
	if _, err := mysql.IncrementTokenVersion(userID); err != nil {
		return err
	}
	redis.InvalidateTokenVersionCache(userID)
	return nil
}

// GetMe 查询当前登录用户信息
func GetMe(userID int64) (*models.AuthUser, error) {
	user, err := mysql.GetUserDetailByID(userID)
	if err != nil {
		return nil, err
	}
	return toAuthUser(user), nil
}

// toAuthUser 转换为对外暴露的用户信息（不包含密码等敏感字段）
func toAuthUser(u *models.User) *models.AuthUser {
	return &models.AuthUser{
		UserID:    int64ToString(u.UserID),
		Email:     u.Email,
		Username:  u.Username,
		AvatarURL: u.AvatarURL,
		Bio:       u.Bio,
	}
}

func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
