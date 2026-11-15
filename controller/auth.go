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

// VerifyResetCodeHandler 找回密码第一步
// @Summary 找回密码：验证码换重置凭据
// @Description 校验 reset_password 验证码，返回一次性、短时效的 reset_token；
// @Description 凭据明文仅本次响应可见，服务端只存摘要
// @Tags 认证相关接口
// @Accept json
// @Produce json
// @Param object body models.ParamVerifyResetCode true "邮箱与验证码"
// @Success 200 {object} controller.Response{data=map[string]interface{}}
// @Router /auth/password/reset/verify [post]
func VerifyResetCodeHandler(c *gin.Context) {
	p := new(models.ParamVerifyResetCode)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	token, expiresIn, err := logic.VerifyResetCode(p)
	if err != nil {
		zap.L().Error("logic.VerifyResetCode failed", zap.Error(err))
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
	ResponseSuccess(c, gin.H{"reset_token": token, "expires_in": expiresIn})
}

// ResetWithTokenHandler 找回密码第二步
// @Summary 找回密码：凭据设置新密码
// @Description 凭据单次消费（重复使用失败），改密与撤销所有旧会话同事务完成
// @Tags 认证相关接口
// @Accept json
// @Produce json
// @Param object body models.ParamResetWithToken true "重置凭据与新密码"
// @Success 200 {object} controller.Response
// @Router /auth/password/reset [post]
func ResetWithTokenHandler(c *gin.Context) {
	p := new(models.ParamResetWithToken)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.ResetPasswordWithToken(p); err != nil {
		zap.L().Error("logic.ResetPasswordWithToken failed", zap.Error(err))
		switch {
		case errors.Is(err, mysql.ErrResetTokenInvalid):
			ResponseError(c, CodeResetTokenInvalid)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, nil)
}

// BindEmailHandler 绑定已验证邮箱
// @Summary 绑定邮箱
// @Description 登录用户使用 bind_email 验证码绑定真实邮箱；占位邮箱的旧账号通过此流程完成迁移。
// @Description 成功后撤销所有旧会话，需重新登录；邮箱被他人占用返回明确错误
// @Tags 认证相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body models.ParamBindEmail true "邮箱与验证码"
// @Success 200 {object} controller.Response
// @Router /auth/email/bind [post]
func BindEmailHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamBindEmail)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.BindEmail(userID, p); err != nil {
		zap.L().Error("logic.BindEmail failed", zap.Error(err))
		switch {
		case errors.Is(err, redis.ErrVerifyCodeInvalid):
			ResponseError(c, CodeVerifyCodeInvalid)
		case errors.Is(err, redis.ErrVerifyCodeLocked):
			ResponseError(c, CodeVerifyCodeLocked)
		case errors.Is(err, mysql.ErrEmailTaken):
			ResponseError(c, CodeEmailConflict)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, nil)
}

// LogoutAllHandler 全部设备登出
// @Summary 退出所有设备
// @Description 递增 token_version，撤销当前用户所有已签发 token
// @Tags 认证相关接口
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} controller.Response
// @Router /auth/logout-all [post]
func LogoutAllHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.LogoutAll(userID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// AuthResponse 登录/注册成功响应体
type AuthResponse struct {
	Token string           `json:"token"`
	User  *models.AuthUser `json:"user"`
}

// BindSendCodeHandler 发送邮箱绑定验证码（需登录，purpose 固定 bind_email）
// @Summary 发送绑定邮箱验证码
// @Description 仅登录用户可发送；绑定流程专用验证码
// @Tags 认证相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body models.ParamSendCode true "邮箱与用途（bind_email）"
// @Success 200 {object} controller.Response
// @Router /auth/send-code/bind [post]
func BindSendCodeHandler(c *gin.Context) {
	if _, err := getCurrentUserID(c); err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamSendCode)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if p.Purpose != models.CodePurposeBindEmail {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := logic.SendCode(p.Email, models.CodePurposeBindEmail, c.ClientIP()); err != nil {
		zap.L().Error("logic.SendCode(bind) failed", zap.Error(err))
		switch {
		case errors.Is(err, redis.ErrCodeSendTooFrequent), errors.Is(err, redis.ErrCodeSendIPLimited):
			ResponseError(c, CodeSendTooFrequent)
		default:
			ResponseError(c, CodeSendCodeFailed)
		}
		return
	}
	ResponseSuccess(c, nil)
}
