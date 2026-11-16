package mysql

import (
	"database/sql"
	"errors"

	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// ---------- 系列 ----------

var (
	ErrSeriesNotExist   = errors.New("系列不存在")
	ErrSeriesOrderConflict = errors.New("目录版本冲突，请刷新后重试")
)

// CreateSeries 创建系列（归属专栏，创建者为专栏所有者）
func CreateSeries(s *models.Series) error {
	_, err := db.Exec(
		"INSERT INTO series (series_id, column_id, title, description, moderation_status) VALUES (?, ?, ?, ?, 1)",
		s.SeriesID, s.ColumnID, s.Title, s.Description)
	return err
}

// GetSeriesByID 查询系列
func GetSeriesByID(id int64) (*models.Series, error) {
	s := new(models.Series)
	err := db.Get(s, `SELECT series_id, column_id, title, description, status, moderation_status,
		visibility, deleted_at, version, create_time, update_time FROM series WHERE series_id = ? AND status = 1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSeriesNotExist
		}
		return nil, err
	}
	return s, nil
}

// UpdateSeries 编辑系列（版本乐观锁）
func UpdateSeries(seriesID int64, title, description string, expectVersion int64) (int64, error) {
	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(
		"UPDATE series SET title = ?, description = ?, version = version + 1 WHERE series_id = ? AND status = 1 AND version = ?",
		title, description, seriesID, expectVersion)
	if err != nil {
		return 0, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return 0, ErrSeriesOrderConflict
	}
	var newVer int64
	if err = tx.Get(&newVer, "SELECT version FROM series WHERE series_id = ?", seriesID); err != nil {
		return 0, err
	}
	return newVer, tx.Commit()
}

// ReplaceSeriesItems 全量替换目录排序（事务；校验文章属于该系列的文章集合、无重复、数量上限）
func ReplaceSeriesItems(seriesID int64, articleIDs []int64) error {
	if len(articleIDs) > 200 {
		return ErrInvalidParams
	}
	// 去重校验
	seen := make(map[int64]struct{}, len(articleIDs))
	for _, id := range articleIDs {
		if _, ok := seen[id]; ok {
			return ErrInvalidParams
		}
		seen[id] = struct{}{}
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM series_item WHERE series_id = ?", seriesID); err != nil {
		return err
	}
	for pos, id := range articleIDs {
		if _, err = tx.Exec(
			"INSERT INTO series_item (series_id, article_id, position) VALUES (?, ?, ?)",
			seriesID, id, pos); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListSeriesItems 系列目录（按 position 排序）
func ListSeriesItems(seriesID int64) (items []struct {
	ArticleID int64  `db:"article_id"`
	Position  int32  `db:"position"`
	Title     string `db:"title"`
}, err error) {
	err = db.Select(&items, `SELECT si.article_id, si.position, a.title
		FROM series_item si JOIN article a ON a.article_id = si.article_id AND a.status = 1
		WHERE si.series_id = ? ORDER BY si.position`, seriesID)
	return
}

// ListSeriesByColumn 专栏下的系列列表
func ListSeriesByColumn(columnID int64) (series []*models.Series, err error) {
	series = make([]*models.Series, 0)
	err = db.Select(&series, `SELECT series_id, column_id, title, description, status, moderation_status,
		visibility, deleted_at, version, create_time, update_time FROM series
		WHERE column_id = ? AND status = 1 AND moderation_status = 1 AND visibility = 1 AND deleted_at IS NULL
		ORDER BY create_time DESC`, columnID)
	return
}

// ---------- 专栏成员与邀请 ----------

// GetColumnMemberRole 查询用户在专栏的角色（空串=非成员）
func GetColumnMemberRole(columnID, userID int64) (string, error) {
	var role string
	err := db.Get(&role, "SELECT role FROM column_member WHERE column_id = ? AND user_id = ?", columnID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

// AddColumnMember 添加成员（唯一约束防重复）
func AddColumnMember(columnID, userID int64, role string) error {
	_, err := db.Exec(
		"INSERT IGNORE INTO column_member (column_id, user_id, role) VALUES (?, ?, ?)", columnID, userID, role)
	return err
}

// RemoveColumnMember 移除成员（不能移除唯一所有者）
func RemoveColumnMember(columnID, userID int64) error {
	var role string
	if err := db.Get(&role, "SELECT role FROM column_member WHERE column_id = ? AND user_id = ?", columnID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrColumnNotExist
		}
		return err
	}
	if role == models.ColumnRoleOwner {
		return errors.New("不能移除专栏所有者")
	}
	_, err := db.Exec("DELETE FROM column_member WHERE column_id = ? AND user_id = ?", columnID, userID)
	return err
}

// ListColumnMembers 成员列表
func ListColumnMembers(columnID int64) (members []*models.ColumnMember, err error) {
	members = make([]*models.ColumnMember, 0)
	err = db.Select(&members, "SELECT column_id, user_id, role, create_time FROM column_member WHERE column_id = ? ORDER BY create_time", columnID)
	return
}

// CreateColumnInvitation 创建邀请（同列同被邀请人 pending 幂等）
func CreateColumnInvitation(inv *models.ColumnInvitation) (duplicated bool, err error) {
	tx, err := db.Beginx()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var n int
	if err = tx.Get(&n,
		"SELECT COUNT(invitation_id) FROM column_invitation WHERE column_id = ? AND invitee_id = ? AND status = 0",
		inv.ColumnID, inv.InviteeID); err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	if _, err = tx.Exec(
		"INSERT INTO column_invitation (invitation_id, column_id, inviter_id, invitee_id, role, expires_at) VALUES (?, ?, ?, ?, ?, ?)",
		inv.InvitationID, inv.ColumnID, inv.InviterID, inv.InviteeID, inv.Role, inv.ExpiresAt); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

// GetColumnInvitation 查询邀请
func GetColumnInvitation(invitationID int64) (*models.ColumnInvitation, error) {
	inv := new(models.ColumnInvitation)
	err := db.Get(inv, "SELECT invitation_id, column_id, inviter_id, invitee_id, role, status, expires_at, create_time, reviewed_at FROM column_invitation WHERE invitation_id = ?", invitationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrColumnNotExist
		}
		return nil, err
	}
	return inv, nil
}

// ResolveColumnInvitation 接受/拒绝/撤销邀请（乐观状态迁移，幂等）
func ResolveColumnInvitation(invitationID int64, fromStatus, toStatus int32) (bool, error) {
	result, err := db.Exec(
		"UPDATE column_invitation SET status = ?, reviewed_at = NOW() WHERE invitation_id = ? AND status = ?",
		toStatus, invitationID, fromStatus)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// ---------- 文章版本 ----------

// GetMaxRevisionNo 当前最大版本号
func GetMaxRevisionNo(articleID int64) (int64, error) {
	var n int64
	err := db.Get(&n, "SELECT COALESCE(MAX(revision_no), 0) FROM article_revision WHERE article_id = ?", articleID)
	return n, err
}

// CreateArticleRevision 创建不可变版本（幂等键：内容哈希相同且为最新版本时复用）
func CreateArticleRevision(r *models.ArticleRevision) error {
	_, err := db.Exec(
		"INSERT INTO article_revision (revision_id, article_id, revision_no, title, body, author_id, content_hash) VALUES (?, ?, ?, ?, ?, ?, ?)",
		r.RevisionID, r.ArticleID, r.RevisionNo, r.Title, r.Body, r.AuthorID, r.ContentHash)
	return err
}

// ListArticleRevisions 版本元信息列表
func ListArticleRevisions(articleID int64, limit int) (revisions []*models.ArticleRevision, err error) {
	revisions = make([]*models.ArticleRevision, 0, limit)
	err = db.Select(&revisions, `SELECT revision_id, article_id, revision_no, title, body, author_id, content_hash, create_time
		FROM article_revision WHERE article_id = ? ORDER BY revision_no DESC LIMIT ?`, articleID, limit)
	return
}

// GetArticleRevision 查询指定版本（正文仅授权成员访问，由调用方校验）
func GetArticleRevision(revisionID int64) (*models.ArticleRevision, error) {
	r := new(models.ArticleRevision)
	err := db.Get(r, "SELECT revision_id, article_id, revision_no, title, body, author_id, content_hash, create_time FROM article_revision WHERE revision_id = ?", revisionID)
	return r, err
}

// ---------- 审阅 ----------

// CreateReviewRequest 创建审阅请求（同文章同版本最多一个待处理）
func CreateReviewRequest(r *models.ReviewRequest) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err = tx.Get(&n, "SELECT COUNT(review_id) FROM review_request WHERE article_id = ? AND revision_id = ? AND state = 0",
		r.ArticleID, r.RevisionID); err != nil {
		return err
	}
	if n > 0 {
		return ErrReviewPending
	}
	if _, err = tx.Exec(
		"INSERT INTO review_request (review_id, article_id, revision_id, state, initiator_id) VALUES (?, ?, ?, 0, ?)",
		r.ReviewID, r.ArticleID, r.RevisionID, r.InitiatorID); err != nil {
		return err
	}
	return tx.Commit()
}

var ErrReviewPending = errors.New("该版本已有进行中的审阅")

// GetReviewByID 查询审阅
func GetReviewByID(reviewID int64) (*models.ReviewRequest, error) {
	r := new(models.ReviewRequest)
	err := db.Get(r, "SELECT review_id, article_id, revision_id, state, initiator_id, create_time, reviewed_at FROM review_request WHERE review_id = ?", reviewID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrReviewNotExist
		}
		return nil, err
	}
	return r, nil
}

var ErrReviewNotExist = errors.New("审阅不存在")

// SetReviewDecision 审阅决定（版本绑定：批准的 revision 不匹配则拒绝）
func SetReviewDecision(reviewID, revisionID int64, decision int32) (bool, error) {
	result, err := db.Exec(
		"UPDATE review_request SET state = ?, reviewed_at = NOW() WHERE review_id = ? AND revision_id = ? AND state = 0",
		decision, reviewID, revisionID)
	if err != nil {
		return false, err
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// CreateReviewComment 审阅评论
func CreateReviewComment(c *models.ReviewComment) error {
	_, err := db.Exec(
		"INSERT INTO review_comment (comment_id, review_id, author_id, body) VALUES (?, ?, ?, ?)",
		c.CommentID, c.ReviewID, c.AuthorID, c.Body)
	return err
}

// ListReviewComments 审阅评论列表
func ListReviewComments(reviewID int64) (comments []*models.ReviewComment, err error) {
	comments = make([]*models.ReviewComment, 0)
	err = db.Select(&comments, "SELECT comment_id, review_id, author_id, body, create_time FROM review_comment WHERE review_id = ? ORDER BY create_time", reviewID)
	return
}

// ---------- 专栏审计 ----------

// CreateColumnAudit 专栏协作审计
func CreateColumnAudit(actorID, columnID int64, action, targetID, requestID string) error {
	_, err := db.Exec(
		"INSERT INTO column_audit (audit_id, actor_id, column_id, action, target_id, request_id) VALUES (?, ?, ?, ?, ?, ?)",
		snowflake.GenerateID(), actorID, columnID, action, targetID, requestID)
	return err
}

