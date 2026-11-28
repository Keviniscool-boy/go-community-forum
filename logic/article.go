package logic

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"
	"go.uber.org/zap"
)

// 文章与专栏业务错误
var (
	ErrArticleInvalid  = errors.New("文章参数不合法")
	ErrColumnInvalid   = errors.New("专栏参数不合法")
	ErrColumnOwnership = errors.New("无权操作该专栏")
)

// parseColumnRef 解析可选专栏 ID（空串/0 = 无专栏）
func parseColumnRef(s string) int64 {
	if s == "" {
		return 0
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 {
		return v
	}
	return -1 // 非法
}

// CreateArticle 创建文章（发布审核流：fail-closed 敏感词检查）
func CreateArticle(p *models.ParamCreateArticle, authorID int64) (articleID int64, err error) {
	if strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Content) == "" {
		return 0, ErrArticleInvalid
	}
	columnID := int64(0)
	if p.ColumnID != "" {
		columnID = parseColumnRef(p.ColumnID)
		if columnID < 0 {
			return 0, ErrArticleInvalid
		}
		// 专栏必须存在且为本人所有（前置阶段：仅所有者可向专栏投文章）
		c, err := mysql.GetColumnByID(columnID)
		if err != nil {
			if errors.Is(err, mysql.ErrColumnNotExist) {
				return 0, ErrColumnInvalid
			}
			return 0, err
		}
		if c.OwnerID != authorID {
			return 0, ErrColumnOwnership
		}
	}
	// 发布审核流（规格 7.1，fail-closed）
	decision, err := CheckContentModeration(p.Title, p.Content)
	if err != nil {
		return 0, err
	}
	a := &models.Article{
		ArticleID: snowflake.GenerateID(),
		AuthorID:  authorID,
		ColumnID:  columnID,
		Title:     strings.TrimSpace(p.Title),
		Content:   p.Content,
	}
	if err = mysql.CreateArticle(a, decision.Status); err != nil {
		return 0, err
	}
	return a.ArticleID, nil
}

// GetArticleDetail 公开文章详情（author_only 仅作者可见），自增浏览量
func GetArticleDetail(id int64, viewerID int64) (*models.ApiArticleDetail, error) {
	a, err := mysql.GetArticlePublicByID(id, viewerID)
	if err != nil {
		return nil, err
	}
	if err := mysql.IncrementArticleViewCount(id); err != nil {
		zap.L().Warn("increment article view_count failed", zap.Error(err))
	}
	detail := &models.ApiArticleDetail{Article: a}
	if user, err := mysql.GetUserByID(a.AuthorID); err == nil {
		detail.AuthorName = user.Username
	}
	if a.ColumnID > 0 {
		if c, err := mysql.GetColumnByID(a.ColumnID); err == nil {
			detail.Column = &models.ColumnBrief{ColumnID: int64ToString(c.ColumnID), Name: c.Name}
		}
	}
	return detail, nil
}

// EditArticle 作者编辑文章（更新内容不自动改变审核状态；重新发布走编辑后按策略重审——
// 当前模型：编辑保留原审核状态，敏感词命中由举报与审核链路兜底）
func EditArticle(userID, articleID int64, p *models.ParamUpdateArticle) error {
	if strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Content) == "" {
		return ErrArticleInvalid
	}
	updated, err := mysql.UpdateArticle(articleID, userID, strings.TrimSpace(p.Title), p.Content)
	if err != nil {
		return err
	}
	if !updated {
		return ErrForbidden
	}
	return nil
}

// DeleteArticle 作者软删除文章
func DeleteArticle(userID, articleID int64) error {
	deleted, err := mysql.SoftDeleteArticle(articleID, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrForbidden
	}
	return nil
}

