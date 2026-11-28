package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/dao/mysql"
	"bluebell/logic"
)

// GetFeedHandler Feed 流
// @Summary Feed 流
// @Description 推荐流（/feed/recommend）、最新流（/feed/latest）、热榜（/feed/hot）为公开接口；
// @Description 关注流（/feed/following）需登录；全部使用 cursor 分页
// @Tags Feed 相关接口
// @Produce json
// @Param cursor query string false "分页游标（首页不传）"
// @Param limit query int false "数量，默认 20，最大 50"
// @Success 200 {object} controller.Response{data=models.FeedItemPage}
// @Router /feed/recommend [get]
func GetFeedHandler(c *gin.Context) {
	feedType := c.Param("type")
	cursor := c.Query("cursor")
	limit := atoiDefault(c.Query("limit"), 20)

	var (
		data interface{}
		err  error
	)
	switch feedType {
	case "recommend":
		data, err = logic.GetRecommendFeed(tryGetUserID(c), cursor, limit)
	case "latest":
		data, err = logic.GetLatestFeed(tryGetUserID(c), cursor, limit)
	case "hot":
		data, err = logic.GetHotFeed(tryGetUserID(c), cursor, limit)
	case "following":
		userID, e := getCurrentUserID(c)
		if e != nil {
			ResponseError(c, CodeNeedLogin)
			return
		}
		data, err = logic.GetFollowingFeed(userID, cursor, limit)
	default:
		ResponseError(c, CodeInvalidParams)
		return
	}
	if err != nil {
		zap.L().Error("get feed failed", zap.String("type", feedType), zap.Error(err))
		switch {
		case errors.Is(err, logic.ErrCursorInvalid):
			ResponseError(c, CodeInvalidParams)
		case errors.Is(err, logic.ErrFeedRefreshing):
			// 并发冷启动或缓存重建中：明确可重试，不伪装成空列表
			ResponseError(c, CodeFeedRefreshing)
		case errors.Is(err, logic.ErrFeedSnapshotExpired):
			// 缓存快照已刷新，旧游标失效：客户端重刷首页
			ResponseError(c, CodeFeedExpired)
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, data)
}

// GetUserProfileHandler 用户主页
// @Summary 用户主页
// @Description 用户基本信息、内容统计与当前用户的关注关系
// @Tags 用户相关接口
// @Produce json
// @Param id path int true "用户 ID"
// @Success 200 {object} controller.Response{data=logic.ApiUserProfile}
// @Router /users/{id} [get]
func GetUserProfileHandler(c *gin.Context) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	data, err := logic.GetUserProfile(targetID, tryGetUserID(c))
	if err != nil {
		zap.L().Error("logic.GetUserProfile failed", zap.Error(err))
		if errors.Is(err, mysql.ErrUserNotExist) {
			ResponseError(c, CodeUserNotExist)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// ListUserQuestionsHandler 用户提问列表
// @Summary 用户提问列表
// @Tags 用户相关接口
// @Produce json
// @Param id path int true "用户 ID"
// @Success 200 {object} controller.Response{data=[]models.Question}
// @Router /users/{id}/questions [get]
func ListUserQuestionsHandler(c *gin.Context) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	data, err := logic.ListUserQuestions(targetID, atoiDefault(c.Query("limit"), 20))
	if err != nil {
		zap.L().Error("logic.ListUserQuestions failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// ListUserAnswersHandler 用户回答列表
// @Summary 用户回答列表
// @Tags 用户相关接口
// @Produce json
// @Param id path int true "用户 ID"
// @Success 200 {object} controller.Response{data=[]models.ApiAnswerDetail}
// @Router /users/{id}/answers [get]
func ListUserAnswersHandler(c *gin.Context) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	data, err := logic.ListUserAnswers(targetID, atoiDefault(c.Query("limit"), 20))
	if err != nil {
		zap.L().Error("logic.ListUserAnswers failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}
// ListUserArticlesHandler 用户公开文章列表（用户主页作品聚合）
func ListUserArticlesHandler(c *gin.Context) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	data, err := logic.ListUserArticles(targetID, atoiDefault(c.Query("limit"), 20))
	if err != nil {
		zap.L().Error("logic.ListUserArticles failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// ListUserColumnsHandler 用户公开专栏列表（用户主页作品聚合）
func ListUserColumnsHandler(c *gin.Context) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	data, err := logic.ListUserColumns(targetID, atoiDefault(c.Query("limit"), 20))
	if err != nil {
		zap.L().Error("logic.ListUserColumns failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

