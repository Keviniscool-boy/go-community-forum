package controller

import (
	"errors"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/redis"
	"bluebell/logic"
	"bluebell/models"
)

// CreateReportHandler 创建举报
// @Summary 创建举报
// @Description 举报问题/回答/评论/用户；同用户同目标幂等；每小时限流
// @Tags 举报相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body models.ParamCreateReport true "举报参数"
// @Success 200 {object} controller.Response
// @Router /reports [post]
func CreateReportHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamCreateReport)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	dup, err := logic.CreateReport(userID, p)
	if err != nil {
		zap.L().Error("logic.CreateReport failed", zap.Error(err))
		switch {
		case errors.Is(err, redis.ErrCodeSendTooFrequent):
			ResponseError(c, CodeSendTooFrequent)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	if dup {
		ResponseSuccess(c, gin.H{"duplicated": true})
		return
	}
	ResponseSuccess(c, nil)
}
