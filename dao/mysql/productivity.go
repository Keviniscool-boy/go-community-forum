package mysql

import (
	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// ---------- 收藏夹 ----------

// CreateFolder 创建收藏夹（重名拒绝）
func CreateFolder(f *models.BookmarkFolder) error {
	_, err := db.Exec("INSERT INTO bookmark_folder (folder_id, user_id, name, is_private) VALUES (?, ?, ?, 1)",
		f.FolderID, f.UserID, f.Name)
	return err
}

// ListFolders 用户收藏夹列表
func ListFolders(userID int64) (folders []*models.BookmarkFolder, err error) {
	folders = make([]*models.BookmarkFolder, 0)
	err = db.Select(&folders, "SELECT folder_id, user_id, name, is_private, created_at FROM bookmark_folder WHERE user_id = ? ORDER BY created_at", userID)
	return
}

// GetFolder 校验归属
func GetFolder(folderID, userID int64) (*models.BookmarkFolder, error) {
	f := new(models.BookmarkFolder)
	err := db.Get(f, "SELECT folder_id, user_id, name, is_private, created_at FROM bookmark_folder WHERE folder_id = ? AND user_id = ?", folderID, userID)
	return f, err
}

// DeleteFolder 删除收藏夹（连同条目）
func DeleteFolder(folderID, userID int64) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM bookmark_item WHERE folder_id = ? AND user_id = ?", folderID, userID); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM bookmark_folder WHERE folder_id = ? AND user_id = ?", folderID, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// AddBookmarkItem 收藏条目（幂等）
func AddBookmarkItem(folderID, userID int64, targetType int32, targetID int64) error {
	_, err := db.Exec("INSERT IGNORE INTO bookmark_item (folder_id, user_id, target_type, target_id) VALUES (?, ?, ?, ?)",
		folderID, userID, targetType, targetID)
	return err
}

// RemoveBookmarkItem 取消收藏条目
func RemoveBookmarkItem(folderID, userID int64, targetType int32, targetID int64) error {
	_, err := db.Exec("DELETE FROM bookmark_item WHERE folder_id = ? AND user_id = ? AND target_type = ? AND target_id = ?",
		folderID, userID, targetType, targetID)
	return err
}

// ListBookmarkItems 收藏条目（附带目标存活状态）
func ListBookmarkItems(folderID, userID int64, limit int) (items []*models.BookmarkItem, err error) {
	items = make([]*models.BookmarkItem, 0, limit)
	err = db.Select(&items, "SELECT target_type, target_id, created_at FROM bookmark_item WHERE folder_id = ? AND user_id = ? ORDER BY created_at DESC LIMIT ?",
		folderID, userID, limit)
	return
}

// TargetExists 判断目标是否公开可见（收藏展示"不可用"状态用）
func TargetExists(targetType int32, targetID int64) bool {
	var n int
	switch targetType {
	case models.ReportTargetQuestion:
		_ = db.Get(&n, "SELECT COUNT(*) FROM question WHERE question_id = ? AND status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL", targetID)
	case models.ReportTargetAnswer:
		_ = db.Get(&n, "SELECT COUNT(*) FROM answer WHERE answer_id = ? AND status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL", targetID)
	default:
		return true
	}
	return n > 0
}

// ---------- 浏览历史 ----------

// RecordView 记录浏览（登录用户；upsert 累计次数）
func RecordView(userID int64, targetType int32, targetID int64) error {
	_, err := db.Exec(
		"INSERT INTO content_view_history (user_id, target_type, target_id) VALUES (?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE last_viewed_at = NOW(), view_count = view_count + 1",
		userID, targetType, targetID)
	return err
}

// ListViewHistory 历史列表
func ListViewHistory(userID int64, limit int) (items []*models.BookmarkItem, err error) {
	items = make([]*models.BookmarkItem, 0, limit)
	err = db.Select(&items, "SELECT target_type, target_id, last_viewed_at AS created_at FROM content_view_history WHERE user_id = ? ORDER BY last_viewed_at DESC LIMIT ?",
		userID, limit)
	return
}

// ClearHistory 清空历史
func ClearHistory(userID int64) error {
	_, err := db.Exec("DELETE FROM content_view_history WHERE user_id = ?", userID)
	return err
}

// ---------- 推荐事件 ----------

// CreateRecommendationEvents 批量写入事件（事件 ID 唯一去重，重复忽略）
func CreateRecommendationEvents(events []*models.RecommendationEvent) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, e := range events {
		if _, err = tx.Exec(
			"INSERT IGNORE INTO recommendation_event (event_id, anonymous_id_hash, user_id, session_id_hash, event_type, item_type, item_id, feed_type, position, request_id, metadata_json) "+
				"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CAST(? AS JSON))",
			e.EventID, e.AnonymousIDHash, nullableID(e.UserID), e.SessionIDHash, e.EventType, e.ItemType, e.ItemID, e.FeedType, e.Position, e.RequestID, e.MetadataStr); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullableID(v int64) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

// RecommendationVersion 当前推荐规则版本（配置表，缺省默认规则 v1）
func RecommendationVersion() string {
	var value string
	if err := db.Get(&value, "SELECT setting_value FROM system_setting WHERE setting_key = 'recommendation_version'"); err == nil && value != "" {
		return value
	}
	return "rule-v1"
}

// EnsureDefaultFolder 确保用户有"我的收藏"默认收藏夹
func EnsureDefaultFolder(userID int64) {
	var n int
	_ = db.Get(&n, "SELECT COUNT(*) FROM bookmark_folder WHERE user_id = ? AND name = '我的收藏'", userID)
	if n == 0 {
		_, _ = db.Exec("INSERT IGNORE INTO bookmark_folder (folder_id, user_id, name, is_private) VALUES (?, ?, '我的收藏', 1)",
			snowflake.GenerateID(), userID)
	}
}
