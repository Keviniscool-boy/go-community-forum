package controller

// 私信接口（第五阶段 PRD F6）

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/logic"
	"bluebell/models"
)

// respondMessageError 私信域统一错误映射（防探测：拉黑与统一不可联系文案一致）
func respondMessageError(c *gin.Context, err error) {
	zap.L().Error("message op failed", zap.Error(err))
	switch {
	case errors.Is(err, logic.ErrMessageSelf):
		ResponseError(c, CodeInvalidParams)
	case errors.Is(err, logic.ErrMessageBlocked):
		ResponseErrorWithMsg(c, CodeForbidden, "无法向该用户发送私信")
	case errors.Is(err, logic.ErrMessageRejected):
		ResponseErrorWithMsg(c, CodeInvalidParams, "消息包含违规内容，发送失败")
	case errors.Is(err, logic.ErrMessageRateLimit):
		ResponseError(c, CodeSendTooFrequent)
	case errors.Is(err, logic.ErrMessageNotPeer):
		ResponseError(c, CodeInvalidParams)
	case errors.Is(err, mysql.ErrUserNotExist):
		ResponseError(c, CodeUserNotExist)
	case errors.Is(err, logic.ErrCursorInvalid):
		ResponseError(c, CodeInvalidParams)
	default:
		ResponseError(c, CodeServerBusy)
	}
}

// ListMessageThreadsHandler 我的会话列表
// @Summary 私信会话列表
// @Description 按最近消息倒序；带对方资料、未读数与最后一条摘要；自己删除的会话不出现
// @Tags 私信
// @Security ApiKeyAuth
// @Param cursor query string false "游标"
// @Param limit query int false "每页数量（默认 20 最大 50）"
// @Success 200 {object} controller.Response{data=models.PageResult[*models.MessageThread]}
// @Router /messages/threads [get]
func ListMessageThreadsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	limit := atoiDefault(c.Query("limit"), 20)
	data, err := logic.ListMyThreads(userID, c.Query("cursor"), limit)
	if err != nil {
		respondMessageError(c, err)
		return
	}
	ResponseSuccess(c, data)
}

// GetMessageThreadHandler 会话内消息
// @Summary 会话内消息
// @Description 最早在前，cursor 向上翻页；打开即更新自己的已读水位
// @Tags 私信
// @Security ApiKeyAuth
// @Param id path int true "会话 ID"
// @Param cursor query string false "游标（取更早消息）"
// @Param limit query int false "每页数量（默认 30 最大 50）"
// @Success 200 {object} controller.Response{data=models.PageResult[*models.ApiMessageDetail]}
// @Router /messages/threads/{id} [get]
func GetMessageThreadHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	limit := atoiDefault(c.Query("limit"), 30)
	data, err := logic.GetThreadMessages(userID, c.Param("id"), c.Query("cursor"), limit)
	if err != nil {
		respondMessageError(c, err)
		return
	}
	ResponseSuccess(c, data)
}

// OpenDirectMessageHandler 开聊
// @Summary 与某用户开聊
// @Description 已存在会话则追加消息并返回 thread_id；拉黑/拒绝词/限流拒绝
// @Tags 私信
// @Security ApiKeyAuth
// @Param id path int true "对方用户 ID"
// @Param object body models.ParamSendMessage true "消息内容"
// @Success 200 {object} controller.Response{data=map[string]object}
// @Router /messages/users/{id} [post]
func OpenDirectMessageHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamSendMessage)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	msg, threadID, err := logic.OpenDirectMessage(userID, c.Param("id"), p)
	if err != nil {
		respondMessageError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{"thread_id": strconv.FormatInt(threadID, 10), "message": msg})
}

// ReplyMessageHandler 会话内发消息
// @Summary 会话内发送消息
// @Tags 私信
// @Security ApiKeyAuth
// @Param id path int true "会话 ID"
// @Param object body models.ParamSendMessage true "消息内容"
// @Success 200 {object} controller.Response{data=map[string]object}
// @Router /messages/threads/{id} [post]
func ReplyMessageHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamSendMessage)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	msg, threadID, err := logic.ReplyInThread(userID, c.Param("id"), p)
	if err != nil {
		respondMessageError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{"thread_id": strconv.FormatInt(threadID, 10), "message": msg})
}

// DeleteThreadHandler 删除会话（仅自己视角）
// @Summary 删除会话
// @Description 仅自己视角隐藏；消息保留
// @Tags 私信
// @Security ApiKeyAuth
// @Param id path int true "会话 ID"
// @Success 200 {object} controller.Response
// @Router /messages/threads/{id} [delete]
func DeleteThreadHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.DeleteMyThread(userID, c.Param("id")); err != nil {
		respondMessageError(c, err)
		return
	}
	ResponseSuccess(c, nil)
}

// MessageUnreadCountHandler 私信未读合计
// @Summary 私信未读数（TopNav 角标）
// @Tags 私信
// @Security ApiKeyAuth
// @Success 200 {object} controller.Response{data=map[string]int64}
// @Router /messages/unread-count [get]
func MessageUnreadCountHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	count, err := logic.CountMyMessageUnread(userID)
	if err != nil {
		respondMessageError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{"count": count})
}
