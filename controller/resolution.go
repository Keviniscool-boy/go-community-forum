package controller

// 问答解决闭环接口（产品审计 PRD §4.4）：
// 采纳/撤销采纳、问题关闭/重开、我的内容管理列表。

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/logic"
	"bluebell/models"
)

// respondResolutionError 解决闭环统一错误映射（稳定错误码，追加不重排）
func respondResolutionError(c *gin.Context, err error) {
	zap.L().Error("resolution op failed", zap.Error(err))
	switch {
	case errors.Is(err, logic.ErrForbidden):
		ResponseError(c, CodeForbidden)
	case errors.Is(err, logic.ErrQuestionNotExist):
		ResponseError(c, CodeQuestionNotExist)
	case errors.Is(err, logic.ErrQuestionStateConflict):
		ResponseError(c, CodeQuestionStateConflict)
	case errors.Is(err, logic.ErrAnswerCannotAccept):
		ResponseError(c, CodeAnswerNotAcceptable)
	case errors.Is(err, logic.ErrQuestionClosedOp):
		ResponseError(c, CodeQuestionClosedState)
	case errors.Is(err, logic.ErrQuestionTransition):
		ResponseError(c, CodeQuestionTransition)
	case errors.Is(err, logic.ErrCursorInvalid):
		ResponseError(c, CodeInvalidParams)
	default:
		ResponseError(c, CodeServerBusy)
	}
}

// AcceptAnswerHandler 采纳回答
// @Summary 采纳回答
// @Description 问题作者采纳一条回答为解决答案（同问题唯一；幂等；solved 状态原子维护）
// @Tags 问答相关接口
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Param answer_id path int true "回答 ID"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /questions/{id}/answers/{answer_id}/accept [post]
func AcceptAnswerHandler(c *gin.Context) {
	questionID, answerID, ok := parseQuestionAnswerParams(c)
	if !ok {
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.AcceptAnswer(userID, questionID, answerID, c.GetString("request_id")); err != nil {
		respondResolutionError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{
		"question_id":        strconv.FormatInt(questionID, 10),
		"accepted_answer_id": strconv.FormatInt(answerID, 10),
		"status":             "solved",
	})
}

// UnacceptAnswerHandler 撤销采纳
// @Summary 撤销采纳回答
// @Description 问题作者撤销当前采纳，问题恢复 open（幂等）
// @Tags 问答相关接口
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Param answer_id path int true "回答 ID"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /questions/{id}/answers/{answer_id}/accept [delete]
func UnacceptAnswerHandler(c *gin.Context) {
	questionID, answerID, ok := parseQuestionAnswerParams(c)
	if !ok {
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.UnacceptAnswer(userID, questionID, answerID, c.GetString("request_id")); err != nil {
		respondResolutionError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{
		"question_id":        strconv.FormatInt(questionID, 10),
		"accepted_answer_id": "",
		"status":             "open",
	})
}

// SetQuestionStatusHandler 问题状态变更（open/closed 转换；solved 由采纳维护）
// @Summary 问题关闭/重开
// @Description 问题作者（或有权限管理员）在 open/closed 间转换；closed 不接受新回答
// @Tags 问答相关接口
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Param object body models.ParamSetQuestionStatus true "目标状态"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /questions/{id}/status [patch]
func SetQuestionStatusHandler(c *gin.Context) {
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || questionID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamSetQuestionStatus)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.SetQuestionStatus(userID, questionID, p.Status, c.GetString("request_id")); err != nil {
		respondResolutionError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{
		"question_id": strconv.FormatInt(questionID, 10),
		"status":      p.Status,
	})
}

// ListMyContentHandler 我的内容管理列表
// @Summary 我的内容管理
// @Description 当前用户按类型与状态筛选自己的问题/回答/文章（含审核与删除状态）
// @Tags 用户相关接口
// @Security ApiKeyAuth
// @Param type query string true "内容类型 question|answer|article"
// @Param status query string false "状态 published|pending|hidden|deleted，空为全部"
// @Param cursor query string false "分页游标"
// @Param limit query int false "每页数量（默认 20 最大 50）"
// @Success 200 {object} controller.Response{data=models.PageResult[*models.ApiMyContentItem]}
// @Router /me/content [get]
func ListMyContentHandler(c *gin.Context) {
	p := new(models.ParamMyContent)
	if err := c.ShouldBindQuery(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	result, err := logic.ListMyContent(userID, p)
	if err != nil {
		respondResolutionError(c, err)
		return
	}
	ResponseSuccess(c, result)
}

// parseQuestionAnswerParams 解析路径中的问题/回答 ID
func parseQuestionAnswerParams(c *gin.Context) (questionID, answerID int64, ok bool) {
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || questionID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return 0, 0, false
	}
	answerID, err = strconv.ParseInt(c.Param("answer_id"), 10, 64)
	if err != nil || answerID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return 0, 0, false
	}
	return questionID, answerID, true
}
