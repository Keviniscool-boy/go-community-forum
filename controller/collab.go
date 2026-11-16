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

// ---------- 系列 ----------

// CreateSeriesHandler 创建系列
func CreateSeriesHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	columnID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || columnID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamCreateSeries)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	seriesID, err := logic.CreateSeries(userID, columnID, p)
	if err != nil {
		zap.L().Error("logic.CreateSeries failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		if errors.Is(err, logic.ErrModerationUnavailable) {
			ResponseErrorWithMsg(c, CodeServerBusy, err.Error())
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"series_id": strconv.FormatInt(seriesID, 10)})
}

// GetSeriesHandler 系列详情与目录
func GetSeriesHandler(c *gin.Context) {
	seriesID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || seriesID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	s, items, err := logic.GetSeriesDetail(seriesID, tryGetUserID(c))
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	ResponseSuccess(c, gin.H{"series": s, "items": items})
}

// UpdateSeriesHandler 更新系列
func UpdateSeriesHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	seriesID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || seriesID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamUpdateSeries)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.UpdateSeriesMeta(userID, seriesID, p); err != nil {
		zap.L().Error("logic.UpdateSeriesMeta failed", zap.Error(err))
		if errors.Is(err, mysql.ErrSeriesOrderConflict) || errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

type seriesOrderRequest struct {
	ArticleIDs []string `json:"article_ids" binding:"required,max=200"`
	Version    int64    `json:"version" binding:"required,min=1"`
}

// SetSeriesOrderHandler 目录排序
func SetSeriesOrderHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	seriesID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || seriesID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	var req seriesOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	p := &models.ParamSeriesOrder{ArticleIDs: req.ArticleIDs, Version: req.Version}
	if err := logic.SetSeriesOrder(userID, seriesID, p); err != nil {
		zap.L().Error("logic.SetSeriesOrder failed", zap.Error(err))
		if errors.Is(err, mysql.ErrSeriesOrderConflict) {
			ResponseError(c, CodeSeriesOrderConflict)
			return
		}
		if errors.Is(err, logic.ErrForbidden) || errors.Is(err, logic.ErrArticleInvalid) {
			ResponseError(c, CodeInvalidParams)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// ListSeriesForColumnHandler 专栏系列列表
func ListSeriesForColumnHandler(c *gin.Context) {
	columnID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || columnID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	data, err := logic.ListSeriesForColumn(columnID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// ---------- 成员与邀请 ----------

// InviteMemberHandler 邀请成员
func InviteMemberHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	columnID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || columnID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamInviteMember)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	dup, err := logic.InviteMember(userID, columnID, p, c.GetString("request_id"))
	if err != nil {
		zap.L().Error("logic.InviteMember failed", zap.Error(err))
		switch {
		case errors.Is(err, logic.ErrForbidden):
			ResponseError(c, CodeForbidden)
		case errors.Is(err, logic.ErrCannotInviteSelf), errors.Is(err, logic.ErrCannotContact):
			ResponseError(c, CodeInvalidParams)
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

// AcceptInvitationHandler 接受邀请
func AcceptInvitationHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	invitationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || invitationID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := logic.AcceptInvitation(userID, invitationID, c.GetString("request_id")); err != nil {
		zap.L().Error("logic.AcceptInvitation failed", zap.Error(err))
		switch {
		case errors.Is(err, logic.ErrInvitationExpired), errors.Is(err, logic.ErrCannotContact):
			ResponseErrorWithMsg(c, CodeServerBusy, err.Error())
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, nil)
}

// DeclineInvitationHandler 拒绝邀请
func DeclineInvitationHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	invitationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || invitationID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := logic.DeclineInvitation(userID, invitationID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// RemoveMemberHandler 移除成员
func RemoveMemberHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	columnID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || columnID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	targetUserID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || targetUserID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := logic.RemoveMember(userID, columnID, targetUserID, c.GetString("request_id")); err != nil {
		zap.L().Error("logic.RemoveMember failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) || errors.Is(err, logic.ErrOwnerOnly) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// ListColumnMembersHandler 成员列表
func ListColumnMembersHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	columnID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || columnID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	members, err := logic.ListMembers(columnID, userID)
	if err != nil {
		if errors.Is(err, logic.ErrNotColumnMember) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, members)
}

// ---------- 版本与审阅 ----------

// ListArticleRevisionsHandler 版本列表
func ListArticleRevisionsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	articleID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || articleID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	revisions, err := logic.ListRevisions(userID, articleID)
	if err != nil {
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, revisions)
}

type restoreRequest struct {
	RevisionID string `json:"revision_id" binding:"required"`
}

// RestoreRevisionHandler 恢复历史版本为新草稿版本
func RestoreRevisionHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	articleID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || articleID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	var req restoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	revID, err := strconv.ParseInt(req.RevisionID, 10, 64)
	if err != nil || revID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	rev, err := logic.RestoreRevision(userID, articleID, revID, 0)
	if err != nil {
		zap.L().Error("logic.RestoreRevision failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"revision_id": strconv.FormatInt(rev.RevisionID, 10), "revision_no": rev.RevisionNo})
}

// SubmitReviewHandler 提交审阅
func SubmitReviewHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	articleID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || articleID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	var req struct {
		RevisionID string `json:"revision_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	reviewID, err := logic.SubmitReview(userID, articleID, req.RevisionID)
	if err != nil {
		zap.L().Error("logic.SubmitReview failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) || errors.Is(err, logic.ErrArticleInvalid) {
			ResponseError(c, CodeInvalidParams)
			return
		}
		if errors.Is(err, mysql.ErrReviewPending) {
			ResponseError(c, CodeReviewPending)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"review_id": strconv.FormatInt(reviewID, 10)})
}

// ListReviewCommentsHandler 审阅评论列表
func ListReviewCommentsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	reviewID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || reviewID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	comments, err := logic.ListReviewCommentsFor(userID, reviewID)
	if err != nil {
		if errors.Is(err, logic.ErrNotColumnMember) || errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, comments)
}

// AddReviewCommentHandler 审阅评论
func AddReviewCommentHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	reviewID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || reviewID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamReviewComment)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.AddReviewComment(userID, reviewID, p); err != nil {
		zap.L().Error("logic.AddReviewComment failed", zap.Error(err))
		if errors.Is(err, logic.ErrNotColumnMember) || errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// DecideReviewHandler 审阅决定
func DecideReviewHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	reviewID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || reviewID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	p := new(models.ParamReviewDecision)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.DecideReview(userID, reviewID, p); err != nil {
		zap.L().Error("logic.DecideReview failed", zap.Error(err))
		switch {
		case errors.Is(err, logic.ErrSelfApproval):
			ResponseError(c, CodeForbidden)
		case errors.Is(err, logic.ErrStateNotAllowed), errors.Is(err, logic.ErrArticleInvalid):
			ResponseError(c, CodeInvalidParams)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, nil)
}
