package controller

import (
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/pkg/search"
)

// searchIndex 全局搜索实现（main.go 初始化，默认 MySQL fallback）
var searchIndex search.SearchIndex

// InitSearchIndex 注入搜索实现（生产可替换 ES 适配器）
func InitSearchIndex(idx search.SearchIndex) {
	searchIndex = idx
}

// validateSearchKeyword 搜索词校验：1-100 字符
func validateSearchKeyword(q string) (string, bool) {
	q = strings.TrimSpace(q)
	if len(q) == 0 || len([]rune(q)) > 100 {
		return "", false
	}
	return q, true
}

// SearchHandler 统一搜索
// @Summary 统一搜索
// @Description 搜索问题、回答、用户、话题；只返回公开可见内容；q 1-100 字符
// @Tags 搜索相关接口
// @Produce json
// @Param q query string true "关键词（1-100 字符）"
// @Param scope query string false "all（默认）| question | answer | user | topic"
// @Param sort query string false "relevance（默认）| latest"
// @Param cursor query string false "游标"
// @Param limit query int false "数量，默认 20，最大 50"
// @Success 200 {object} controller.Response{data=search.Result}
// @Router /search [get]
func SearchHandler(c *gin.Context) {
	if searchIndex == nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	keyword, ok := validateSearchKeyword(c.Query("q"))
	if !ok {
		ResponseError(c, CodeInvalidParams)
		return
	}
	limit := atoiDefault(c.Query("limit"), 20)
	if limit > 50 {
		limit = 50
	}
	res, err := searchIndex.Search(c.Request.Context(), search.Query{
		Keyword:  keyword,
		Scope:    c.DefaultQuery("scope", "all"),
		Sort:     c.DefaultQuery("sort", "relevance"),
		Cursor:   c.Query("cursor"),
		Limit:    limit,
		ViewerID: tryGetUserID(c),
	})
	if err != nil {
		zap.L().Error("search failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, res)
}

// SearchSuggestionsHandler 建议词
// @Summary 搜索建议
// @Description 话题名与用户名前缀建议
// @Tags 搜索相关接口
// @Produce json
// @Param q query string true "前缀"
// @Success 200 {object} controller.Response{data=[]string}
// @Router /search/suggestions [get]
func SearchSuggestionsHandler(c *gin.Context) {
	if searchIndex == nil {
		ResponseSuccess(c, []string{})
		return
	}
	keyword, ok := validateSearchKeyword(c.Query("q"))
	if !ok {
		ResponseSuccess(c, []string{})
		return
	}
	list, err := searchIndex.Suggest(c.Request.Context(), keyword, 10)
	if err != nil {
		ResponseSuccess(c, []string{})
		return
	}
	ResponseSuccess(c, list)
}
