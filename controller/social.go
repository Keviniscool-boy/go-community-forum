package controller

// 第五阶段 M1 社交关系接口（关注/粉丝列表、拉黑、通知单条已读、评论编辑）

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/logic"
	"bluebell/models"
)

// respondSocialError 社交域统一错误映射（不泄露关系状态，防探测）
func respondSocialError(c *gin.Context, err error) {
	zap.L().Error("social op failed", zap.Error(err))
	switch {
	case errors.Is(err, logic.ErrFollowBlocked):
		// 统一文案，不暴露"被拉黑"（PRD F2）
		ResponseErrorWithMsg(c, CodeForbidden, "对方暂不接受关注")
	case errors.Is(err, logic.ErrForbidden):
		ResponseError(c, CodeForbidden)
	case errors.Is(err, logic.ErrNotificationForbidden):
		ResponseError(c, CodeForbidden)
	case errors.Is(err, logic.ErrCommentEditWindow):
		ResponseErrorWithMsg(c, CodeForbidden, "评论已超出可编辑时间")
	case errors.Is(err, logic.ErrCommentNotOwner):
		ResponseError(c, CodeForbidden)
	case errors.Is(err, logic.ErrCommentNotExist):
		ResponseError(c, CodeInvalidParams)
	case errors.Is(err, mysql.ErrUserNotExist):
		ResponseError(c, CodeUserNotExist)
	case errors.Is(err, logic.ErrCommentInvalid):
		ResponseError(c, CodeInvalidParams)
	default:
		ResponseError(c, CodeServerBusy)
	}
}

// ListUserFollowersHandler 粉丝列表
// @Summary 用户的粉丝列表
// @Description 公开可读；登录用户视角带 is_following；被封禁用户不出现
// @Tags 用户相关接口
// @Produce json
// @Param id path int true "用户 ID"
// @Param cursor query string false "游标（上一页最后一个 follow id）"
// @Param limit query int false "每页数量（默认 20 最大 50）"
// @Success 200 {object} controller.Response{data=models.PageResult[*models.FollowListUser]}
// @Router /users/{id}/followers [get]
func ListUserFollowersHandler(c *gin.Context) {
	listFollowRelationsHandler(c, "followers")
}

// ListUserFollowingHandler 关注列表
// @Summary 用户的关注列表
// @Description 公开可读；登录用户视角带 is_following
// @Tags 用户相关接口
// @Produce json
// @Param id path int true "用户 ID"
// @Param cursor query string false "游标"
// @Param limit query int false "每页数量"
// @Success 200 {object} controller.Response{data=models.PageResult[*models.FollowListUser]}
// @Router /users/{id}/following [get]
func ListUserFollowingHandler(c *gin.Context) {
	listFollowRelationsHandler(c, "following")
}

func listFollowRelationsHandler(c *gin.Context, which string) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	limit := atoiDefault(c.Query("limit"), 20)
	cursorID := int64(0)
	if v, perr := strconv.ParseInt(c.Query("cursor"), 10, 64); perr == nil {
		cursorID = v
	}
	viewerID := tryGetUserID(c)

	var data *models.PageResult[*models.FollowListUser]
	if which == "followers" {
		data, err = logic.ListFollowers(targetID, viewerID, cursorID, int64(limit))
	} else {
		data, err = logic.ListFollowing(targetID, viewerID, cursorID, int64(limit))
	}
	if err != nil {
		respondSocialError(c, err)
		return
	}
	ResponseSuccess(c, data)
}

// ListUserBlocksHandler 我的黑名单
// @Summary 我的黑名单列表
// @Tags 用户相关接口
// @Security ApiKeyAuth
// @Param cursor query string false "游标"
// @Param limit query int false "每页数量"
// @Success 200 {object} controller.Response{data=models.PageResult[*models.BlockedUserItem]}
// @Router /users/blocks [get]
func ListUserBlocksHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	limit := atoiDefault(c.Query("limit"), 20)
	data, err := logic.ListBlocks(userID, c.Query("cursor"), limit)
	if err != nil {
		respondSocialError(c, err)
		return
	}
	ResponseSuccess(c, data)
}

