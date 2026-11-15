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
		case errors.Is(err, logic.ErrEmailExist):
			ResponseError(c, CodeEmailExist)
		case errors.Is(err, logic.ErrEmailNotFound):
			ResponseError(c, CodeEmailNotExist)
		case errors.Is(err, redis.ErrCodeSendTooFrequent), errors.Is(err, redis.ErrCodeSendIPLimited):
			ResponseError(c, CodeSendTooFrequent)
		default:
			ResponseError(c, CodeServerBusy)
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
		case errors.Is(err, logic.ErrEmailExist):
			ResponseError(c, CodeEmailExist)
		case errors.Is(err, logic.ErrUserExist):
			ResponseError(c, CodeUserExist)
		case errors.Is(err, redis.ErrVerifyCodeInvalid):
			ResponseError(c, CodeVerifyCodeInvalid)
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

// AuthResponse 登录/注册成功响应体
type AuthResponse struct {
	Token string            `json:"token"`
	User  *models.AuthUser  `json:"user"`
}
