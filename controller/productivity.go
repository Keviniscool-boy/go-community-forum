package controller

import (
	"encoding/json"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/logic"
	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// ---------- 收藏夹 ----------

// ListBookmarkFoldersHandler 收藏夹列表（首次访问自动建"我的收藏"）
func ListBookmarkFoldersHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	mysql.EnsureDefaultFolder(userID)
	folders, err := mysql.ListFolders(userID)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, folders)
}

type createFolderRequest struct {
	Name string `json:"name" binding:"required,min=1,max=64"`
}

// CreateBookmarkFolderHandler 创建收藏夹
func CreateBookmarkFolderHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	var req createFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	f := &models.BookmarkFolder{
		FolderID: snowflake.GenerateID(),
		UserID:   userID,
		Name:     req.Name,
	}
	if err := mysql.CreateFolder(f); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"folder_id": strconv.FormatInt(f.FolderID, 10)})
}

// DeleteBookmarkFolderHandler 删除收藏夹
func DeleteBookmarkFolderHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	folderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.DeleteFolder(folderID, userID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

type bookmarkItemRequest struct {
	TargetType string `json:"target_type" binding:"required,oneof=question answer"`
	TargetID   string `json:"target_id" binding:"required"`
}

// ListBookmarkItemsHandler 收藏条目（目标已删显示 unavailable）
func ListBookmarkItemsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	folderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	items, err := mysql.ListBookmarkItems(folderID, userID, 100)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	type itemVO struct {
		models.BookmarkItem
		Available bool `json:"available"`
	}
	out := make([]itemVO, 0, len(items))
	for _, it := range items {
		out = append(out, itemVO{BookmarkItem: *it, Available: mysql.TargetExists(it.TargetType, it.TargetID)})
	}
	ResponseSuccess(c, out)
}

// AddBookmarkItemHandler 收藏到收藏夹
func AddBookmarkItemHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	folderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	var req bookmarkItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if _, err := mysql.GetFolder(folderID, userID); err != nil {
		ResponseError(c, CodeForbidden)
		return
	}
	tt := models.ReportTargetTypeToCode(req.TargetType)
	if tt == 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	tid, err := strconv.ParseInt(req.TargetID, 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.AddBookmarkItem(folderID, userID, tt, tid); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// RemoveBookmarkItemHandler 取消收藏
func RemoveBookmarkItemHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	folderID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	tt := models.ReportTargetTypeToCode(c.Param("type"))
	tid, err := strconv.ParseInt(c.Param("target_id"), 10, 64)
	if err != nil || tt == 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.RemoveBookmarkItem(folderID, userID, tt, tid); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// ---------- 浏览历史 ----------

// ListViewHistoryHandler 历史列表
func ListViewHistoryHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	items, err := mysql.ListViewHistory(userID, 100)
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	type itemVO struct {
		models.BookmarkItem
		Available bool `json:"available"`
	}
	out := make([]itemVO, 0, len(items))
	for _, it := range items {
		out = append(out, itemVO{BookmarkItem: *it, Available: mysql.TargetExists(it.TargetType, it.TargetID)})
	}
	ResponseSuccess(c, out)
}

// ClearHistoryHandler 清空历史
func ClearHistoryHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := mysql.ClearHistory(userID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// RecordViewHandler 浏览埋点（登录用户记录历史）
func RecordViewHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	tt := models.ReportTargetTypeToCode(c.Query("type"))
	tid, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil || tt == 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.RecordView(userID, tt, tid); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// ---------- 推荐事件 ----------

type eventRequest struct {
	Events []struct {
		EventType string `json:"event_type" binding:"required"`
		ItemType  string `json:"item_type" binding:"required"`
		ItemID    string `json:"item_id" binding:"required"`
		FeedType  string `json:"feed_type"`
		Position  *int   `json:"position"`
		EventID   string `json:"event_id"` // 客户端去重 ID
	} `json:"events" binding:"required,max=50"`
}

var allowedEventTypes = map[string]bool{
	"feed_impression": true, "feed_click": true, "content_view": true, "content_dwell": true,
	"answer_vote": true, "answer_collect": true, "comment_create": true, "follow_create": true,
	"search_submit": true, "search_result_click": true, "report_create": true,
}

// BatchReportEventsHandler 埋点批量上报
// @Summary 推荐事件批量上报
// @Description 曝光/点击/停留等事件；客户端事件不直接改变分数与权限；同 event_id 幂等
// @Tags 推荐相关接口
// @Accept json
// @Produce json
// @Param object body eventRequest true "事件批次"
// @Success 200 {object} controller.Response
// @Router /events [post]
func BatchReportEventsHandler(c *gin.Context) {
	userID := tryGetUserID(c)
	var req eventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	requestID := c.GetString("request_id")
	events := make([]*models.RecommendationEvent, 0, len(req.Events))
	for _, e := range req.Events {
		if !allowedEventTypes[e.EventType] {
			continue
		}
		itemID, err := strconv.ParseInt(e.ItemID, 10, 64)
		if err != nil {
			continue
		}
		// 事件 ID：客户端提供则透传（幂等键），否则服务端生成
		eventID := snowflake.GenerateID()
		if e.EventID != "" {
			if parsed, err := strconv.ParseInt(e.EventID, 10, 64); err == nil && parsed > 0 {
				eventID = parsed
			}
		}
		metadata, _ := json.Marshal(map[string]string{"client_time": ""})
		events = append(events, &models.RecommendationEvent{
			EventID:     eventID,
			UserID:      userID,
			EventType:   e.EventType,
			ItemType:    e.ItemType,
			ItemID:      itemID,
			FeedType:    e.FeedType,
			Position:    e.Position,
			RequestID:   requestID,
			MetadataStr: string(metadata),
		})
	}
	if len(events) == 0 {
		ResponseSuccess(c, nil)
		return
	}
	if err := mysql.CreateRecommendationEvents(events); err != nil {
		zap.L().Error("create recommendation events failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// RecommendationVersionHandler 当前推荐规则版本
func RecommendationVersionHandler(c *gin.Context) {
	ResponseSuccess(c, gin.H{"recommendation_version": mysql.RecommendationVersion()})
}

var _ = logic.RenderMarkdownSafe
