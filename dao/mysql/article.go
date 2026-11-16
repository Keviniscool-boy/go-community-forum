package mysql

import (
	"database/sql"
	"errors"

	"bluebell/models"
)

var (
	ErrArticleNotExist = errors.New("文章不存在")
	ErrColumnNotExist  = errors.New("专栏不存在")
)

// ---------- 文章 ----------

// CreateArticle 创建文章（moderationStatus 由发布审核流决定）
func CreateArticle(a *models.Article, moderationStatus int32) error {
	_, err := db.Exec(
		"INSERT INTO article (article_id, author_id, column_id, title, content, moderation_status) VALUES (?, ?, ?, ?, ?, ?)",
		a.ArticleID, a.AuthorID, a.ColumnID, a.Title, a.Content, moderationStatus)
	return err
}

// GetArticleByID 内部用途：status=1 校验（作者编辑等）
func GetArticleByID(id int64) (*models.Article, error) {
	a := new(models.Article)
	err := db.Get(a, `SELECT article_id, author_id, column_id, title, content, view_count,
		status, moderation_status, visibility, deleted_at, update_time, create_time
		FROM article WHERE article_id = ? AND status = 1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrArticleNotExist
		}
		return nil, err
	}
	return a, nil
}

// GetArticlePublicByID 公开详情：统一治理可见条件（author_only 仅作者本人）
func GetArticlePublicByID(id int64, viewerID int64) (*models.Article, error) {
	a := new(models.Article)
	err := db.Get(a, `SELECT a.article_id, a.author_id, a.column_id, a.title, a.content, a.view_count,
		a.status, a.moderation_status, a.visibility, a.deleted_at, a.update_time, a.create_time
		FROM article a JOIN user u ON u.user_id = a.author_id AND u.status = 1
		WHERE a.article_id = ? AND a.status = 1
		AND a.moderation_status = 1 AND a.deleted_at IS NULL
		AND (a.visibility = 1 OR (a.visibility = 2 AND a.author_id = ?))`, id, viewerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrArticleNotExist
		}
		return nil, err
	}
	return a, nil
}

// UpdateArticle 作者编辑文章
func UpdateArticle(articleID, authorID int64, title, content string) (bool, error) {
	result, err := db.Exec(
		"UPDATE article SET title = ?, content = ?, updated_by = ? WHERE article_id = ? AND author_id = ? AND status = 1",
		title, content, authorID, articleID, authorID)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// SoftDeleteArticle 作者软删除文章
func SoftDeleteArticle(articleID, authorID int64) (bool, error) {
	result, err := db.Exec(
		"UPDATE article SET status = 0, deleted_at = NOW(), updated_by = ? WHERE article_id = ? AND author_id = ? AND status = 1",
		authorID, articleID, authorID)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// IncrementArticleViewCount 浏览量自增
func IncrementArticleViewCount(articleID int64) error {
	_, err := db.Exec("UPDATE article SET view_count = view_count + 1 WHERE article_id = ?", articleID)
	return err
}

// ListArticles 公开文章列表（时间倒序，分页）
func ListArticles(cursorTime sql.NullTime, cursorID int64, limit int) (articles []*models.Article, err error) {
	articles = make([]*models.Article, 0)
	q := limit + 1
	var query string
	var args []interface{}
	if cursorID > 0 {
		query = `SELECT article_id, author_id, column_id, title, LEFT(content, 200) AS content, view_count,
			status, moderation_status, visibility, deleted_at, update_time, create_time
			FROM article a JOIN user u ON u.user_id = a.author_id AND u.status = 1
			WHERE a.status = 1 AND a.moderation_status = 1 AND a.visibility = 1 AND a.deleted_at IS NULL
			AND (a.create_time < ? OR (a.create_time = ? AND a.article_id < ?))
			ORDER BY a.create_time DESC, a.article_id DESC LIMIT ?`
		args = []interface{}{cursorTime.Time, cursorTime.Time, cursorID, q}
	} else {
		query = `SELECT article_id, author_id, column_id, title, LEFT(content, 200) AS content, view_count,
			status, moderation_status, visibility, deleted_at, update_time, create_time
			FROM article a JOIN user u ON u.user_id = a.author_id AND u.status = 1
			WHERE a.status = 1 AND a.moderation_status = 1 AND a.visibility = 1 AND a.deleted_at IS NULL
			ORDER BY a.create_time DESC, a.article_id DESC LIMIT ?`
		args = []interface{}{q}
	}
	err = db.Select(&articles, query, args...)
	return
}

// ListArticlesByColumn 专栏下的文章（读者视图：公开；作者视图含全部）
func ListArticlesByColumn(columnID int64, viewerID int64, limit int) (articles []*models.Article, err error) {
	articles = make([]*models.Article, 0, limit)
	err = db.Select(&articles, `SELECT article_id, author_id, column_id, title, content, view_count,
		status, moderation_status, visibility, deleted_at, update_time, create_time
		FROM article WHERE column_id = ? AND status = 1 AND moderation_status = 1 AND deleted_at IS NULL
		AND (visibility = 1 OR (visibility = 2 AND author_id = ?))
		ORDER BY create_time DESC LIMIT ?`, columnID, viewerID, limit)
	return
}

// CountArticlesByColumn 专栏公开文章数
func CountArticlesByColumn(columnID int64) (int32, error) {
	var n int32
	err := db.Get(&n, `SELECT COUNT(*) FROM article WHERE column_id = ? AND status = 1
		AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL`, columnID)
	return n, err
}

// SearchArticles LIKE 搜索（公开可见）
func SearchArticles(like string, limit int) (rows []struct {
	ArticleID int64
	Title     string
	Excerpt   string
	AuthorID  int64
	Score     float64
	CreateTime int64
}, err error) {
	rows = make([]struct {
		ArticleID int64
		Title     string
		Excerpt   string
		AuthorID  int64
		Score     float64
		CreateTime int64
	}, 0, limit)
	err = db.Select(&rows, `SELECT article_id AS article_id, title,
		LEFT(content, 200) AS excerpt, author_id AS author_id,
		(CASE WHEN title LIKE ? THEN 3 ELSE 0 END + CASE WHEN content LIKE ? THEN 1 ELSE 0 END) AS score,
		UNIX_TIMESTAMP(create_time) AS create_time
		FROM article a JOIN user u ON u.user_id = a.author_id AND u.status = 1
		WHERE a.status = 1 AND a.moderation_status = 1 AND a.visibility = 1 AND a.deleted_at IS NULL
		AND (title LIKE ? OR content LIKE ?) LIMIT ?`, like, like, like, like, limit)
	return
}

// ---------- 专栏 ----------

// CreateColumn 创建专栏
func CreateColumn(c *models.Column, moderationStatus int32) error {
	_, err := db.Exec(
		"INSERT INTO column_info (column_id, owner_id, name, description, moderation_status) VALUES (?, ?, ?, ?, ?)",
		c.ColumnID, c.OwnerID, c.Name, c.Description, moderationStatus)
	return err
}

// GetColumnByID 内部用途
func GetColumnByID(id int64) (*models.Column, error) {
	c := new(models.Column)
	err := db.Get(c, `SELECT column_id, owner_id, name, description, status, moderation_status,
		visibility, deleted_at, create_time FROM column_info WHERE column_id = ? AND status = 1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrColumnNotExist
		}
		return nil, err
	}
	return c, nil
}

