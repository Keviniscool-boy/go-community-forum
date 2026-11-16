package controller

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/logic"
	"bluebell/models"
	"bluebell/pkg/snowflake"
)

func dmError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, logic.ErrCannotContact):
		ResponseErrorWithMsg(c, CodeServerBusy, err.Error())
	case errors.Is(err, logic.ErrDMRateLimited):
		ResponseError(c, CodeSendTooFrequent)
	case errors.Is(err, logic.ErrNotConversationPeer), errors.Is(err, logic.ErrNotRequestReceiver), errors.Is(err, logic.ErrStateNotAllowed):
		// 统一不可见/无权响应，不区分不存在与无权
		ResponseError(c, CodeInvalidParams)
	case errors.Is(err, mysql.ErrPrivacyConflict):
		ResponseError(c, CodePreferenceConflict)
	default:
		zap.L().Error("dm error", zap.Error(err))
		ResponseError(c, CodeServerBusy)
	}
}

// StartConversationHandler 发起会话（首条消息）
func StartConversationHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamStartConversation)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	convID, msg, err := logic.StartConversation(userID, p)
	if err != nil {
		dmError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{"conversation_id": strconv.FormatInt(convID, 10), "message": msg})
}

// ListConversationsHandler 会话列表
func ListConversationsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	convs, err := mysql.ListConversations(userID, atoiDefault(c.Query("limit"), 20))
	if err != nil {
		dmError(c, err)
		return
	}
	ResponseSuccess(c, convs)
}

// AcceptConversationHandler 接受请求
func AcceptConversationHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	convID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || convID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := logic.AcceptConversation(userID, convID); err != nil {
		dmError(c, err)
		return
	}
	ResponseSuccess(c, nil)
}

// DeclineConversationHandler 拒绝请求
func DeclineConversationHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	convID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || convID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := logic.DeclineConversation(userID, convID); err != nil {
		dmError(c, err)
		return
	}
	ResponseSuccess(c, nil)
}

// CloseConversationHandler 关闭会话
func CloseConversationHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	convID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || convID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := logic.CloseConversation(userID, convID); err != nil {
		dmError(c, err)
		return
	}
	ResponseSuccess(c, nil)
}

// HideConversationHandler 隐藏本人会话视图
func HideConversationHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	convID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || convID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.HideConversation(convID, userID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// ListMessagesHandler 消息分页
func ListMessagesHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	convID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || convID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	isMember, err := mysql.IsConversationMember(convID, userID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	if !isMember {
		ResponseError(c, CodeInvalidParams)
		return
	}
	limit := atoiDefault(c.Query("limit"), 20)
	if limit > 50 {
		limit = 50
	}
	beforeID, _ := strconv.ParseInt(c.Query("before"), 10, 64)
	messages, err := mysql.ListMessages(convID, beforeID, limit)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, messages)
}

// SendMessageHandler 会话内发送
func SendMessageHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	convID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || convID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamSendMessage)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	msg, err := logic.SendMessageToConversation(userID, convID, p)
	if err != nil {
		dmError(c, err)
		return
	}
	ResponseSuccess(c, msg)
}

// MarkConversationReadHandler 已读上报
func MarkConversationReadHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	convID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || convID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	var req models.ParamReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	messageID, err := strconv.ParseInt(req.LastReadMessageID, 10, 64)
	if err != nil || messageID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	isMember, err := mysql.IsConversationMember(convID, userID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	if !isMember {
		ResponseError(c, CodeInvalidParams)
		return
	}
	m, err := mysql.GetMessage(messageID)
	if err != nil || m.ConversationID != convID {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.AdvanceReadMarker(convID, userID, messageID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// GetPrivacySettingsHandler 隐私设置
func GetPrivacySettingsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	policy, receipts, version, err := mysql.GetMessagePrivacySetting(userID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"incoming_policy": policy, "read_receipts_enabled": receipts, "version": version})
}

// PutPrivacySettingsHandler 更新隐私设置
func PutPrivacySettingsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	var req models.ParamPrivacySettings
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	newVersion, err := mysql.UpsertPrivacySettings(userID, req.IncomingPolicy, *req.ReadReceiptsEnabled, req.Version)
	if err != nil {
		if err == mysql.ErrPrivacyConflict {
			ResponseError(c, CodePreferenceConflict)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"incoming_policy": req.IncomingPolicy, "read_receipts_enabled": *req.ReadReceiptsEnabled, "version": newVersion})
}

// ReportMessageHandler 举报消息
func ReportMessageHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	messageID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || messageID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	var req struct {
		ReasonCode  string `json:"reason_code" binding:"required,oneof=spam abuse copyright unsafe other"`
		Description string `json:"description" binding:"max=1000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	// 只能举报自己参与会话中收到的消息（非本人发送）
	m, err := mysql.GetMessage(messageID)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	isMember, _ := mysql.IsConversationMember(m.ConversationID, userID)
	if !isMember || m.SenderID == userID {
		ResponseError(c, CodeForbidden)
		return
	}
	r := &models.Report{
		ReportID:    snowflake.GenerateID(),
		ReporterID:  userID,
		TargetType:  5, // message
		TargetID:    messageID,
		ReasonCode:  req.ReasonCode,
		Description: req.Description,
	}
	dup, err := mysql.CreateReport(r)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	// 举报成立前先保存加密证据快照（规格 3.5：证据含消息快照，加密存储）
	retainUntil := time.Now().UTC().Add(365 * 24 * time.Hour)
	if err := mysql.SaveMessageEvidence(r.ReportID, messageID, m.Body, retainUntil); err != nil {
		zap.L().Error("save message evidence failed", zap.Error(err))
	}
	if dup {
		ResponseSuccess(c, gin.H{"duplicated": true})
		return
	}
	ResponseSuccess(c, nil)
}

// DeleteAccountHandler 注销账号（去标识化）
func DeleteAccountHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	var req struct {
		Confirm string `json:"confirm" binding:"required,eq=DELETE"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, "请携带 confirm=\"DELETE\" 确认")
		return
	}
	if err := logic.DeactivateAccount(userID); err != nil {
		zap.L().Error("logic.DeactivateAccount failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	// 清除本地登录态由前端完成；服务端 token 已全部失效
	ResponseSuccess(c, nil)
}
