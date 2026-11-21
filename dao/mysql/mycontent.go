package mysql

// GET /me/content：当前用户内容管理列表（问题/回答/文章统一形状，cursor 分页）
// 用户看到自己的内容状态（公开/审核中/退回隔离/已删除），审核与治理状态对作者可见（PRD §4.2）。

import (
	"time"

	"bluebell/models"
)

// myContentRow 跨表扫描行
type myContentRow struct {
	ContentID        int64     `db:"content_id"`
	Title            string    `db:"title"`
	Excerpt          string    `db:"excerpt"`
	Status           int32     `db:"status"`
	ModerationStatus int32     `db:"moderation_status"`
	ResolutionStatus int32     `db:"resolution_status"`
	CreateTime       time.Time `db:"create_time"`
}

// myContentStatusFilter 状态过滤 → SQL 条件；col 为列限定前缀（"a." 或 ""，单表查询无需前缀）
func myContentStatusFilter(col, status string) string {
	switch status {
	case models.MeContentPublished:
		return " AND " + col + "status = 1 AND " + col + "moderation_status = 1 AND " + col + "visibility = 1 AND " + col + "deleted_at IS NULL"
	case models.MeContentPending:
		return " AND " + col + "status = 1 AND " + col + "moderation_status = 0 AND " + col + "deleted_at IS NULL"
	case models.MeContentHidden:
		// 退回（拒绝/隔离）或管理员隐藏
		return " AND " + col + "status = 1 AND " + col + "deleted_at IS NULL AND (" + col + "moderation_status IN (2, 3) OR " + col + "visibility = 0)"
	case models.MeContentDeleted:
		return " AND (" + col + "deleted_at IS NOT NULL OR " + col + "status = 0)"
	default:
		// 空 = 全部状态
		return ""
	}
}

// ListUserContent 按 cursor 分页查询用户自己的内容（contentType 必须已校验为合法枚举）
func ListUserContent(userID int64, contentType, status string, cursorTime time.Time, cursorID int64, limit int) (items []*models.ApiMyContentItem, err error) {
	items = make([]*models.ApiMyContentItem, 0)
	fetchLimit := limit + 1

	// 每个类型使用完全限定列名（回答 JOIN question，避免 create_time/status 歧义）
	var query string
	args := []interface{}{userID} // author_id 绑定在最前
	switch contentType {
	case models.MeContentQuestion:
		query = `SELECT question_id AS content_id, title, '' AS excerpt, status,
			 moderation_status, resolution_status, create_time
			 FROM question WHERE author_id = ?` + myContentStatusFilter("", status)
		if cursorID > 0 {
			query += " AND (create_time < ? OR (create_time = ? AND question_id < ?))"
			args = append(args, cursorTime, cursorTime, cursorID)
		}
	case models.MeContentAnswer:
		query = `SELECT a.answer_id AS content_id, qu.title AS title, LEFT(a.content, 100) AS excerpt,
			 a.status, a.moderation_status, 0 AS resolution_status, a.create_time
			 FROM answer a JOIN question qu ON qu.question_id = a.question_id
			 WHERE a.author_id = ?` + myContentStatusFilter("a.", status)
		if cursorID > 0 {
			query += " AND (a.create_time < ? OR (a.create_time = ? AND a.answer_id < ?))"
			args = append(args, cursorTime, cursorTime, cursorID)
		}
	case models.MeContentArticle:
		query = `SELECT article_id AS content_id, title, LEFT(content, 100) AS excerpt, status,
			 moderation_status, 0 AS resolution_status, create_time
			 FROM article WHERE author_id = ?` + myContentStatusFilter("", status)
		if cursorID > 0 {
			query += " AND (create_time < ? OR (create_time = ? AND article_id < ?))"
			args = append(args, cursorTime, cursorTime, cursorID)
		}
	default:
		// controller binding 校验枚举；此处兜底防御非法类型
		return nil, ErrInvalidParams
	}
	query += " ORDER BY create_time DESC, content_id DESC LIMIT ?"
	args = append(args, fetchLimit)

	rows := make([]*myContentRow, 0, fetchLimit)
	if err = db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		items = append(items, &models.ApiMyContentItem{
			ContentType:      contentType,
			ContentID:        r.ContentID,
			Title:            r.Title,
			Excerpt:          r.Excerpt,
			Status:           r.Status,
			ModerationStatus: r.ModerationStatus,
			ResolutionStatus: r.ResolutionStatus,
			CreateTime:       r.CreateTime,
		})
	}
	return items, nil
}
