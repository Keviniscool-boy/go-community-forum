package logic

import (
	"errors"
	"strconv"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/models"
	"bluebell/pkg/jwt"
	"bluebell/pkg/mail"
	"bluebell/pkg/snowflake"
	"go.uber.org/zap"
)

// 邮箱认证业务错误
var (
	ErrEmailExist     = errors.New("邮箱已被注册")
	ErrEmailNotFound  = errors.New("该邮箱尚未注册")
	ErrAccountBanned  = errors.New("账号已被禁用")
)

// SendCode 发送邮箱验证码
func SendCode(email, purpose, ip string) error {
	// 注册场景先检查邮箱是否已被占用，避免用户等到验证码才发现无法注册
	if purpose == models.CodePurposeRegister {
		exist, err := mysql.CheckEmailExist(email)
		if err != nil {
			return err
		}
		if exist {
			return ErrEmailExist
		}
	}
	// 登录场景检查邮箱是否已注册
	if purpose == models.CodePurposeLogin {
		exist, err := mysql.CheckEmailExist(email)
		if err != nil {
			return err
		}
		if !exist {
			return ErrEmailNotFound
		}
	}
	code, err := redis.SendAuthCode(email, purpose, ip)
	if err != nil {
		return err
	}
	// 发送邮件失败时验证码仍在 Redis 中，用户可等待倒计时结束后重发
	return mail.SendCodeMail(email, code)
}

// RegisterByEmail 邮箱验证码注册，成功后返回 token 和用户信息
func RegisterByEmail(p *models.ParamEmailRegister) (token string, user *models.AuthUser, err error) {
	// 校验验证码（成功后立即删除，一次性使用）
	if err = redis.VerifyAuthCode(p.Email, models.CodePurposeRegister, p.Code); err != nil {
		return "", nil, err
	}
	// 邮箱唯一性检查
	exist, err := mysql.CheckEmailExist(p.Email)
	if err != nil {
		return "", nil, err
	}
	if exist {
		return "", nil, ErrEmailExist
	}
	// 用户名唯一性检查
	usernameExist, err := mysql.CheckUsernameExist(p.Username)
	if err != nil {
		return "", nil, err
	}
	if usernameExist {
		return "", nil, ErrUserExist
	}
	// 生成 UID 并入库（密码 bcrypt 哈希在 dao 层处理）
	newUser := &models.User{
		UserID:   snowflake.GenerateID(),
		Username: p.Username,
		Email:    p.Email,
		Password: p.Password,
	}
	if err = mysql.CreateEmailUser(newUser); err != nil {
		if errors.Is(err, mysql.ErrUserExist) {
			return "", nil, ErrEmailExist
		}
		return "", nil, err
	}
	// 注册成功直接签发 token
	token, err = jwt.GenToken(newUser.UserID, newUser.Username)
	if err != nil {
		return "", nil, err
	}
	return token, toAuthUser(newUser), nil
}

// LoginByEmail 邮箱密码登录，成功后返回 token 和用户信息
func LoginByEmail(p *models.ParamEmailLogin) (token string, user *models.AuthUser, err error) {
	loginUser := &models.User{
		Email:    p.Email,
		Password: p.Password,
	}
	if err = mysql.LoginByEmail(loginUser); err != nil {
		if errors.Is(err, mysql.ErrInvalidCredentials) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, err
	}
	// 禁用账号不允许登录
	if loginUser.Status == 0 {
		return "", nil, ErrAccountBanned
	}
	// 更新最后登录时间（失败不影响登录）
	if err := mysql.UpdateLastLogin(loginUser.UserID); err != nil {
		zap.L().Warn("update last_login_at failed", zap.Error(err))
	}
	token, err = jwt.GenToken(loginUser.UserID, loginUser.Username)
	if err != nil {
		return "", nil, err
	}
	return token, toAuthUser(loginUser), nil
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
	}
}

func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
