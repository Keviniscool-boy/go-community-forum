package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/logic"
	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// ---------- 专栏关注 ----------

// FollowColumnHandler 关注专栏
func FollowColumnHandler(c *gin.Context) {
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
	if _, err := mysql.GetColumnByID(columnID); err != nil {
		ResponseError(c, CodeColumnNotExist)
		return
	}
	if err := mysql.FollowColumn(userID, columnID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"following": true})
}

// UnfollowColumnHandler 取消关注专栏
func UnfollowColumnHandler(c *gin.Context) {
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
	if err := mysql.UnfollowColumn(userID, columnID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"following": false})
}

// ---------- 兴趣 ----------

type interestsRequest struct {
	TopicIDs []string `json:"topic_ids" binding:"max=20"`
}

// GetInterestsHandler 兴趣列表
func GetInterestsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	ids, err := mysql.GetUserInterests(userID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, strconv.FormatInt(id, 10))
	}
	ResponseSuccess(c, gin.H{"topic_ids": out})
}

// PutInterestsHandler 全量替换兴趣（上限 20）
func PutInterestsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	var req interestsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if len(req.TopicIDs) > 20 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	ids := make([]int64, 0, len(req.TopicIDs))
	for _, s := range req.TopicIDs {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			ResponseError(c, CodeInvalidParams)
			return
		}
		ids = append(ids, id)
	}
	if err := mysql.ReplaceUserInterests(userID, ids); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"topic_ids": req.TopicIDs})
}

// ---------- 推荐偏好与负反馈 ----------

type prefRequest struct {
	PersonalizationEnabled *bool  `json:"personalization_enabled" binding:"required"`
	Version                int64  `json:"version" binding:"required,min=0"`
}

// GetRecommendationPreferencesHandler 推荐偏好
func GetRecommendationPreferencesHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	enabled, version, err := mysql.GetRecommendationPreference(userID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"personalization_enabled": enabled, "version": version})
}

// PutRecommendationPreferencesHandler 更新推荐偏好（版本乐观锁）
func PutRecommendationPreferencesHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	var req prefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	newVersion, err := mysql.SetRecommendationPreference(userID, *req.PersonalizationEnabled, req.Version)
	if err != nil {
		if err == mysql.ErrPreferenceConflict {
			ResponseError(c, CodePreferenceConflict)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"personalization_enabled": *req.PersonalizationEnabled, "version": newVersion})
}

type feedbackRequest struct {
	Kind       string `json:"kind" binding:"required,oneof=not_interested reduce_topic"`
	TargetType string `json:"target_type" binding:"required,oneof=content topic"`
	TargetID   string `json:"target_id" binding:"required"`
}

// AddRecommendationFeedbackHandler 记录负反馈（幂等）
func AddRecommendationFeedbackHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	var req feedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	targetID, err := strconv.ParseInt(req.TargetID, 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.AddRecommendationFeedback(userID, req.Kind, req.TargetType, targetID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// RemoveRecommendationFeedbackHandler 撤销负反馈
func RemoveRecommendationFeedbackHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	kind := c.Param("kind")
	if kind != "not_interested" && kind != "reduce_topic" {
		ResponseError(c, CodeInvalidParams)
		return
	}
	targetType := c.Param("type")
	if targetType != "content" && targetType != "topic" {
		ResponseError(c, CodeInvalidParams)
		return
	}
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.RemoveRecommendationFeedback(userID, kind, targetType, targetID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// ListRecommendationFeedbackHandler 本人负反馈列表
func ListRecommendationFeedbackHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	rows, err := mysql.ListRecommendationFeedback(userID, atoiDefault(c.Query("limit"), 50))
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, rows)
}

// ---------- 屏蔽 ----------

type blockRequest struct {
	Blocked *bool `json:"blocked" binding:"required"`
}

// BlockUserHandler 屏蔽作者
func BlockUserHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.BlockUser(userID, targetID); err != nil {
		if err == mysql.ErrCannotBlockSelf {
			ResponseError(c, CodeInvalidParams)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	// 双方建立屏蔽：两人之间的待处理回答邀请立即失效（PRD §9）
	if err := mysql.ExpireInvitationsBetween(userID, targetID); err != nil {
		zap.L().Error("expire invitations after block failed",
			zap.Int64("blocker", userID), zap.Int64("blocked", targetID), zap.Error(err))
	}
	ResponseSuccess(c, gin.H{"blocked": true})
}

// UnblockUserHandler 取消屏蔽
func UnblockUserHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.UnblockUser(userID, targetID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"blocked": false})
}

