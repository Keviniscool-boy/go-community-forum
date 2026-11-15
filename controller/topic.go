package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// ListTopicsHandler 话题列表
// @Summary 话题列表
// @Tags 话题相关接口
// @Produce json
// @Param limit query int false "数量，默认 20，最大 50"
// @Success 200 {object} controller.Response{data=[]models.Topic}
// @Router /topics [get]
func ListTopicsHandler(c *gin.Context) {
	limit := atoiDefault(c.Query("limit"), 20)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	data, err := mysql.ListTopics(limit)
	if err != nil {
		zap.L().Error("mysql.ListTopics failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// GetTopicHandler 话题详情
// @Summary 话题详情
// @Tags 话题相关接口
// @Produce json
// @Param id path int true "话题 ID"
// @Success 200 {object} controller.Response{data=models.Topic}
// @Router /topics/{id} [get]
func GetTopicHandler(c *gin.Context) {
	topicID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || topicID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	topic, err := mysql.GetTopicByID(topicID)
	if err != nil {
		zap.L().Error("mysql.GetTopicByID failed", zap.Error(err))
		if errors.Is(err, mysql.ErrTopicNotExist) {
			ResponseError(c, CodeInvalidParams)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	// 附带话题下的问题（最新 20 条）
	questions, err := mysql.GetQuestionsByTopicID(topicID, 20)
	if err != nil {
		zap.L().Error("mysql.GetQuestionsByTopicID failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"topic": topic, "questions": questions})
}

// CreateTopicHandler 创建话题
// @Summary 创建话题
// @Description 创建新话题，需登录，名称唯一
// @Tags 话题相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body models.ParamCreateTopic true "话题参数"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /topics [post]
func CreateTopicHandler(c *gin.Context) {
	p := new(models.ParamCreateTopic)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if _, err := getCurrentUserID(c); err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	topic := &models.Topic{
		TopicID:     snowflake.GenerateID(),
		Name:        p.Name,
		Description: p.Description,
	}
	if err := mysql.CreateTopic(topic); err != nil {
		zap.L().Error("mysql.CreateTopic failed", zap.Error(err))
		if errors.Is(err, mysql.ErrTopicExist) {
			ResponseError(c, CodeTopicExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"topic_id": strconv.FormatInt(topic.TopicID, 10)})
}

// FollowTopicHandler 关注话题
// @Summary 关注话题
// @Tags 话题相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "话题 ID"
// @Success 200 {object} controller.Response
// @Router /topics/{id}/follow [post]
func FollowTopicHandler(c *gin.Context) {
	topicID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || topicID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := mysql.FollowTopic(userID, topicID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// UnfollowTopicHandler 取消关注话题
// @Summary 取消关注话题
// @Tags 话题相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "话题 ID"
// @Success 200 {object} controller.Response
// @Router /topics/{id}/follow [delete]
func UnfollowTopicHandler(c *gin.Context) {
	topicID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || topicID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := mysql.UnfollowTopic(userID, topicID); err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}
