package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/logic"
	"bluebell/models"
)

// CreateArticleHandler 创建文章
// @Summary 创建文章
// @Description 发布文章（Markdown 安全子集），发布走 fail-closed 审核流；column_id 可选（允许无专栏）
// @Tags 文章相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body models.ParamCreateArticle true "文章参数"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /articles [post]
func CreateArticleHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamCreateArticle)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	articleID, err := logic.CreateArticle(p, userID)
	if err != nil {
		zap.L().Error("logic.CreateArticle failed", zap.Error(err))
		switch {
		case errors.Is(err, logic.ErrArticleInvalid), errors.Is(err, logic.ErrColumnInvalid):
			ResponseError(c, CodeInvalidParams)
		case errors.Is(err, logic.ErrColumnOwnership):
			ResponseError(c, CodeForbidden)
		case errors.Is(err, logic.ErrModerationUnavailable):
			ResponseErrorWithMsg(c, CodeServerBusy, err.Error())
		default:
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, gin.H{"article_id": strconv.FormatInt(articleID, 10)})
}

// GetArticleDetailHandler 文章详情
// @Summary 文章详情
// @Description 公开可见（author_only 仅作者本人）；游客可访问公开文章
// @Tags 文章相关接口
// @Produce json
// @Param id path int true "文章 ID"
// @Success 200 {object} controller.Response{data=models.ApiArticleDetail}
// @Router /articles/{id} [get]
func GetArticleDetailHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	data, err := logic.GetArticleDetail(id, tryGetUserID(c))
	if err != nil {
		zap.L().Error("logic.GetArticleDetail failed", zap.Error(err))
		ResponseError(c, CodeArticleNotExist)
		return
	}
	ResponseSuccess(c, data)
}

// ListArticlesHandler 公开文章列表
// @Summary 文章列表
// @Description cursor 分页，仅公开可见文章
// @Tags 文章相关接口
// @Produce json
// @Param cursor query string false "游标"
// @Param limit query int false "数量，默认 20，最大 50"
// @Success 200 {object} controller.Response{data=models.ArticlePage}
// @Router /articles [get]
func ListArticlesHandler(c *gin.Context) {
	data, err := logic.ListArticlesPage(c.Query("cursor"), atoiDefault(c.Query("limit"), 20))
	if err != nil {
		zap.L().Error("logic.ListArticlesPage failed", zap.Error(err))
		if errors.Is(err, logic.ErrCursorInvalid) {
			ResponseError(c, CodeInvalidParams)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// EditArticleHandler 编辑文章
// @Summary 编辑文章
// @Description 仅作者可编辑
// @Tags 文章相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "文章 ID"
// @Param object body models.ParamUpdateArticle true "更新参数"
// @Success 200 {object} controller.Response
// @Router /articles/{id} [patch]
func EditArticleHandler(c *gin.Context) {
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
	p := new(models.ParamUpdateArticle)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.EditArticle(userID, articleID, p); err != nil {
		zap.L().Error("logic.EditArticle failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// DeleteArticleHandler 删除文章（软删除）
// @Summary 删除文章
// @Tags 文章相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "文章 ID"
// @Success 200 {object} controller.Response
// @Router /articles/{id} [delete]
func DeleteArticleHandler(c *gin.Context) {
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
	if err := logic.DeleteArticle(userID, articleID); err != nil {
		zap.L().Error("logic.DeleteArticle failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// ---------- 专栏 ----------

// CreateColumnHandler 创建专栏
// @Summary 创建专栏
// @Tags 专栏相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param object body models.ParamCreateColumn true "专栏参数"
// @Success 200 {object} controller.Response{data=map[string]string}
// @Router /columns [post]
func CreateColumnHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamCreateColumn)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	columnID, err := logic.CreateColumn(p, userID)
	if err != nil {
		zap.L().Error("logic.CreateColumn failed", zap.Error(err))
		if errors.Is(err, logic.ErrModerationUnavailable) {
			ResponseErrorWithMsg(c, CodeServerBusy, err.Error())
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, gin.H{"column_id": strconv.FormatInt(columnID, 10)})
}

// GetColumnDetailHandler 专栏详情
// @Summary 专栏详情
// @Tags 专栏相关接口
// @Produce json
// @Param id path int true "专栏 ID"
// @Success 200 {object} controller.Response{data=models.ApiColumnDetail}
// @Router /columns/{id} [get]
func GetColumnDetailHandler(c *gin.Context) {
	columnID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || columnID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	data, err := logic.GetColumnDetail(columnID)
	if err != nil {
		zap.L().Error("logic.GetColumnDetail failed", zap.Error(err))
		ResponseError(c, CodeColumnNotExist)
		return
	}
	ResponseSuccess(c, data)
}

// ListColumnsHandler 专栏列表
// @Summary 专栏列表
// @Tags 专栏相关接口
// @Produce json
// @Param limit query int false "数量，默认 20"
// @Success 200 {object} controller.Response{data=[]models.ApiColumnDetail}
// @Router /columns [get]
func ListColumnsHandler(c *gin.Context) {
	data, err := logic.ListColumnsPage(atoiDefault(c.Query("limit"), 20))
	if err != nil {
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, data)
}

// ListColumnArticlesHandler 专栏文章列表
// @Summary 专栏文章列表
// @Tags 专栏相关接口
// @Produce json
// @Param id path int true "专栏 ID"
// @Success 200 {object} controller.Response{data=[]models.ApiArticleDetail}
// @Router /columns/{id}/articles [get]
func ListColumnArticlesHandler(c *gin.Context) {
	columnID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || columnID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	data, err := logic.ListColumnArticles(columnID, tryGetUserID(c), atoiDefault(c.Query("limit"), 20))
	if err != nil {
		zap.L().Error("logic.ListColumnArticles failed", zap.Error(err))
		ResponseError(c, CodeColumnNotExist)
		return
	}
	ResponseSuccess(c, data)
}

// EditColumnHandler 编辑专栏
// @Summary 编辑专栏
// @Description 仅所有者
// @Tags 专栏相关接口
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "专栏 ID"
// @Param object body models.ParamUpdateColumn true "更新参数"
// @Success 200 {object} controller.Response
// @Router /columns/{id} [patch]
func EditColumnHandler(c *gin.Context) {
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
	p := new(models.ParamUpdateColumn)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.EditColumn(userID, columnID, p); err != nil {
		zap.L().Error("logic.EditColumn failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

// DeleteColumnHandler 删除专栏
// @Summary 删除专栏
// @Description 软删除；文章转为无专栏文章
// @Tags 专栏相关接口
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "专栏 ID"
// @Success 200 {object} controller.Response
// @Router /columns/{id} [delete]
func DeleteColumnHandler(c *gin.Context) {
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
	if err := logic.DeleteColumn(userID, columnID); err != nil {
		zap.L().Error("logic.DeleteColumn failed", zap.Error(err))
		if errors.Is(err, logic.ErrForbidden) {
			ResponseError(c, CodeForbidden)
			return
		}
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}