// GetUserInteractionsHandler 用户互动状态（本期新增，PRD F2）
// @Summary 与目标用户的互动状态
// @Description 登录视角返回 is_following / is_blocked，供前端渲染按钮
// @Tags 用户相关接口
// @Security ApiKeyAuth
// @Param id path int true "目标用户 ID"
// @Success 200 {object} controller.Response{data=map[string]bool}
// @Router /users/{id}/interactions [get]
func GetUserInteractionsHandler(c *gin.Context) {
	viewerID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	following, err := logic.IsFollowing(viewerID, targetID)
	if err != nil {
		respondSocialError(c, err)
		return
	}
	blocked, err := mysql.IsBlockedBetween(viewerID, targetID)
	if err != nil {
		respondSocialError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{
		"is_following": following,
		"is_blocked":   blocked,
	})
}

// EditCommentHandler 编辑评论
// @Summary 编辑自己的评论
// @Description 仅作者本人；5 分钟编辑窗口（超时返回禁止）；内容重走发布审核流；已编辑标记由 update_time 派生
// @Tags 互动相关接口
// @Security ApiKeyAuth
// @Param id path int true "评论 ID"
// @Param object body models.ParamEditComment true "新内容"
// @Success 200 {object} controller.Response{data=models.ApiCommentDetail}
// @Router /comments/{id} [patch]
func EditCommentHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamEditComment)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	detail, err := logic.EditComment(userID, c.Param("id"), p.Content)
	if err != nil {
		respondSocialError(c, err)
		return
	}
	ResponseSuccess(c, detail)
}

// GetNotifySettingsHandler 读取通知偏好
// @Summary 读取通知偏好
// @Description 按类型开关（like/comment/reply/collect/dm），缺省全开
// @Tags 用户相关接口
// @Security ApiKeyAuth
// @Success 200 {object} controller.Response{data=map[string]bool}
// @Router /auth/notify-settings [get]
func GetNotifySettingsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	data, err := logic.GetNotifySettings(userID)
	if err != nil {
		respondSocialError(c, err)
		return
	}
	ResponseSuccess(c, data)
}

// SetNotifySettingsHandler 写通知偏好
// @Summary 写通知偏好
// @Description 全量覆盖；键位 like/comment/reply/collect/dm；关闭仅影响新通知的产生
// @Tags 用户相关接口
// @Security ApiKeyAuth
// @Param object body map[string]bool true "偏好"
// @Success 200 {object} controller.Response
// @Router /auth/notify-settings [put]
func SetNotifySettingsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	pref := map[string]bool{}
	if err := c.ShouldBindJSON(&pref); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.SetNotifySettings(userID, pref); err != nil {
		respondSocialError(c, err)
		return
	}
	ResponseSuccess(c, pref)
}

// SetQuestionRedirectHandler 问题重定向（治理）
// @Summary 问题重定向到目标问题
// @Description 仅 admin；可逆（DELETE 清除）；审计记录动作与原因
// @Tags 管理接口
// @Security ApiKeyAuth
// @Param id path int true "源问题 ID（被合并）"
// @Param object body models.ParamQuestionRedirect true "目标与原因"
// @Success 200 {object} controller.Response
// @Router /admin/questions/{id}/redirect [post]
func SetQuestionRedirectHandler(c *gin.Context) {
	operatorID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || questionID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamQuestionRedirect)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	targetID, perr := strconv.ParseInt(p.TargetID, 10, 64)
	if perr != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := logic.SetQuestionRedirect(operatorID, questionID, targetID, p.Note); err != nil {
		respondSocialError(c, err)
		return
	}
	ResponseSuccess(c, nil)
}

// ClearQuestionRedirectHandler 清除问题重定向
// @Summary 清除问题重定向（可逆）
// @Tags 管理接口
// @Security ApiKeyAuth
// @Param id path int true "源问题 ID"
// @Success 200 {object} controller.Response
// @Router /admin/questions/{id}/redirect [delete]
func ClearQuestionRedirectHandler(c *gin.Context) {
	operatorID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || questionID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := logic.ClearQuestionRedirect(operatorID, questionID); err != nil {
		respondSocialError(c, err)
		return
	}
	ResponseSuccess(c, nil)
}
