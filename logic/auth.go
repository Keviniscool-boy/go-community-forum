package logic

import (
	"errors"
	"strconv"
	"strings"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/jwt"
	"bluebell/pkg/mail"
	"bluebell/pkg/snowflake"
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

// normalizeEmail 服务端统一规范化：去空白 + 转小写，所有流程（发码/注册/登录/重置）一致
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// SendCode 发送邮箱验证码
// 防枚举：不再预先检查邮箱是否存在，统一发码（注册用不了的码在最终注册时被统一错误拦截）
func SendCode(rawEmail, purpose, ip string) error {
	email := normalizeEmail(rawEmail)
	code, err := redis.SendAuthCode(email, purpose, ip)
	if err != nil {
		return err
	}
	// 发送邮件失败时验证码仍在 Redis 中保持有效，等待 60s 倒计时后可重发
	return mail.SendCodeMail(email, code)
}

// RegisterByEmail 邮箱验证码注册，成功后返回 token 和用户信息
func RegisterByEmail(p *models.ParamEmailRegister) (token string, user *models.AuthUser, err error) {
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

// ResetPassword 找回密码：验证一次性重置码 → 更新密码 → token_version+1 撤销旧会话
// 统一响应：无论邮箱是否存在，验证码正确即返回成功（邮箱不存在时不做任何修改），
// 避免枚举；占位邮箱（legacy，未验证）不可重置
func ResetPassword(p *models.ParamResetPassword) error {
	email := normalizeEmail(p.Email)
	// 原子校验并消费重置码
	if err := redis.VerifyAuthCode(email, models.CodePurposeResetPassword, p.Code); err != nil {
		return err
	}
	updated, err := mysql.UpdatePasswordByEmail(email, p.Password)
	if err != nil {
		return err
	}
	if updated {
		// 清除该用户 token 版本缓存，旧 JWT 在下一次受保护请求即失效
		// 需要查一次 user_id；邮箱不存在时上面 updated=false 已跳过
		if u, err := mysql.GetUserByEmail(email); err == nil {
			redis.InvalidateTokenVersionCache(u.UserID)
		}
	}
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
