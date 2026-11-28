package logic

import (
	"database/sql"
	"errors"
	"time"
	"strconv"

	"bluebell/dao/mysql"
	"bluebell/models"
)

// 互动相关业务错误
var ErrAnswerNotExist = errors.New("回答不存在")
var ErrCommentInvalid = errors.New("评论参数不合法")

// ---------- 关注用户 ----------

// FollowUser 关注用户（通知在 dao 事务内写入）
var ErrFollowBlocked = errors.New("对方暂不接受关注")

func FollowUser(followerID, followingID int64) error {
	// 拉黑任一方向 → 统一文案拒绝（PRD F2：不暴露"被拉黑"字样，防关系探测）
	if blocked, err := mysql.IsBlockedBetween(followerID, followingID); err != nil {
		return err
	} else if blocked {
		return ErrFollowBlocked
	}
	return mysql.FollowUser(followerID, followingID)
}

// UnfollowUser 取消关注
func UnfollowUser(followerID, followingID int64) error {
	return mysql.UnfollowUser(followerID, followingID)
}

// ErrNotificationForbidden 他人通知越权（PRD F3 验收）
var ErrNotificationForbidden = errors.New("无权操作该通知")

// MarkNotificationRead 单条已读：仅接收人本人；重复调用幂等
func MarkNotificationRead(userID int64, notificationIDStr string) error {
	id, err := strconv.ParseInt(notificationIDStr, 10, 64)
	if err != nil || id <= 0 {
		return ErrCommentInvalid
	}
	// 归属校验：不存在与越权区分（PRD F3 验收：越权返回禁止）
	n, err := mysql.GetNotificationByID(id)
	if err != nil {
		if errors.Is(err, mysql.ErrNotificationNotExist) {
			return ErrCommentInvalid
		}
		return err
	}
	if n.UserID != userID {
		return ErrNotificationForbidden
	}
	_, err = mysql.MarkNotificationRead(id, userID)
	return err
}

// IsFollowing 查询关注关系
func IsFollowing(followerID, followingID int64) (bool, error) {
	if followerID <= 0 {
		return false, nil
	}
	return mysql.IsFollowing(followerID, followingID)
}

// ---------- 回答投票 ----------

// VoteAnswer 投票（通知在 dao 事务内写入）
func VoteAnswer(userID, answerID int64, direction int8) error {
	return mysql.VoteAnswer(userID, answerID, direction)
}

// GetUserVoteDirection 当前用户对回答的投票方向
func GetUserVoteDirection(userID, answerID int64) (int8, error) {
	if userID <= 0 {
		return 0, nil
	}
	return mysql.GetUserVoteDirection(userID, answerID)
}

// ---------- 收藏 ----------

// CollectAnswer 收藏回答（通知在 dao 事务内写入）
func CollectAnswer(userID, answerID int64) error {
	return mysql.CollectAnswer(userID, answerID)
}

// UncollectAnswer 取消收藏
func UncollectAnswer(userID, answerID int64) error {
	return mysql.UncollectAnswer(userID, answerID)
}

// IsCollected 是否已收藏
func IsCollected(userID, answerID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	return mysql.IsCollected(userID, answerID)
}

// ---------- 关注问题 ----------

// FollowQuestion 关注问题（通知在 dao 事务内写入）
func FollowQuestion(userID, questionID int64) error {
	return mysql.FollowQuestion(userID, questionID)
}

// UnfollowQuestion 取消关注问题
func UnfollowQuestion(userID, questionID int64) error {
	return mysql.UnfollowQuestion(userID, questionID)
}

// IsQuestionFollowed 是否已关注问题
func IsQuestionFollowed(userID, questionID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	return mysql.IsQuestionFollowed(userID, questionID)
}

// ListFollowers 粉丝列表（PRD F1）
func ListFollowers(targetID, viewerID, cursorID, limit int64) (*models.PageResult[*models.FollowListUser], error) {
	if targetID <= 0 {
		return nil, mysql.ErrInvalidParams
	}
	// 双向屏蔽统一不可枚举（PRD F2：直接访问列表返回不存在）
	if viewerID > 0 && viewerID != targetID {
		if blocked, err := mysql.IsBlockedBetween(targetID, viewerID); err == nil && blocked {
			return nil, mysql.ErrUserNotExist
		}
	}
	items, err := mysql.ListFollowers(targetID, viewerID, cursorID, int(limit))
	if err != nil {
		return nil, err
	}
	// DAO 返回 limit+1 判定 has_more
	hasMore := false
	if len(items) > int(limit) {
		hasMore = true
		items = items[:limit]
	}
	next := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		next = strconv.FormatInt(last.ListCursorID, 10)
	}
	return &models.PageResult[*models.FollowListUser]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// ListFollowing 关注列表（PRD F1）
func ListFollowing(targetID, viewerID, cursorID, limit int64) (*models.PageResult[*models.FollowListUser], error) {
	if targetID <= 0 {
		return nil, mysql.ErrInvalidParams
	}
	if viewerID > 0 && viewerID != targetID {
		if blocked, err := mysql.IsBlockedBetween(targetID, viewerID); err == nil && blocked {
			return nil, mysql.ErrUserNotExist
		}
	}
	items, err := mysql.ListFollowing(targetID, viewerID, cursorID, int(limit))
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(items) > int(limit) {
		hasMore = true
		items = items[:limit]
	}
	next := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		next = strconv.FormatInt(last.ListCursorID, 10)
	}
	return &models.PageResult[*models.FollowListUser]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// BlockUser 拉黑（PRD F2；幂等；事务内解除双向关注）
func BlockUser(blockerID, blockedID int64) error {
	return mysql.BlockUser(blockerID, blockedID)
}

// UnblockUser 取消拉黑（幂等）
func UnblockUser(blockerID, blockedID int64) error {
	return mysql.UnblockUser(blockerID, blockedID)
}

// ListBlocks 我的黑名单（cursor 分页；opaque 游标在 logic 内解析）
func ListBlocks(userID int64, cursor string, limit int) (*models.PageResult[*models.BlockedUserItem], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var cursorTime time.Time
	var cursorID int64
	if cursor != "" {
		c, err := parseTimeCursor(cursor)
		if err != nil {
			return nil, ErrCursorInvalid
		}
		cursorTime, cursorID = c.Time, c.ID
	}
	nt := sql.NullTime{Time: cursorTime, Valid: !cursorTime.IsZero()}
	items, err := mysql.ListBlocks(userID, nt, cursorID, limit)
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(items) > limit {
		hasMore = true
		items = items[:limit]
	}
	next := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		next = encodeTimeCursor(last.CreateTime, last.BlockedID)
	}
	return &models.PageResult[*models.BlockedUserItem]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}