// GetColumnPublicByID 公开详情（统一治理条件）
func GetColumnPublicByID(id int64) (*models.Column, error) {
	c := new(models.Column)
	err := db.Get(c, `SELECT column_id, owner_id, name, description, status, moderation_status,
		visibility, deleted_at, create_time FROM column_info
		WHERE column_id = ? AND status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrColumnNotExist
		}
		return nil, err
	}
	return c, nil
}

// UpdateColumn 所有者编辑专栏
func UpdateColumn(columnID, ownerID int64, name, description string) (bool, error) {
	result, err := db.Exec(
		"UPDATE column_info SET name = ?, description = ?, updated_by = ? WHERE column_id = ? AND owner_id = ? AND status = 1",
		name, description, ownerID, columnID, ownerID)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// SoftDeleteColumn 所有者软删除专栏（文章保留，column_id 置空即成为无专栏文章）
func SoftDeleteColumn(columnID, ownerID int64) (bool, error) {
	tx, err := db.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(
		"UPDATE column_info SET status = 0, deleted_at = NOW(), updated_by = ? WHERE column_id = ? AND owner_id = ? AND status = 1",
		ownerID, columnID, ownerID)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return false, nil
	}
	if _, err = tx.Exec("UPDATE article SET column_id = 0 WHERE column_id = ?", columnID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ListColumns 公开专栏列表
func ListColumns(limit int) (columns []*models.Column, err error) {
	columns = make([]*models.Column, 0, limit)
	err = db.Select(&columns, `SELECT column_id, owner_id, name, description, status, moderation_status,
		visibility, deleted_at, create_time FROM column_info
		WHERE status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL
		ORDER BY create_time DESC LIMIT ?`, limit)
	return
}

// ListColumnsByOwner 所有者的专栏
func ListColumnsByOwner(ownerID int64) (columns []*models.Column, err error) {
	columns = make([]*models.Column, 0)
	err = db.Select(&columns, `SELECT column_id, owner_id, name, description, status, moderation_status,
		visibility, deleted_at, create_time FROM column_info
		WHERE owner_id = ? AND status = 1 AND deleted_at IS NULL ORDER BY create_time DESC`, ownerID)
	return
}

