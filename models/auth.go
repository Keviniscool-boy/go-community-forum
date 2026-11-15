package models

// 邮箱认证相关请求/响应模型

// 验证码用途
const (
	CodePurposeRegister      = "register"
	CodePurposeLogin         = "login"
	CodePurposeResetPassword = "reset_password"
)

// ParamSendCode 发送邮箱验证码请求
type ParamSendCode struct {
	Email   string `json:"email" binding:"required,email"`
	Purpose string `json:"purpose" binding:"required,oneof=register login reset_password"`
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

// ParamResetPassword 找回密码请求（验证码 purpose=reset_password）
type ParamResetPassword struct {
	Email      string `json:"email" binding:"required,email"`
	Code       string `json:"code" binding:"required,len=6"`
	Password   string `json:"password" binding:"required,min=8,max=64"`
	RePassword string `json:"re_password" binding:"required,eqfield=Password"`
}

// AuthUser 登录/注册成功后返回的用户信息
type AuthUser struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
}
