package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/logic"
	"bluebell/models"
)

// resolveRequest 审核处理请求体
type resolveRequest struct {
	Decision string `json:"decision" binding:"required,oneof=resolve dismiss"`
	Note     string `json:"note" binding:"max=1000"`
}

// ListReportsHandler 审核队列
// @Summary 举报队列
// @Description 管理员/审核员查看举报队列，cursor 分页
// @Tags 管理后台
// @Produce json
// @Security ApiKeyAuth
// @Param status query string false "pending | resolved | dismissed，不传查全部"
// @Param cursor query string false "游标"
// @Param limit query int false "数量，默认 50，最大 100"
// @Success 200 {object} controller.Response{data=models.ReportPage}
// @Router /admin/reports [get]
func ListReportsHandler(c *gin.Context) {
	var statusPtr *int32
	switch c.Query("status") {
	case "pending":
		s := int32(0)
		statusPtr = &s
	case "resolved":
		s := int32(1)
		statusPtr = &s
	case "dismissed":
		s := int32(2)
		statusPtr = &s
	}
	data, err := logic.ListReportQueue(statusPtr, c.Query("cursor"), atoiDefault(c.Query("limit"), 50))
	if err != nil {
		zap.L().Error("logic.ListReportQueue failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// ResolveReportHandler 处理举报
// @Summary 处理举报
// @Description resolve 表示举报成立（内容隔离下架），dismiss 表示驳回；乐观锁防并发覆盖
// @Tags 管理后台
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "举报 ID"
// @Param object body resolveRequest true "处理决定"
// @Success 200 {object} controller.Response
// @Router /admin/reports/{id}/resolve [post]
func ResolveReportHandler(c *gin.Context) {
	actorID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	reportID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || reportID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	var req resolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.ResolveReport(actorID, reportID, req.Decision, req.Note, c.GetString("request_id")); err != nil {
		zap.L().Error("logic.ResolveReport failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// ModerateContentHandler 内容审核
// @Summary 内容审核
// @Description 对问题/回答执行 approve / reject / quarantine
// @Tags 管理后台
// @Produce json
// @Security ApiKeyAuth
// @Param type path string true "question | answer"
// @Param id path int true "内容 ID"
// @Param action path string true "approve | reject | quarantine"
// @Success 200 {object} controller.Response
// @Router /admin/content/{type}/{id}/{action} [post]
func ModerateContentHandler(c *gin.Context) {
	actorID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.ModerateContent(actorID, c.Param("type"), c.Param("action"), c.Param("id"), c.GetString("request_id")); err != nil {
		zap.L().Error("logic.ModerateContent failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// AdminSetUserStatusHandler 用户禁用/启用
// @Summary 禁用/启用用户
// @Description 禁用后该用户全部会话立即失效
// @Tags 管理后台
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "用户 ID"
// @Param object body setEnableRequest true "enable: true 启用 / false 禁用"
// @Success 200 {object} controller.Response
// @Router /admin/users/{id}/disable [post]
func AdminSetUserStatusHandler(c *gin.Context) {
	actorID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	var req setEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if req.Enable != nil {
		if err := logic.SetUserEnabled(actorID, targetID, *req.Enable, c.GetString("request_id")); err != nil {
			zap.L().Error("logic.SetUserEnabled failed", zap.Error(err))
			ResponseError(c, CodeServerBusy)
			return
		}
	}
	ResponseSuccess(c, nil)
}

type setEnableRequest struct {
	Enable *bool `json:"enable" binding:"required"`
}

// ListAuditLogsHandler 审计日志
// @Summary 审计日志
// @Description 只追加的管理操作审计记录，cursor 分页
// @Tags 管理后台
// @Produce json
// @Security ApiKeyAuth
// @Param cursor query string false "游标"
// @Param limit query int false "数量，默认 50，最大 100"
// @Success 200 {object} controller.Response{data=models.AuditLogPage}
// @Router /admin/audit-logs [get]
func ListAuditLogsHandler(c *gin.Context) {
	data, err := logic.ListAuditLogsPage(c.Query("cursor"), atoiDefault(c.Query("limit"), 50))
	if err != nil {
		zap.L().Error("logic.ListAuditLogsPage failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// AdminMetricsHandler 指标总览
// @Summary 指标总览
// @Description 用户/内容/举报/审计的计数总览
// @Tags 管理后台
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} controller.Response{data=map[string]interface{}}
// @Router /admin/metrics/overview [get]
func AdminMetricsHandler(c *gin.Context) {
	data, err := logic.MetricsOverview()
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// AdminAggregateMetricsHandler 触发内容逐日指标聚合（幂等重算，metric_version 递增）
func AdminAggregateMetricsHandler(c *gin.Context) {
	if _, err := getCurrentUserID(c); err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	days := atoiDefault(c.Query("days"), 30)
	version, err := logic.AggregateMetrics(days)
	if err != nil {
		zap.L().Error("logic.AggregateMetrics failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"metric_version": version})
}

// ListMyReportsHandler 本人举报状态
func ListMyReportsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	cursorID, _ := strconv.ParseInt(c.Query("cursor"), 10, 64)
	limit := atoiDefault(c.Query("limit"), 20)
	statusPtr := (*int32)(nil) // 全部
	reports, err := mysql.ListReports(&mysql.ReportPageParams{Status: statusPtr, CursorID: cursorID, Limit: limit})
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	// 只返回本人提交的举报
	out := make([]*models.ApiReportDetail, 0)
	for _, r := range reports {
		if r.ReporterID == userID {
			out = append(out, &models.ApiReportDetail{Report: r})
		}
	}
	ResponseSuccess(c, out)
}