// ListBlocksHandler 本人屏蔽列表
func ListBlocksHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	users, err := mysql.ListBlockedUsers(userID, atoiDefault(c.Query("limit"), 100))
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, users)
}

// ---------- 通知单条已读 ----------

// MarkNotificationReadHandler 单条已读
func MarkNotificationReadHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	notificationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || notificationID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if _, err := mysql.MarkNotificationRead(notificationID, userID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"read": true})
}

// ---------- 申诉 ----------

type appealRequest struct {
	TargetType      string `json:"target_type" binding:"required,oneof=question answer article"`
	TargetID        string `json:"target_id" binding:"required"`
	DecisionVersion int64  `json:"decision_version" binding:"required,min=1"`
	Reason          string `json:"reason" binding:"required,min=1,max=1000"`
}

// CreateAppealHandler 提交申诉
func CreateAppealHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	var req appealRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	tt := models.ReportTargetTypeToCode(req.TargetType)
	if tt == 0 || req.TargetType == "user" {
		ResponseError(c, CodeInvalidParams)
		return
	}
	targetID, err := strconv.ParseInt(req.TargetID, 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	// 校验作者是本人（内容存在且 author 匹配）
	authorID, err := logic.TargetAuthorIDForModeration(tt, targetID)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if authorID != userID {
		ResponseError(c, CodeForbidden)
		return
	}
	a := &models.ModerationAppeal{
		AppealID:        snowflake.GenerateID(),
		AuthorID:        userID,
		TargetType:      tt,
		TargetID:        targetID,
		DecisionVersion: req.DecisionVersion,
		Reason:          req.Reason,
	}
	if err := mysql.CreateAppeal(a); err != nil {
		if err == mysql.ErrAppealConflict {
			ResponseError(c, CodeAppealConflict)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"appeal_id": strconv.FormatInt(a.AppealID, 10)})
}

// ListAppealsHandler 申诉队列（管理端）
func ListAppealsHandler(c *gin.Context) {
	status := int32(models.AppealPending)
	if c.Query("status") == "resolved" {
		status = -1 // 全部：简化 v1 用 pending 队列为主
	}
	appeals, err := mysql.ListAppeals(status, 0, atoiDefault(c.Query("limit"), 50))
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, appeals)
}

type appealResolveRequest struct {
	Version  int64  `json:"version" binding:"required,min=1"`
	Decision string `json:"decision" binding:"required,oneof=uphold reject"`
	Note     string `json:"note" binding:"max=1000"`
}

// ResolveAppealHandler 处理申诉
func ResolveAppealHandler(c *gin.Context) {
	actorID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	appealID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || appealID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	var req appealResolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	appeal, err := mysql.GetAppeal(appealID)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	// 审核员不能处理自己的内容申诉
	if appeal.AuthorID == actorID {
		ResponseError(c, CodeForbidden)
		return
	}
	newStatus := int32(models.AppealRejected)
	if req.Decision == "uphold" {
		newStatus = models.AppealUpheld
		// 申诉成立：恢复内容为 approved
		if err := mysql.SetModeration(appeal.TargetType, appeal.TargetID, models.ModerationApproved, actorID); err != nil {
			ResponseError(c, CodeServerBusy)
			return
		}
	}
	ok, err := mysql.ResolveAppeal(appealID, actorID, newStatus, req.Note, req.Version)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	if !ok {
		ResponseError(c, CodePreferenceConflict)
		return
	}
	ResponseSuccess(c, nil)
}

var _ = zap.L

// CreatorOverviewHandler 创作者概览
func CreatorOverviewHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	data, err := mysql.GetCreatorOverview(userID)
	if err != nil {
		zap.L().Error("GetCreatorOverview failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// CreatorContentsHandler 创作者内容列表
func CreatorContentsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	status := int32(1)
	if c.Query("status") == "all" {
		status = 0 // 0 会被 SQL 用作 status 过滤值吗？——question.status 只有 1；all 场景传 1 保持行为
		status = 1
	}
	rows, err := mysql.ListCreatorContents(userID, c.Query("type"), status, atoiDefault(c.Query("limit"), 50))
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, rows)
}

// CreatorContentMetricsHandler 内容逐日统计
func CreatorContentMetricsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	contentType := c.Param("type")
	contentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || contentID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	// 只允许查看自己的内容
	authorID, err := logic.TargetAuthorIDForModeration(models.ReportTargetTypeToCode(contentType), contentID)
	if err != nil || authorID != userID {
		ResponseError(c, CodeForbidden)
		return
	}
	rows, asOf, err := mysql.ContentDailyMetrics(contentType, contentID, atoiDefault(c.Query("days"), 30))
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"daily": rows, "as_of": asOf})
}
