package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/logic"
	"bluebell/models"
)

// EditQuestionHandler 编辑问题
// @Summary 编辑问题
// @Description 仅作者可编辑问题标题与补充说明
// @Tags 问答相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Param object body models.ParamUpdateQuestion true "更新参数"
// @Success 200 {object} controller.Response
// @Router /questions/{id} [put]
func EditQuestionHandler(c *gin.Context) {
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || questionID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamUpdateQuestion)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.EditQuestion(userID, questionID, p); err != nil {
		zap.L().Error("logic.EditQuestion failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// DeleteQuestionHandler 删除问题（软删除）
// @Summary 删除问题
// @Description 仅作者可删除，软删除后 Feed 与详情不再展示
// @Tags 问答相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Success 200 {object} controller.Response
// @Router /questions/{id} [delete]
func DeleteQuestionHandler(c *gin.Context) {
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || questionID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.DeleteQuestion(userID, questionID); err != nil {
		zap.L().Error("logic.DeleteQuestion failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// EditAnswerHandler 编辑回答
// @Summary 编辑回答
// @Description 仅作者可编辑回答内容
// @Tags 问答相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param answer_id path int true "回答 ID"
// @Param object body models.ParamUpdateAnswer true "更新参数"
// @Success 200 {object} controller.Response
// @Router /answers/{answer_id} [put]
func EditAnswerHandler(c *gin.Context) {
	answerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || answerID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamUpdateAnswer)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.EditAnswer(userID, answerID, p.Content); err != nil {
		zap.L().Error("logic.EditAnswer failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// DeleteAnswerHandler 删除回答（软删除，回答数重算）
// @Summary 删除回答
// @Description 仅作者可删除，软删除后问题的回答数自动重算
// @Tags 问答相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param answer_id path int true "回答 ID"
// @Success 200 {object} controller.Response
// @Router /answers/{answer_id} [delete]
func DeleteAnswerHandler(c *gin.Context) {
	answerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || answerID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.DeleteAnswer(userID, answerID); err != nil {
		zap.L().Error("logic.DeleteAnswer failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// UpdateProfileHandler 更新个人资料
// @Summary 更新个人资料
// @Description 更新当前登录用户的简介与头像地址
// @Tags 认证相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body models.ParamUpdateProfile true "资料参数"
// @Success 200 {object} controller.Response
// @Router /auth/profile [put]
func UpdateProfileHandler(c *gin.Context) {
	p := new(models.ParamUpdateProfile)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.UpdateProfile(userID, p); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}
