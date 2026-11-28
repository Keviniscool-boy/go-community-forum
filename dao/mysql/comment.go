package mysql

import (
	"database/sql"
	"errors"

	"bluebell/models"

	"github.com/jmoiron/sqlx"
)

// 评论体系：发表（含楼中楼回复）、点赞、软删除与楼中楼查询。
// 约定为两级结构：parent_id = 0 为一级评论，其余为挂在某条一级评论下的回复。
// 计数一律以关系表为准在事务内重算或加锁递增，杜绝并发增量漂移。

var (
	ErrCommentNotExist  = errors.New("评论不存在")
	ErrCommentForbidden = errors.New("无权操作该评论")
)

// 评论公开可见条件（与 006 治理状态机一致：已发布 + 审核通过 + 未软删除 + 作者未被封禁）
// a 为 comment 表别名：列表查询会与 comment 自联表，条件列必须带别名
func commentVisibleSQL(a string) string {
	return a + ".status = 1 AND " + a + ".moderation_status = 1 AND " + a + ".deleted_at IS NULL"
}

func commentAuthorAliveSQL(a string) string {
	return a + ".user_id IN (SELECT user_id FROM user WHERE status = 1)"
}

// commentWithVisibleRepliesSQL 至少有一条可见回复（用于保留「该评论已删除」占位）
func commentWithVisibleRepliesSQL(a string) string {
	return a + ".comment_id IN (SELECT r.parent_id FROM comment r WHERE r.parent_id <> 0 AND " +
		commentVisibleSQL("r") + " AND " + commentAuthorAliveSQL("r") + ")"
}

// CommentRow 评论查询行：评论本体 + 点赞/回复聚合列
// 被回复人 reply_to_user_id 在 models.Comment 上（两级楼中楼里「回复 @xxx」）
type CommentRow struct {
	models.Comment
	LikeCount  int32 `db:"like_count"`
	ReplyCount int32 `db:"reply_count"`
}

const commentRowColumns = "c.comment_id, c.user_id, c.target_type, c.target_id, c.parent_id, c.reply_to_user_id, " +
	"c.content, c.status, c.moderation_status, c.deleted_at, c.create_time, c.update_time, c.like_count, c.reply_count"

// CommentPageParams 评论分页查询参数（cursor）
type CommentPageParams struct {
	TargetType int32
	TargetID   int64
	CursorTime sql.NullTime
	CursorID   int64
	Limit      int
	ViewerID   int64 // 拉黑过滤（0 = 游客不过滤）
}

