package models

// 邮箱认证相关请求/响应模型

// 验证码用途
const (
	CodePurposeRegister      = "register"
	CodePurposeLogin         = "login"
	CodePurposeResetPassword = "reset_password"
	CodePurposeBindEmail     = "bind_email"
)

// 重置凭据有效期（秒）
const ResetTokenTTLSeconds = 600

// ParamSendCode 发送邮箱验证码请求
type ParamSendCode struct {
	Email   string `json:"email" binding:"required,email"`
	Purpose string `json:"purpose" binding:"required,oneof=register login reset_password bind_email"`
}

// ParamEmailRegister 邮箱注册请求
type ParamEmailRegister struct {
	Email      string `json:"email" binding:"required,email"`
	Code       string `json:"code" binding:"required,len=6"`
	Username   string `json:"username" binding:"required,min=2,max=32"`
	Password   string `json:"password" binding:"required,min=8,max=64"`
	RePassword string `json:"re_password" binding:"required,eqfield=Password"`
}

// ParamEmailLogin 邮箱密码登录请求
type ParamEmailLogin struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// ParamVerifyResetCode 找回密码第一步：验证码换取一次性重置凭据
type ParamVerifyResetCode struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required,len=6"`
}

// ParamResetWithToken 找回密码第二步：凭据设置新密码
type ParamResetWithToken struct {
	ResetToken string `json:"reset_token" binding:"required"`
	Password   string `json:"password" binding:"required,min=8,max=64"`
	RePassword string `json:"re_password" binding:"required,eqfield=Password"`
}

// ParamBindEmail 旧账号绑定已验证邮箱（需登录）
type ParamBindEmail struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required,len=6"`
}

// AuthUser 登录/注册成功后返回的用户信息
type AuthUser struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Bio       string `json:"bio"`
	Role      string `json:"role"`
}
