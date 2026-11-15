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

// ---------- 草稿箱 ----------

// CreateDraftHandler 创建草稿
// @Summary 创建草稿
// @Tags 草稿相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body models.ParamSaveDraft true "草稿参数"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /drafts [post]
func CreateDraftHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamSaveDraft)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	d := &models.Draft{
		DraftID:    snowflake.GenerateID(),
		UserID:     userID,
		DraftType:  draftTypeCode(p.DraftType),
		QuestionID: parseIDOrDefault(p.QuestionID),
		Title:      p.Title,
		Content:    p.Content,
		Version:    1,
	}
	if err := mysql.CreateDraft(d); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"draft_id": strconv.FormatInt(d.DraftID, 10), "version": d.Version})
}

// ListDraftsHandler 草稿列表
// @Summary 草稿列表
// @Tags 草稿相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param type query string true "question | answer"
// @Success 200 {object} controller.Response{data=[]models.Draft}
// @Router /drafts [get]
func ListDraftsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	drafts, err := mysql.ListDrafts(userID, draftTypeCode(c.Query("type")))
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, drafts)
}

// GetDraftHandler 草稿详情
// @Summary 草稿详情
// @Tags 草稿相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "草稿 ID"
// @Success 200 {object} controller.Response{data=models.Draft}
// @Router /drafts/{id} [get]
func GetDraftHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	draftID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	d, err := mysql.GetDraft(draftID, userID)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	ResponseSuccess(c, d)
}

type updateDraftRequest struct {
	Title   string `json:"title" binding:"max=200"`
	Content string `json:"content" binding:"required,max=20000"`
	Version int64  `json:"version" binding:"required,min=1"`
}

// UpdateDraftHandler 自动保存
// @Summary 更新草稿（自动保存）
// @Description 带版本号乐观锁；冲突返回 CodeDraftConflict
// @Tags 草稿相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "草稿 ID"
// @Param object body updateDraftRequest true "内容与版本"
// @Success 200 {object} controller.Response{data=map[string]interface{}}
// @Router /drafts/{id} [patch]
func UpdateDraftHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	draftID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	var req updateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	newVersion, err := mysql.UpdateDraft(draftID, userID, req.Version, req.Title, req.Content)
	if err != nil {
		if err == mysql.ErrDraftConflict {
			ResponseError(c, CodeDraftConflict)
			return
		}
		zap.L().Error("mysql.UpdateDraft failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"version": newVersion})
}

// DeleteDraftHandler 删除草稿
// @Summary 删除草稿
// @Tags 草稿相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "草稿 ID"
// @Success 200 {object} controller.Response
// @Router /drafts/{id} [delete]
func DeleteDraftHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	draftID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.DeleteDraft(draftID, userID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// PublishDraftHandler 发布草稿
// @Summary 发布草稿
// @Description 草稿内容发布时重新执行审核流（敏感词检查），question 草稿创建问题、answer 草稿创建回答
// @Tags 草稿相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "草稿 ID"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /drafts/{id}/publish [post]
func PublishDraftHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	draftID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	d, err := mysql.GetDraft(draftID, userID)
	if err != nil {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if d.DraftType == 0 {
		qid, err := logic.CreateQuestion(&models.ParamCreateQuestion{
			Title:       d.Title,
			Description: d.Content,
		}, userID)
		if err != nil {
			ResponseError(c, CodeServerBusy)
			return
		}
		_ = mysql.DeleteDraft(draftID, userID)
		ResponseSuccess(c, gin.H{"question_id": strconv.FormatInt(qid, 10)})
		return
	}
	aid, err := logic.CreateAnswer(d.QuestionID, &models.ParamCreateAnswer{Content: d.Content}, userID)
	if err != nil {
		zap.L().Error("publish answer draft failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	_ = mysql.DeleteDraft(draftID, userID)
	ResponseSuccess(c, gin.H{"answer_id": strconv.FormatInt(aid, 10)})
}

func draftTypeCode(s string) int32 {
	if s == "answer" {
		return 1
	}
	return 0
}

func parseIDOrDefault(s string) int64 {
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v
	}
	return 0
}

// ---------- Markdown 预览 ----------

// PreviewMarkdownHandler Markdown 安全预览
// @Summary Markdown 安全渲染预览
// @Description 安全子集（标题/粗体/斜体/列表/代码/链接/图片），服务端清洗，无原始 HTML
// @Tags 媒体相关接口
// @Accept json
// @Produce json
// @Param object body previewRequest true "markdown 源文本"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /markdown/preview [post]
func PreviewMarkdownHandler(c *gin.Context) {
	var req previewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	ResponseSuccess(c, gin.H{"html": logic.RenderMarkdownSafe(req.Markdown)})
}

type previewRequest struct {
	Markdown string `json:"markdown" binding:"required,max=20000"`
}
