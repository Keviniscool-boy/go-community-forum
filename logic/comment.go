package logic

import (
	"database/sql"
	"errors"
	"strconv"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// 评论体系业务逻辑：两级结构（一级评论 + 楼中楼回复）、点赞、删除。
// 列表组装统一走 assembleCommentDetails：批量补作者、点赞状态与删除权限，避免 N+1 查询。

var ErrCommentNotExist = errors.New("评论不存在")

// parseCommentTarget 解析并校验 target_type / target_id
func parseCommentTarget(targetTypeStr, targetIDStr string) (int32, int64, error) {
	targetType := models.TargetTypeToCode(targetTypeStr)
	if targetType == 0 {
		return 0, 0, ErrCommentInvalid
	}
	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil || targetID <= 0 {
		return 0, 0, ErrCommentInvalid
	}
	return targetType, targetID, nil
}

// CreateComment 发表评论（parent_id 为空为一级评论，否则为楼中楼回复）
// 回复只能挂在同一目标下、当前可见的一级评论上；评论、计数与通知在同一事务。
func CreateComment(p *models.ParamCreateComment, userID int64) (commentID int64, err error) {
	targetType, targetID, err := parseCommentTarget(p.TargetType, p.TargetID)
	if err != nil {
		return 0, err
	}
	var parentID, replyToUserID int64
	if p.ParentID != "" {
		target, err := strconv.ParseInt(p.ParentID, 10, 64)
		if err != nil || target <= 0 {
			return 0, ErrCommentInvalid
		}
		parent, err := mysql.GetCommentByID(target)
		if err != nil || !commentVisible(parent) || parent.TargetType != targetType || parent.TargetID != targetID {
			// 被回复的评论必须属于同一目标且当前可见（已删除 / 未过审的评论不可回复）
			return 0, ErrCommentInvalid
		}
		if parent.ParentID == 0 {
			parentID = parent.CommentID
		} else {
			// 回复另一条回复：挂到其所属一级评论下，并记录被回复人（「回复 @xxx」）
			parentID = parent.ParentID
			replyToUserID = parent.UserID
			root, err := mysql.GetCommentByID(parentID)
			if err != nil || !commentVisible(root) {
				return 0, ErrCommentInvalid
			}
		}
	}
	cm := &models.Comment{
		CommentID:     snowflake.GenerateID(),
		UserID:        userID,
		TargetType:    targetType,
		TargetID:      targetID,
		ParentID:      parentID,
		ReplyToUserID: replyToUserID,
		Content:       p.Content,
	}
	// 目标不存在时 dao 层事务内返回 ErrAnswerNotExist / ErrQuestionNotExist
	if err = mysql.CreateComment(cm); err != nil {
		switch {
		case errors.Is(err, mysql.ErrAnswerNotExist):
			return 0, ErrAnswerNotExist
		case errors.Is(err, mysql.ErrQuestionNotExist):
			return 0, ErrQuestionNotExist
		case errors.Is(err, mysql.ErrCommentNotExist):
			return 0, ErrCommentInvalid
		default:
			return 0, err
		}
	}
	return cm.CommentID, nil
}

// commentVisible 评论当前是否对外可见（已发布 + 审核通过 + 未软删除）
func commentVisible(cm *models.Comment) bool {
	return cm != nil && cm.Status == 1 && cm.ModerationStatus == 1 && !cm.DeletedAt.Valid
}

// assembleCommentDetails 批量组装评论详情（作者、被回复人、点赞状态、删除权限）
// 已删除的占位评论抹掉内容与作者信息，只保留「该评论已删除」语义。
func assembleCommentDetails(rows []*mysql.CommentRow, viewerID int64, privileged bool) ([]*models.ApiCommentDetail, error) {
	userIDs := make([]int64, 0, len(rows)*2)
	commentIDs := make([]int64, 0, len(rows))
	for _, r := range rows {
		if r.DeletedAt.Valid {
			continue
		}
		userIDs = append(userIDs, r.UserID)
		if r.ReplyToUserID != 0 {
			userIDs = append(userIDs, r.ReplyToUserID)
		}
		commentIDs = append(commentIDs, r.CommentID)
	}
	users, err := mysql.GetUsersByIDs(userIDs)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*models.User, len(users))
	for _, u := range users {
		userMap[u.UserID] = u
	}
	liked, err := mysql.UserLikedCommentIDs(viewerID, commentIDs)
	if err != nil {
		return nil, err
	}
	items := make([]*models.ApiCommentDetail, 0, len(rows))
	for _, r := range rows {
		detail := &models.ApiCommentDetail{
			Comment:    &r.Comment,
			LikeCount:  r.LikeCount,
			ReplyCount: r.ReplyCount,
			Liked:      liked[r.CommentID],
			CanDelete:  viewerID > 0 && (r.UserID == viewerID || privileged),
		}
		if r.DeletedAt.Valid {
			detail.IsDeleted = true
			detail.Content = ""
			detail.UserID = 0
			detail.Username = ""
			detail.AvatarURL = ""
			detail.LikeCount = 0
			detail.Liked = false
			detail.ReplyToUser = ""
			items = append(items, detail)
			continue
		}
		if u := userMap[r.UserID]; u != nil {
			detail.Username = u.Username
			detail.AvatarURL = u.AvatarURL
		}
		if r.ReplyToUserID != 0 {
			if u := userMap[r.ReplyToUserID]; u != nil {
				detail.ReplyToUser = u.Username
			}
		}
		items = append(items, detail)
	}
	return items, nil
}

