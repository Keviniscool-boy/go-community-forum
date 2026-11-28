package controller

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/logic"
	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// logAuditSensitive 记录词表变更审计
func logAuditSensitive(requestID string, actorID int64, action, word string) {
	a := &models.AuditLog{
		AuditID:    snowflake.GenerateID(),
		ActorID:    actorID,
		Action:     action,
		TargetType: "sensitive_word",
		TargetID:   word,
		RequestID:  requestID,
		Metadata:   "{}",
	}
	if err := mysql.CreateAuditLog(a); err != nil {
		zap.L().Error("create audit log failed", zap.Error(err))
	}
}

// ListSensitiveWordsHandler 词表列表
// @Summary 敏感词列表
// @Tags 管理后台
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} controller.Response
// @Router /admin/sensitive-words [get]
func ListSensitiveWordsHandler(c *gin.Context) {
	actorID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	_ = actorID
	rows, ver, err := logic.ListSensitiveWords()
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"words": rows, "version": ver})
}

type sensitiveWordRequest struct {
	Word string `json:"word" binding:"required,min=1,max=128"`
	Type int32  `json:"word_type" binding:"omitempty,oneof=0 1"`
}

// AddSensitiveWordHandler 新增敏感词
// @Summary 新增敏感词
// @Description 词表更新写审计日志并触发缓存版本失效；type: 0 普通词（命中隔离）1 拒绝词（命中进人工队列）
// @Tags 管理后台
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body sensitiveWordRequest true "词与类型"
// @Success 200 {object} controller.Response
// @Router /admin/sensitive-words [post]
func AddSensitiveWordHandler(c *gin.Context) {
	actorID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	var req sensitiveWordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	wordType := req.Type
	if err := mysql.AddSensitiveWord(req.Word, wordType, actorID); err != nil {
		zap.L().Error("mysql.AddSensitiveWord failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	logic.InvalidateSensitiveWords()
	logAuditSensitive(c.GetString("request_id"), actorID, "sensitive_word_add", req.Word)
	ResponseSuccess(c, nil)
}

// RemoveSensitiveWordHandler 删除敏感词
// @Summary 删除敏感词
// @Tags 管理后台
// @Produce json
// @Security ApiKeyAuth
// @Param word query string true "敏感词"
// @Success 200 {object} controller.Response
// @Router /admin/sensitive-words [delete]
func RemoveSensitiveWordHandler(c *gin.Context) {
	actorID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	word := c.Query("word")
	if word == "" {
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err := mysql.RemoveSensitiveWord(word); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	logic.InvalidateSensitiveWords()
	logAuditSensitive(c.GetString("request_id"), actorID, "sensitive_word_remove", word)
	ResponseSuccess(c, nil)
}
