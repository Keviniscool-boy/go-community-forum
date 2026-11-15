package controller

import (
	"errors"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/logic"
	"bluebell/models"
)

// SendCodeHandler 发送邮箱验证码
// @Summary 发送邮箱验证码
// @Description 向指定邮箱发送验证码，用于注册/登录/找回密码
// @Tags 认证相关接口
// @Accept json
// @Produce json
// @Param object body models.ParamSendCode true "邮箱和用途"
// @Success 200 {object} controller.Response
// @Router /auth/send-code [post]
func SendCodeHandler(c *gin.Context) {
	p := new(models.ParamSendCode)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	// 客户端 IP，用于发送频率限制
	ip := c.ClientIP()
	if err := logic.SendCode(p.Email, p.Purpose, ip); err != nil {
		zap.L().Error("logic.SendCode failed", zap.Error(err))
		switch {
		case errors.Is(err, redis.ErrCodeSendTooFrequent), errors.Is(err, redis.ErrCodeSendIPLimited):
			ResponseError(c, CodeSendTooFrequent)
		default:
			// 其余（含邮件发送失败、Redis 故障）统一为"发送失败"，不区分具体原因
			ResponseError(c, CodeSendCodeFailed)
		}
		return
	}
	ResponseSuccess(c, nil)
}

// RegisterHandler 邮箱验证码注册
// @Summary 邮箱注册
// @Description 使用邮箱验证码注册账号，成功直接返回 token 和用户信息
// @Tags 认证相关接口
// @Accept json
// @Produce json
// @Param object body models.ParamEmailRegister true "注册参数"
// @Success 200 {object} controller.Response{data=controller.AuthResponse}
// @Router /auth/register [post]
func RegisterHandler(c *gin.Context) {
	p := new(models.ParamEmailRegister)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	token, user, err := logic.RegisterByEmail(p)
	if err != nil {
		zap.L().Error("logic.RegisterByEmail failed", zap.Error(err))
		switch {
		case errors.Is(err, logic.ErrRegisterConflict):
			// 邮箱或用户名冲突统一为模糊错误，不暴露具体原因（防枚举）
			ResponseError(c, CodeRegisterFailed)
		case errors.Is(err, redis.ErrVerifyCodeInvalid):
			ResponseError(c, CodeVerifyCodeInvalid)
		case errors.Is(err, redis.ErrVerifyCodeLocked):
			ResponseError(c, CodeVerifyCodeLocked)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, AuthResponse{Token: token, User: user})
}

// LoginHandler 邮箱密码登录
// @Summary 邮箱登录
// @Description 使用邮箱和密码登录，返回 token 和用户信息
// @Tags 认证相关接口
// @Accept json
// @Produce json
// @Param object body models.ParamEmailLogin true "登录参数"
// @Success 200 {object} controller.Response{data=controller.AuthResponse}
// @Router /auth/login [post]
func EmailLoginHandler(c *gin.Context) {
	p := new(models.ParamEmailLogin)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	token, user, err := logic.LoginByEmail(p)
	if err != nil {
		zap.L().Error("logic.LoginByEmail failed", zap.Error(err))
		switch {
		case errors.Is(err, logic.ErrInvalidCredentials):
			ResponseError(c, CodeInvalidCredentials)
		case errors.Is(err, logic.ErrAccountBanned):
			ResponseError(c, CodeAccountBanned)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, AuthResponse{Token: token, User: user})
}

// MeHandler 当前登录用户信息
// @Summary 当前用户信息
// @Description 获取当前登录用户的基本信息
// @Tags 认证相关接口
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} controller.Response{data=models.AuthUser}
// @Router /auth/me [get]
func MeHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	user, err := logic.GetMe(userID)
	if err != nil {
		zap.L().Error("logic.GetMe failed", zap.Error(err))
		if errors.Is(err, mysql.ErrUserNotExist) {
			ResponseError(c, CodeUserNotExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, user)
}

// ResetPasswordHandler 找回密码
// @Summary 找回密码
// @Description 使用 reset_password 验证码重置密码；成功后所有旧会话（旧 JWT）立即失效。
// @Description 统一响应：无论邮箱是否存在，验证码正确即返回成功，防止账号枚举
// @Tags 认证相关接口
// @Accept json
// @Produce json
// @Param object body models.ParamResetPassword true "重置参数"
// @Success 200 {object} controller.Response
// @Router /auth/reset-password [post]
func ResetPasswordHandler(c *gin.Context) {
	p := new(models.ParamResetPassword)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.ResetPassword(p); err != nil {
		zap.L().Error("logic.ResetPassword failed", zap.Error(err))
		switch {
		case errors.Is(err, redis.ErrVerifyCodeInvalid):
			ResponseError(c, CodeVerifyCodeInvalid)
		case errors.Is(err, redis.ErrVerifyCodeLocked):
			ResponseError(c, CodeVerifyCodeLocked)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, nil)
}

// AuthResponse 登录/注册成功响应体
type AuthResponse struct {
	Token string           `json:"token"`
	User  *models.AuthUser `json:"user"`
}