// pageCommentDetails 游标分页的通用收尾：裁掉多取的一条、生成 next_cursor
func pageCommentDetails(rows []*mysql.CommentRow, limit int, viewerID int64, privileged bool) (*models.PageResult[*models.ApiCommentDetail], error) {
	hasMore := false
	if len(rows) > limit {
		hasMore = true
		rows = rows[:limit]
	}
	items, err := assembleCommentDetails(rows, viewerID, privileged)
	if err != nil {
		return nil, err
	}
	next := ""
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		next = encodeTimeCursor(last.CreateTime, last.CommentID)
	}
	return &models.PageResult[*models.ApiCommentDetail]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// parseCommentCursor 解析评论列表游标
func parseCommentCursor(cursor string) (sql.NullTime, int64, error) {
	var cursorTime sql.NullTime
	var cursorID int64
	if cursor != "" {
		c, err := parseTimeCursor(cursor)
		if err != nil {
			return cursorTime, 0, ErrCursorInvalid
		}
		cursorTime.Time = c.Time
		cursorID = c.ID
	}
	return cursorTime, cursorID, nil
}

// normalizeCommentLimit 评论分页 limit：默认 20，最大 50
func normalizeCommentLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}

// ListComments 一级评论列表（cursor 分页，最新在前；回复需走 ListCommentReplies）
func ListComments(viewerID int64, viewerRole, targetTypeStr, targetIDStr, cursor string, limit int) (*models.PageResult[*models.ApiCommentDetail], error) {
	limit = normalizeCommentLimit(limit)
	targetType, targetID, err := parseCommentTarget(targetTypeStr, targetIDStr)
	if err != nil {
		return nil, err
	}
	cursorTime, cursorID, err := parseCommentCursor(cursor)
	if err != nil {
		return nil, err
	}
	rows, err := mysql.ListComments(&mysql.CommentPageParams{
		TargetType: targetType,
		TargetID:   targetID,
		CursorTime: cursorTime,
		CursorID:   cursorID,
		Limit:      limit,
	})
	if err != nil {
		return nil, err
	}
	return pageCommentDetails(rows, limit, viewerID, models.IsPrivilegedRole(viewerRole))
}

// ListCommentReplies 某条一级评论下的回复（cursor 分页，最早在前）
// 父评论已删除时仍可查看其下回复（列表中以占位形式展示父评论）。
func ListCommentReplies(viewerID int64, viewerRole, commentIDStr, cursor string, limit int) (*models.PageResult[*models.ApiCommentDetail], error) {
	limit = normalizeCommentLimit(limit)
	parentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil || parentID <= 0 {
		return nil, ErrCommentInvalid
	}
	parent, err := mysql.GetCommentByID(parentID)
	if err != nil {
		return nil, ErrCommentNotExist
	}
	if parent.ParentID != 0 {
		// 两级结构：回复没有下级，返回空页
		return &models.PageResult[*models.ApiCommentDetail]{Items: []*models.ApiCommentDetail{}}, nil
	}
	cursorTime, cursorID, err := parseCommentCursor(cursor)
	if err != nil {
		return nil, err
	}
	rows, err := mysql.ListCommentReplies(parentID, cursorTime, cursorID, limit)
	if err != nil {
		return nil, err
	}
	return pageCommentDetails(rows, limit, viewerID, models.IsPrivilegedRole(viewerRole))
}

// DeleteComment 删除评论：作者本人，或管理员 / 审核员
// 删除为软删除，其下回复保留；所属内容的评论数与父评论的回复数在事务内重算。
func DeleteComment(userID int64, role, commentIDStr string) error {
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil || commentID <= 0 {
		return ErrCommentInvalid
	}
	cm, err := mysql.GetCommentByID(commentID)
	if err != nil {
		return ErrCommentNotExist
	}
	if cm.UserID != userID && !models.IsPrivilegedRole(role) {
		return ErrForbidden
	}
	if err := mysql.SoftDeleteComment(commentID, userID, models.IsPrivilegedRole(role)); err != nil {
		if errors.Is(err, mysql.ErrCommentForbidden) {
			return ErrForbidden
		}
		return err
	}
	return nil
}

// LikeComment 点赞评论（幂等）
func LikeComment(userID, commentID int64) error {
	if err := mysql.SetCommentLike(userID, commentID, true); err != nil {
		if errors.Is(err, mysql.ErrCommentNotExist) {
			return ErrCommentNotExist
		}
		return err
	}
	return nil
}

// UnlikeComment 取消点赞（幂等）
func UnlikeComment(userID, commentID int64) error {
	if err := mysql.SetCommentLike(userID, commentID, false); err != nil {
		if errors.Is(err, mysql.ErrCommentNotExist) {
			return ErrCommentNotExist
		}
		return err
	}
	return nil
}
