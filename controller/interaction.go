package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/logic"
	"bluebell/models"
)

// 可选登录：未登录时不报错，返回 userID=0
func tryGetUserID(c *gin.Context) int64 {
	uid, err := getCurrentUserID(c)
	if err != nil {
		return 0
	}
	return uid
}

// FollowUserHandler 关注用户
// @Summary 关注用户
// @Description 关注指定用户，需登录，幂等
// @Tags 互动相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "目标用户 ID"
// @Success 200 {object} controller.Response
// @Router /users/{id}/follow [post]
func FollowUserHandler(c *gin.Context) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.FollowUser(userID, targetID); err != nil {
		zap.L().Error("logic.FollowUser failed", zap.Error(err))
		if errors.Is(err, mysql.ErrCannotFollowSelf) {
			ResponseError(c, CodeInvalidParams)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// UnfollowUserHandler 取消关注用户
// @Summary 取消关注
// @Tags 互动相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "目标用户 ID"
// @Success 200 {object} controller.Response
// @Router /users/{id}/follow [delete]
func UnfollowUserHandler(c *gin.Context) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.UnfollowUser(userID, targetID); err != nil {
		zap.L().Error("logic.UnfollowUser failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// VoteAnswerHandler 回答投票
// @Summary 回答投票
// @Description 对回答投票：direction=1 赞同，-1 反对，0 取消；幂等
// @Tags 互动相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "回答 ID"
// @Param object body models.ParamVoteAnswer true "投票方向"
// @Success 200 {object} controller.Response
// @Router /answers/{id}/vote [post]
func VoteAnswerHandler(c *gin.Context) {
	answerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || answerID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamVoteAnswer)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.VoteAnswer(userID, answerID, p.Direction); err != nil {
		zap.L().Error("logic.VoteAnswer failed", zap.Error(err))
		if errors.Is(err, logic.ErrAnswerNotExist) {
			ResponseError(c, CodeAnswerNotExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// CollectAnswerHandler 收藏回答
// @Summary 收藏回答
// @Tags 互动相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "回答 ID"
// @Success 200 {object} controller.Response
// @Router /answers/{id}/collect [post]
func CollectAnswerHandler(c *gin.Context) {
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
	if err := logic.CollectAnswer(userID, answerID); err != nil {
		zap.L().Error("logic.CollectAnswer failed", zap.Error(err))
		if errors.Is(err, logic.ErrAnswerNotExist) {
			ResponseError(c, CodeAnswerNotExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// UncollectAnswerHandler 取消收藏
// @Summary 取消收藏回答
// @Tags 互动相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "回答 ID"
// @Success 200 {object} controller.Response
// @Router /answers/{id}/collect [delete]
func UncollectAnswerHandler(c *gin.Context) {
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
	if err := logic.UncollectAnswer(userID, answerID); err != nil {
		zap.L().Error("logic.UncollectAnswer failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// FollowQuestionHandler 关注问题
// @Summary 关注问题
// @Tags 互动相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Success 200 {object} controller.Response
// @Router /questions/{id}/follow [post]
func FollowQuestionHandler(c *gin.Context) {
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
	if err := logic.FollowQuestion(userID, questionID); err != nil {
		zap.L().Error("logic.FollowQuestion failed", zap.Error(err))
		if errors.Is(err, logic.ErrQuestionNotExist) {
			ResponseError(c, CodeQuestionNotExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// UnfollowQuestionHandler 取消关注问题
// @Summary 取消关注问题
// @Tags 互动相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Success 200 {object} controller.Response
// @Router /questions/{id}/follow [delete]
func UnfollowQuestionHandler(c *gin.Context) {
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
	if err := logic.UnfollowQuestion(userID, questionID); err != nil {
		zap.L().Error("logic.UnfollowQuestion failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// GetQuestionInteractions 问题互动状态（当前用户是否已关注）
// @Summary 问题互动状态
// @Tags 互动相关接口
// @Produce json
// @Param id path int true "问题 ID"
// @Success 200 {object} controller.Response{data=map[string]bool}
// @Router /questions/{id}/interactions [get]
func GetQuestionInteractionsHandler(c *gin.Context) {
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || questionID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID := tryGetUserID(c)
	followed, err := logic.IsQuestionFollowed(userID, questionID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"followed": followed})
}

// GetAnswerInteractions 回答互动状态（当前用户投票方向、是否收藏）
// @Summary 回答互动状态
// @Tags 互动相关接口
// @Produce json
// @Param id path int true "回答 ID"
// @Success 200 {object} controller.Response{data=map[string]interface{}}
// @Router /answers/{id}/interactions [get]
func GetAnswerInteractionsHandler(c *gin.Context) {
	answerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || answerID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID := tryGetUserID(c)
	vote, err := logic.GetUserVoteDirection(userID, answerID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	collected, err := logic.IsCollected(userID, answerID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"vote": vote, "collected": collected})
}

// CreateCommentHandler 创建评论
// @Summary 创建评论
// @Description 对回答或问题发表评论，需登录
// @Tags 评论相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body models.ParamCreateComment true "评论参数"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /comments [post]
func CreateCommentHandler(c *gin.Context) {
	p := new(models.ParamCreateComment)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	commentID, err := logic.CreateComment(p, userID)
	if err != nil {
		zap.L().Error("logic.CreateComment failed", zap.Error(err))
		switch {
		case errors.Is(err, logic.ErrAnswerNotExist):
			ResponseError(c, CodeAnswerNotExist)
		case errors.Is(err, logic.ErrQuestionNotExist):
			ResponseError(c, CodeQuestionNotExist)
		case errors.Is(err, logic.ErrCommentInvalid):
			ResponseError(c, CodeInvalidParams)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, gin.H{"comment_id": strconv.FormatInt(commentID, 10)})
}

// ListCommentsHandler 评论列表
// @Summary 评论列表
// @Description 按 cursor 分页查询目标的评论
// @Tags 评论相关接口
// @Produce json
// @Param target_type query string true "目标类型：answer | question"
// @Param target_id query string true "目标 ID"
// @Param cursor query string false "游标"
// @Param limit query int false "数量，默认 20，最大 50"
// @Success 200 {object} controller.Response{data=models.PageResult}
// @Router /comments [get]
func ListCommentsHandler(c *gin.Context) {
	data, err := logic.ListComments(c.Query("target_type"), c.Query("target_id"), c.Query("cursor"), atoiDefault(c.Query("limit"), 20))
	if err != nil {
		zap.L().Error("logic.ListComments failed", zap.Error(err))
		if errors.Is(err, logic.ErrCommentInvalid) || errors.Is(err, logic.ErrCursorInvalid) {
			ResponseError(c, CodeInvalidParams)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// atoiDefault 字符串转 int，失败返回默认值
func atoiDefault(s string, d int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return d
	}
	return n
}

// ListNotificationsHandler 通知列表
// @Summary 通知列表
// @Tags 通知相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param cursor query string false "游标"
// @Param limit query int false "数量，默认 20，最大 50"
// @Success 200 {object} controller.Response{data=models.PageResult}
// @Router /notifications [get]
func ListNotificationsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	data, err := logic.ListNotifications(userID, c.Query("cursor"), atoiDefault(c.Query("limit"), 20))
	if err != nil {
		zap.L().Error("logic.ListNotifications failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// UnreadCountHandler 未读通知数
// @Summary 未读通知数
// @Tags 通知相关接口
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} controller.Response{data=map[string]int}
// @Router /notifications/unread-count [get]
func UnreadCountHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	n, err := logic.CountUnreadNotifications(userID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"count": n})
}

// MarkAllNotificationsReadHandler 全部标记已读
// @Summary 全部通知标记已读
// @Tags 通知相关接口
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} controller.Response
// @Router /notifications/read-all [post]
func MarkAllNotificationsReadHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.MarkAllNotificationsRead(userID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}