// ListArticlesPage 公开文章列表（cursor 分页）
func ListArticlesPage(cursor string, limit int) (*models.PageResult[*models.ApiArticleDetail], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var cursorTime sql.NullTime
	var cursorID int64
	if cursor != "" {
		c, err := parseTimeCursor(cursor)
		if err != nil {
			return nil, ErrCursorInvalid
		}
		cursorTime.Time = c.Time
		cursorID = c.ID
	}
	articles, err := mysql.ListArticles(cursorTime, cursorID, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(articles) > limit {
		hasMore = true
		articles = articles[:limit]
	}
	items := make([]*models.ApiArticleDetail, 0, len(articles))
	var last *models.Article
	for _, a := range articles {
		last = a
		detail := &models.ApiArticleDetail{Article: a}
		if user, err := mysql.GetUserByID(a.AuthorID); err == nil {
			detail.AuthorName = user.Username
		}
		if a.ColumnID > 0 {
			if c, err := mysql.GetColumnByID(a.ColumnID); err == nil {
				detail.Column = &models.ColumnBrief{ColumnID: int64ToString(c.ColumnID), Name: c.Name}
			}
		}
		items = append(items, detail)
	}
	next := ""
	if hasMore && last != nil {
		next = encodeFeedTimeCursor(last.CreateTime, "article", last.ArticleID)
	}
	return &models.PageResult[*models.ApiArticleDetail]{Items: items, NextCursor: next, HasMore: hasMore}, nil
}

// ---------- 专栏 ----------

// CreateColumn 创建专栏（fail-closed 审核）
func CreateColumn(p *models.ParamCreateColumn, ownerID int64) (columnID int64, err error) {
	if strings.TrimSpace(p.Name) == "" {
		return 0, ErrColumnInvalid
	}
	decision, err := CheckContentModeration(p.Name, p.Description)
	if err != nil {
		return 0, err
	}
	c := &models.Column{
		ColumnID:    snowflake.GenerateID(),
		OwnerID:     ownerID,
		Name:        strings.TrimSpace(p.Name),
		Description: p.Description,
	}
	if err = mysql.CreateColumn(c, decision.Status); err != nil {
		return 0, err
	}
	return c.ColumnID, nil
}

// GetColumnDetail 专栏详情（公开可见 + 文章数）
func GetColumnDetail(id int64) (*models.ApiColumnDetail, error) {
	c, err := mysql.GetColumnPublicByID(id)
	if err != nil {
		return nil, err
	}
	detail := &models.ApiColumnDetail{Column: c}
	if user, err := mysql.GetUserByID(c.OwnerID); err == nil {
		detail.OwnerName = user.Username
	}
	if n, err := mysql.CountArticlesByColumn(id); err == nil {
		detail.ArticleCount = n
	}
	return detail, nil
}

// EditColumn 所有者编辑专栏
func EditColumn(userID, columnID int64, p *models.ParamUpdateColumn) error {
	if strings.TrimSpace(p.Name) == "" {
		return ErrColumnInvalid
	}
	updated, err := mysql.UpdateColumn(columnID, userID, strings.TrimSpace(p.Name), p.Description)
	if err != nil {
		return err
	}
	if !updated {
		return ErrForbidden
	}
	return nil
}

// DeleteColumn 所有者删除专栏（文章转为无专栏）
func DeleteColumn(userID, columnID int64) error {
	deleted, err := mysql.SoftDeleteColumn(columnID, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrForbidden
	}
	return nil
}

// ListColumnsPage 公开专栏列表
func ListColumnsPage(limit int) ([]*models.ApiColumnDetail, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	columns, err := mysql.ListColumns(limit)
	if err != nil {
		return nil, err
	}
	details := make([]*models.ApiColumnDetail, 0, len(columns))
	for _, c := range columns {
		detail := &models.ApiColumnDetail{Column: c}
		if user, err := mysql.GetUserByID(c.OwnerID); err == nil {
			detail.OwnerName = user.Username
		}
		if n, err := mysql.CountArticlesByColumn(c.ColumnID); err == nil {
			detail.ArticleCount = n
		}
		details = append(details, detail)
	}
	return details, nil
}

// ListColumnArticles 专栏文章列表（读者公开视图）
func ListColumnArticles(columnID int64, viewerID int64, limit int) ([]*models.ApiArticleDetail, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if _, err := mysql.GetColumnPublicByID(columnID); err != nil {
		return nil, err
	}
	articles, err := mysql.ListArticlesByColumn(columnID, viewerID, limit)
	if err != nil {
		return nil, err
	}
	details := make([]*models.ApiArticleDetail, 0, len(articles))
	for _, a := range articles {
		detail := &models.ApiArticleDetail{Article: a}
		if user, err := mysql.GetUserByID(a.AuthorID); err == nil {
			detail.AuthorName = user.Username
		}
		details = append(details, detail)
	}
	return details, nil
}

