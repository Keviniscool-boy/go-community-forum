package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/logic"
)

// viewerRole 当前用户的角色（可选登录场景）：
// 角色中间件已注入时直接复用，否则查库（角色每次查库即时生效）；查询失败按普通用户处理。
func viewerRole(c *gin.Context, userID int64) string {
	if userID <= 0 {
		return ""
	}
	if v, ok := c.Get("role"); ok {
		if role, ok := v.(string); ok {
			return role
		}
	}
	role, err := mysql.UserRole(userID)
	if err != nil {
		zap.L().Warn("get user role failed", zap.Int64("uid", userID), zap.Error(err))
		return ""
	}
	return role
}

// ListCommentRepliesHandler 楼中楼回复列表
// @Summary 楼中楼回复列表
// @Description 按 cursor 分页查询某条一级评论下的回复（最早在前）；父评论已删除时仍返回其下回复
// @Tags 评论相关接口
// @Produce json
// @Param id path int true "一级评论 ID"
// @Param cursor query string false "游标"
// @Param limit query int false "数量，默认 20，最大 50"
// @Success 200 {object} controller.Response{data=models.CommentDetailPage}
// @Router /comments/{id}/replies [get]
func ListCommentRepliesHandler(c *gin.Context) {
	viewerID := tryGetUserID(c)
	data, err := logic.ListCommentReplies(viewerID, viewerRole(c, viewerID), c.Param("id"), c.Query("cursor"), atoiDefault(c.Query("limit"), 20))
	if err != nil {
		zap.L().Error("logic.ListCommentReplies failed", zap.Error(err))
		switch {
		case errors.Is(err, logic.ErrCommentNotExist):
			ResponseError(c, CodeCommentNotExist)
		case errors.Is(err, logic.ErrCommentInvalid), errors.Is(err, logic.ErrCursorInvalid):
			ResponseError(c, CodeInvalidParams)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, data)
}

// LikeCommentHandler 点赞评论
// @Summary 点赞评论
// @Description 给评论点赞，需登录，幂等（重复点赞不重复计数）
// @Tags 评论相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "评论 ID"
// @Success 200 {object} controller.Response
// @Router /comments/{id}/like [post]
func LikeCommentHandler(c *gin.Context) {
	commentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || commentID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.LikeComment(userID, commentID); err != nil {
		zap.L().Error("logic.LikeComment failed", zap.Error(err))
		if errors.Is(err, logic.ErrCommentNotExist) {
			ResponseError(c, CodeCommentNotExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// UnlikeCommentHandler 取消点赞评论
// @Summary 取消点赞评论
// @Tags 评论相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "评论 ID"
// @Success 200 {object} controller.Response
// @Router /comments/{id}/like [delete]
func UnlikeCommentHandler(c *gin.Context) {
	commentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || commentID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.UnlikeComment(userID, commentID); err != nil {
		zap.L().Error("logic.UnlikeComment failed", zap.Error(err))
		if errors.Is(err, logic.ErrCommentNotExist) {
			ResponseError(c, CodeCommentNotExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// DeleteCommentHandler 删除评论
// @Summary 删除评论
// @Description 软删除评论：仅作者本人或管理员/审核员；其下回复保留，所属内容评论数同步回算
// @Tags 评论相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "评论 ID"
// @Success 200 {object} controller.Response
// @Router /comments/{id} [delete]
func DeleteCommentHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.DeleteComment(userID, viewerRole(c, userID), c.Param("id")); err != nil {
		zap.L().Error("logic.DeleteComment failed", zap.Error(err))
		switch {
		case errors.Is(err, logic.ErrForbidden):
			ResponseError(c, CodeForbidden)
		case errors.Is(err, logic.ErrCommentNotExist):
			ResponseError(c, CodeCommentNotExist)
		case errors.Is(err, logic.ErrCommentInvalid):
			ResponseError(c, CodeInvalidParams)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, nil)
}
