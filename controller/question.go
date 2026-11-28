package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/logic"
	"bluebell/models"
)

// CreateQuestionHandler 创建问题
// @Summary 创建问题
// @Description 发布新问题，需登录
// @Tags 问答相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body models.ParamCreateQuestion true "问题参数"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /questions [post]
func CreateQuestionHandler(c *gin.Context) {
	p := new(models.ParamCreateQuestion)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	questionID, err := logic.CreateQuestion(p, userID)
	if err != nil {
		zap.L().Error("logic.CreateQuestion failed", zap.Error(err))
		if errors.Is(err, logic.ErrQuestionInvalid) {
			ResponseError(c, CodeInvalidParams)
			return
		}
		if errors.Is(err, logic.ErrModerationUnavailable) {
			// fail-closed：审核组件故障时拒绝发布并明确提示，不放出未审核内容
			ResponseErrorWithMsg(c, CodeServerBusy, err.Error())
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"question_id": strconv.FormatInt(questionID, 10)})
}

// GetQuestionDetailHandler 问题详情
// @Summary 问题详情
// @Description 根据问题 ID 查询问题详情（含作者名、话题）
// @Tags 问答相关接口
// @Produce json
// @Param id path int true "问题 ID"
// @Success 200 {object} controller.Response{data=models.ApiQuestionDetail}
// @Router /questions/{id} [get]
func GetQuestionDetailHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	data, err := logic.GetQuestionDetail(id, tryGetUserID(c))
	if err != nil {
		zap.L().Error("logic.GetQuestionDetail failed", zap.Error(err))
		if errors.Is(err, logic.ErrQuestionNotExist) {
			ResponseError(c, CodeQuestionNotExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// ListAnswersHandler 问题下的回答列表
// @Summary 回答列表
// @Description 按 cursor 分页查询问题下的回答，支持 hot / latest 排序
// @Tags 问答相关接口
// @Produce json
// @Param id path int true "问题 ID"
// @Param cursor query string false "分页游标（首页不传）"
// @Param limit query int false "每页数量，默认 20，最大 50"
// @Param order query string false "排序：latest 最新（默认）| hot 热门"
// @Success 200 {object} controller.Response{data=models.AnswerDetailPage}
// @Router /questions/{id}/answers [get]
func ListAnswersHandler(c *gin.Context) {
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	data, err := logic.ListAnswers(questionID, c.Query("order"), c.Query("cursor"), limit)
	if err != nil {
		zap.L().Error("logic.ListAnswers failed", zap.Error(err))
		if errors.Is(err, logic.ErrCursorInvalid) {
			ResponseError(c, CodeInvalidParams)
			return
		}
		if errors.Is(err, logic.ErrQuestionNotExist) {
			ResponseError(c, CodeQuestionNotExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// CreateAnswerHandler 创建回答
// @Summary 创建回答
// @Description 在指定问题下发布回答，需登录
// @Tags 问答相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Param object body models.ParamCreateAnswer true "回答参数"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /questions/{id}/answers [post]
func CreateAnswerHandler(c *gin.Context) {
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamCreateAnswer)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	answerID, err := logic.CreateAnswer(questionID, p, userID)
	if err != nil {
		zap.L().Error("logic.CreateAnswer failed", zap.Error(err))
		if errors.Is(err, logic.ErrQuestionNotExist) {
			ResponseError(c, CodeQuestionNotExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"answer_id": strconv.FormatInt(answerID, 10)})
}