// ListComments 按 cursor 分页查询目标的一级评论（最新在前）
// 已删除但下方仍有可见回复的一级评论保留为占位行（内容由 logic 层抹掉），其余已删除评论不返回。
func ListComments(p *CommentPageParams) (rows []*CommentRow, err error) {
	rows = make([]*CommentRow, 0)
	query := "SELECT " + commentRowColumns + " FROM comment c" +
		" WHERE c.target_type = ? AND c.target_id = ? AND c.parent_id = 0" +
		" AND ((" + commentVisibleSQL("c") + " AND " + commentAuthorAliveSQL("c") + ")" +
		" OR (c.deleted_at IS NOT NULL AND " + commentWithVisibleRepliesSQL("c") + "))" +
		FilterBlockViewerSQL(p.ViewerID, "c.user_id")
	args := []interface{}{p.TargetType, p.TargetID}
	if p.CursorID != 0 {
		query += " AND (c.create_time < ? OR (c.create_time = ? AND c.comment_id < ?))"
		args = append(args, p.CursorTime.Time, p.CursorTime.Time, p.CursorID)
	}
	query += " ORDER BY c.create_time DESC, c.comment_id DESC LIMIT ?"
	args = append(args, p.Limit+1) // 多取一条判断 has_more
	if err = db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

// ListCommentReplies 按 cursor 分页查询某条一级评论下的回复（最早在前，符合对话阅读顺序）
// 回复行带 reply_to_user_id：回复的对象本身也是回复时，前端展示「回复 @xxx」。
func ListCommentReplies(parentID int64, viewerID int64, cursorTime sql.NullTime, cursorID int64, limit int) (rows []*CommentRow, err error) {
	rows = make([]*CommentRow, 0)
	query := "SELECT " + commentRowColumns + " FROM comment c" +
		" WHERE c.parent_id = ? AND " + commentVisibleSQL("c") + " AND " + commentAuthorAliveSQL("c") +
		FilterBlockViewerSQL(viewerID, "c.user_id")
	args := []interface{}{parentID}
	if cursorID != 0 {
		query += " AND (c.create_time > ? OR (c.create_time = ? AND c.comment_id > ?))"
		args = append(args, cursorTime.Time, cursorTime.Time, cursorID)
	}
	query += " ORDER BY c.create_time ASC, c.comment_id ASC LIMIT ?"
	args = append(args, limit+1)
	if err = db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

// UpdateCommentContent 作者编辑评论内容（PRD F4）：
// 条件更新保证作者与存活状态；moderation_status 重走发布审核流由业务层决定；
// update_time 由 ON UPDATE CURRENT_TIMESTAMP 自动刷新（>create_time 即"已编辑"）。
func UpdateCommentContent(commentID, userID int64, content string, moderationStatus int32) (bool, error) {
	result, err := db.Exec(
		"UPDATE comment SET content = ?, moderation_status = ? WHERE comment_id = ? AND user_id = ? AND status = 1 AND deleted_at IS NULL",
		content, moderationStatus, commentID, userID)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// GetCommentByID 查询单条评论（回复时校验父评论；不过滤可见性，由调用方判断）
func GetCommentByID(id int64) (*models.Comment, error) {
	cm := new(models.Comment)
	err := db.Get(cm, "SELECT comment_id, user_id, target_type, target_id, parent_id, reply_to_user_id, content, status, "+
		"moderation_status, deleted_at, create_time, update_time FROM comment WHERE comment_id = ?", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCommentNotExist
		}
		return nil, err
	}
	return cm, nil
}

// lockCommentTarget 在事务内锁定评论所属的内容行并取作者（统一锁顺序：先内容行，后评论行）
// 目标必须存在；answer/question 为被软删除的内容时仍可锁（status=0 的行仍在）。
func lockCommentTarget(tx *sqlx.Tx, targetType int32, targetID int64) (authorID int64, err error) {
	switch targetType {
	case models.TargetAnswer:
		err = tx.Get(&authorID, "SELECT author_id FROM answer WHERE answer_id = ? FOR UPDATE", targetID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrAnswerNotExist
		}
	case models.TargetQuestion:
		err = tx.Get(&authorID, "SELECT author_id FROM question WHERE question_id = ? FOR UPDATE", targetID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrQuestionNotExist
		}
	default:
		return 0, ErrInvalidParams
	}
	return authorID, err
}

// recalcTargetCommentCount 以评论表为准重算目标的评论数（含回复；事务内）
func recalcTargetCommentCount(tx *sqlx.Tx, targetType int32, targetID int64) error {
	table, column := "", ""
	switch targetType {
	case models.TargetAnswer:
		table, column = "answer", "answer_id"
	case models.TargetQuestion:
		table, column = "question", "question_id"
	default:
		return ErrInvalidParams
	}
	_, err := tx.Exec("UPDATE "+table+" SET comment_count = (SELECT COUNT(*) FROM comment WHERE target_type = ? "+
		"AND target_id = ? AND status = 1 AND deleted_at IS NULL) WHERE "+column+" = ?",
		targetType, targetID, targetID)
	return err
}

// CreateComment 创建评论：评论、目标计数、父评论回复数与通知在同一事务
// 通知规则：一级评论通知内容作者；回复通知被回复的评论作者（内容作者与被回复人不同时另行通知）。
func CreateComment(cm *models.Comment) error {
	return withDeadlockRetry(func() error { return createCommentOnce(cm) })
}

func createCommentOnce(cm *models.Comment) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	targetAuthor, err := lockCommentTarget(tx, cm.TargetType, cm.TargetID)
	if err != nil {
		return err
	}

	parentAuthor := int64(0)
	if cm.ParentID != 0 {
		// 锁定父评论行：回复数递增与父评论删除串行化，避免回复挂到已删除评论下
		if err = tx.Get(&parentAuthor, "SELECT user_id FROM comment WHERE comment_id = ? AND "+commentVisibleSQL("comment")+" FOR UPDATE",
			cm.ParentID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrCommentNotExist
			}
			return err
		}
	}

	if _, err = tx.Exec(
		"INSERT INTO comment (comment_id, user_id, target_type, target_id, parent_id, reply_to_user_id, content) VALUES (?, ?, ?, ?, ?, ?, ?)",
		cm.CommentID, cm.UserID, cm.TargetType, cm.TargetID, cm.ParentID, cm.ReplyToUserID, cm.Content); err != nil {
		return err
	}
	if err = recalcTargetCommentCount(tx, cm.TargetType, cm.TargetID); err != nil {
		return err
	}
	if cm.ParentID != 0 {
		if _, err = tx.Exec("UPDATE comment SET reply_count = reply_count + 1 WHERE comment_id = ?", cm.ParentID); err != nil {
			return err
		}
	}

	if cm.ParentID == 0 {
		if err := txCreateNotification(tx, targetAuthor, cm.UserID, models.NotifyTypeComment, cm.TargetType, cm.TargetID); err != nil {
			return err
		}
	} else {
		// 楼中楼：一级评论作者收到「回复」通知；回复的对象是另一条回复时，被回复人也收到
		if err := txCreateNotification(tx, parentAuthor, cm.UserID, models.NotifyTypeReply, cm.TargetType, cm.TargetID); err != nil {
			return err
		}
		if cm.ReplyToUserID != 0 && cm.ReplyToUserID != parentAuthor {
			if err := txCreateNotification(tx, cm.ReplyToUserID, cm.UserID, models.NotifyTypeReply, cm.TargetType, cm.TargetID); err != nil {
				return err
			}
		}
		if parentAuthor != targetAuthor && cm.ReplyToUserID != targetAuthor {
			if err := txCreateNotification(tx, targetAuthor, cm.UserID, models.NotifyTypeComment, cm.TargetType, cm.TargetID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// commentState 评论当前状态（事务内加锁读取）
type commentState struct {
	UserID    int64        `db:"user_id"`
	ParentID  int64        `db:"parent_id"`
	TargetID  int64        `db:"target_id"`
	Status    int32        `db:"status"`
	DeletedAt sql.NullTime `db:"deleted_at"`
}

// SoftDeleteComment 作者（或管理员/审核员）软删除评论，并重算目标评论数与父评论回复数
// 仅删除该条评论本身，其下回复保留（列表中以「该评论已删除」占位展示）；重复删除返回 ErrCommentNotExist。
func SoftDeleteComment(commentID, userID int64, privileged bool) error {
	return withDeadlockRetry(func() error { return softDeleteCommentOnce(commentID, userID, privileged) })
}

func softDeleteCommentOnce(commentID, userID int64, privileged bool) error {
	// 先无锁读取定位所属内容，再按「内容行 → 评论行」的统一锁顺序加锁
	locate := new(struct {
		TargetType int32 `db:"target_type"`
		TargetID   int64 `db:"target_id"`
	})
	err := db.Get(locate, "SELECT target_type, target_id FROM comment WHERE comment_id = ?", commentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCommentNotExist
		}
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = lockCommentTarget(tx, locate.TargetType, locate.TargetID); err != nil {
		return err
	}
	state := new(commentState)
	err = tx.Get(state, "SELECT user_id, parent_id, target_id, status, deleted_at FROM comment WHERE comment_id = ? FOR UPDATE", commentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCommentNotExist
		}
		return err
	}
	if state.DeletedAt.Valid || state.Status != 1 {
		return ErrCommentNotExist
	}
	if state.UserID != userID && !privileged {
		return ErrCommentForbidden
	}
	if _, err = tx.Exec("UPDATE comment SET status = 0, deleted_at = NOW(), updated_by = ? WHERE comment_id = ?",
		userID, commentID); err != nil {
		return err
	}
	// 重算所属内容评论数
	if err = recalcTargetCommentCount(tx, locate.TargetType, locate.TargetID); err != nil {
		return err
	}
	// 回复被删除：父评论回复数减一
	if state.ParentID != 0 {
		var visible int
		if err = tx.Get(&visible, "SELECT COUNT(*) FROM comment WHERE parent_id = ? AND "+commentVisibleSQL("comment"), state.ParentID); err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE comment SET reply_count = ? WHERE comment_id = ?", visible, state.ParentID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetCommentLike 点赞 / 取消点赞评论（幂等）；点赞数以 comment_like 为准在事务内重算
// 评论不可见（已删除 / 未过审 / 已封禁作者）时视为不存在。
func SetCommentLike(userID, commentID int64, like bool) error {
	return withDeadlockRetry(func() error { return setCommentLikeOnce(userID, commentID, like) })
}

func setCommentLikeOnce(userID, commentID int64, like bool) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	st := new(struct {
		UserID     int64        `db:"user_id"`
		TargetType int32        `db:"target_type"`
		TargetID   int64        `db:"target_id"`
		Status     int32        `db:"status"`
		Moderation int32        `db:"moderation_status"`
		DeletedAt  sql.NullTime `db:"deleted_at"`
	})
	if err = tx.Get(st, "SELECT user_id, target_type, target_id, status, moderation_status, deleted_at "+
		"FROM comment WHERE comment_id = ? FOR UPDATE", commentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCommentNotExist
		}
		return err
	}
	if st.Status != 1 || st.Moderation != 1 || st.DeletedAt.Valid {
		return ErrCommentNotExist
	}

	var result sql.Result
	if like {
		result, err = tx.Exec("INSERT IGNORE INTO comment_like (user_id, comment_id) VALUES (?, ?)", userID, commentID)
	} else {
		result, err = tx.Exec("DELETE FROM comment_like WHERE user_id = ? AND comment_id = ?", userID, commentID)
	}
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n > 0 {
		if _, err = tx.Exec("UPDATE comment SET like_count = (SELECT COUNT(*) FROM comment_like WHERE comment_id = ?) "+
			"WHERE comment_id = ?", commentID, commentID); err != nil {
			return err
		}
		// 首次点赞才通知评论作者（自己赞自己不通知，由 txCreateNotification 保证）
		if like {
			if err := txCreateNotification(tx, st.UserID, userID, models.NotifyTypeCommentLike, st.TargetType, st.TargetID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// UserLikedCommentIDs 批量查询当前用户已点赞的评论 ID（列表组装用）
func UserLikedCommentIDs(userID int64, commentIDs []int64) (map[int64]bool, error) {
	liked := make(map[int64]bool, len(commentIDs))
	if userID <= 0 || len(commentIDs) == 0 {
		return liked, nil
	}
	query, args, err := sqlx.In("SELECT comment_id FROM comment_like WHERE user_id = ? AND comment_id IN (?)", userID, commentIDs)
	if err != nil {
		return nil, err
	}
	var ids []int64
	if err = db.Select(&ids, query, args...); err != nil {
		return nil, err
	}
	for _, id := range ids {
		liked[id] = true
	}
	return liked, nil
}

// UserRole 查询用户当前角色（每次查库，角色变更即时生效；用户不存在时返回空串）
func UserRole(userID int64) (string, error) {
	if userID <= 0 {
		return "", nil
	}
	u, err := GetUserByID(userID)
	if err != nil {
		return "", err
	}
	if u.Role == "" {
		return models.RoleUser, nil
	}
	return u.Role, nil
}

// AnswerQuestionIDs 批量查询回答所属的问题 ID（通知跳转用：回答通知的 target_id 是回答 ID）
func AnswerQuestionIDs(answerIDs []int64) (map[int64]int64, error) {
	res := make(map[int64]int64, len(answerIDs))
	if len(answerIDs) == 0 {
		return res, nil
	}
	query, args, err := sqlx.In("SELECT answer_id, question_id FROM answer WHERE answer_id IN (?)", answerIDs)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		AnswerID   int64 `db:"answer_id"`
		QuestionID int64 `db:"question_id"`
	}
	if err = db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		res[r.AnswerID] = r.QuestionID
	}
	return res, nil
}
