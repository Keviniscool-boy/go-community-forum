package mysql

// 用户拉黑（第五阶段 PRD F2）
// 规则：拉黑瞬间自动解除双向关注；拉黑后互相不可见、不可关注、不可私信；
// 拉黑期间对方的互动不产生新通知；取消拉黑后可重新关注。

import (
	"database/sql"
	"errors"
	"strconv"

	"bluebell/models"
)

var ErrBlockSelf = errors.New("不能拉黑自己")

// IsBlockedBetween 双向屏蔽判定：任一方拉黑对方即为真
func IsBlockedBetween(a, b int64) (bool, error) {
	var n int
	err := db.Get(&n,
		"SELECT COUNT(*) FROM user_block WHERE (blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)",
		a, b, b, a)
	return n > 0, err
}

// isBlockedBetweenTx 事务内双向屏蔽判定（通知跳过/关注拒绝等场景）
func isBlockedBetweenTx(tx TxLike, a, b int64) (bool, error) {
	var n int
	err := tx.Get(&n,
		"SELECT COUNT(*) FROM user_block WHERE (blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)",
		a, b, b, a)
	return n > 0, err
}

// ListBlocks 我的黑名单（cursor 分页；按拉黑时间倒序）
func ListBlocks(userID int64, cursorTime sql.NullTime, cursorID int64, limit int) (items []*models.BlockedUserItem, err error) {
	items = make([]*models.BlockedUserItem, 0)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := `SELECT b.blocked_id, u.username, u.avatar_url, b.create_time
		FROM user_block b JOIN user u ON u.user_id = b.blocked_id
		WHERE b.blocker_id = ?`
	args := []interface{}{userID}
	if cursorID > 0 {
		query += " AND (b.create_time < ? OR (b.create_time = ? AND b.blocked_id < ?))"
		args = append(args, cursorTime.Time, cursorTime.Time, cursorID)
	}
	query += " ORDER BY b.create_time DESC, b.blocked_id DESC LIMIT ?"
	args = append(args, limit+1)
	err = db.Select(&items, query, args...)
	return
}

// FilterBlockViewerSQL 生成"作者未被 viewer 拉黑"的过滤片段（viewer <= 0 时返回空串，跳过过滤）
// authorExpr 为作者列表达式，如 "q.author_id"。
func FilterBlockViewerSQL(viewerID int64, authorExpr string) string {
	if viewerID <= 0 {
		return ""
	}
	return " AND NOT EXISTS (SELECT 1 FROM user_block ub WHERE ub.blocker_id = " +
		strconv.FormatInt(viewerID, 10) + " AND ub.blocked_id = " + authorExpr + ")"
}

// BlockedIDsFor viewer 拉黑的用户 id 集合（热榜/推荐缓存读时过滤用）
func BlockedIDsFor(viewerID int64) ([]int64, error) {
	if viewerID <= 0 {
		return nil, nil
	}
	ids := make([]int64, 0, 8)
	err := db.Select(&ids, "SELECT blocked_id FROM user_block WHERE blocker_id = ?", viewerID)
	return ids, err
}
