package logic

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"bluebell/dao/mysql"
	"bluebell/models"
	"bluebell/pkg/snowflake"
)

// ---------- 协作创作 ----------

var (
	ErrRevisionConflict = errors.New("版本冲突，请刷新后重试")
	ErrSeriesNotExist   = errors.New("系列不存在")
	ErrNotColumnMember  = errors.New("不是专栏成员")
	ErrOwnerOnly        = errors.New("仅专栏所有者可执行")
	ErrSelfApproval     = errors.New("不能批准自己提交的版本")
	ErrInvitationExpired = errors.New("邀请已过期")
	ErrCannotInviteSelf = errors.New("不能邀请自己")
)

// requireColumnRole 校验用户在专栏的角色（owner 覆盖 editor/reviewer 能力）
// 返回实际角色；非成员返回空串
func requireColumnRole(columnID, userID int64, allowed ...string) (string, error) {
	role, err := mysql.GetColumnMemberRole(columnID, userID)
	if err != nil {
		return "", err
	}
	if role == "" {
		return "", ErrNotColumnMember
	}
	if role == models.ColumnRoleOwner {
		return role, nil
	}
	for _, a := range allowed {
		if a == role {
			return role, nil
		}
	}
	return role, ErrForbidden
}

// CreateSeries 创建系列（专栏所有者）
func CreateSeries(ownerID int64, columnID int64, p *models.ParamCreateSeries) (seriesID int64, err error) {
	if _, err = requireColumnRole(columnID, ownerID, models.ColumnRoleOwner); err != nil {
		return 0, err
	}
	_, err = CheckContentModeration(p.Title, p.Description)
	if err != nil {
		return 0, err // fail-closed
	}
	s := &models.Series{
		SeriesID:    snowflake.GenerateID(),
		ColumnID:    columnID,
		Title:       p.Title,
		Description: p.Description,
	}
	if err = mysql.CreateSeries(s); err != nil {
		return 0, err
	}
	return s.SeriesID, nil
}

// GetSeriesDetail 系列详情 + 可见目录
func GetSeriesDetail(seriesID int64, viewerID int64) (*models.Series, []map[string]interface{}, error) {
	s, err := mysql.GetSeriesByID(seriesID)
	if err != nil {
		return nil, nil, err
	}
	items, err := mysql.ListSeriesItems(seriesID)
	if err != nil {
		return nil, nil, err
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		// 读者只看到有权访问的文章（visibility/作者禁用过滤）
		a, err := mysql.GetArticlePublicByID(it.ArticleID, viewerID)
		available := err == nil
		out = append(out, map[string]interface{}{
			"article_id": strconv.FormatInt(it.ArticleID, 10),
			"position":   it.Position,
			"title":      it.Title,
			"available":  available,
		})
		_ = a
	}
	return s, out, nil
}

// UpdateSeriesMeta 编辑系列（编辑及以上，版本乐观锁）
func UpdateSeriesMeta(userID int64, seriesID int64, p *models.ParamUpdateSeries) error {
	s, err := mysql.GetSeriesByID(seriesID)
	if err != nil {
		return err
	}
	if _, err = requireColumnRole(s.ColumnID, userID, models.ColumnRoleOwner, models.ColumnRoleEditor); err != nil {
		return err
	}
	_, err = mysql.UpdateSeries(seriesID, p.Title, p.Description, p.Version)
	return err
}

// SetSeriesOrder 设置目录顺序（编辑及以上）
func SetSeriesOrder(userID int64, seriesID int64, p *models.ParamSeriesOrder) error {
	s, err := mysql.GetSeriesByID(seriesID)
	if err != nil {
		return err
	}
	if _, err = requireColumnRole(s.ColumnID, userID, models.ColumnRoleOwner, models.ColumnRoleEditor); err != nil {
		return err
	}
	if s.Version != p.Version {
		return mysql.ErrSeriesOrderConflict
	}
	ids := make([]int64, 0, len(p.ArticleIDs))
	for _, strID := range p.ArticleIDs {
		id, err := strconv.ParseInt(strID, 10, 64)
		if err != nil || id <= 0 {
			return ErrArticleInvalid
		}
		ids = append(ids, id)
	}
	if err = mysql.ReplaceSeriesItems(seriesID, ids); err != nil {
		return err
	}
	return nil
}

// ListSeriesForColumn 专栏系列列表
func ListSeriesForColumn(columnID int64) ([]*models.Series, error) {
	return mysql.ListSeriesByColumn(columnID)
}

// ---------- 成员与邀请 ----------

// InviteMember 所有者邀请成员（7 天有效；幂等返回已有待处理邀请）
func InviteMember(ownerID, columnID int64, p *models.ParamInviteMember, requestID string) (duplicated bool, err error) {
	role, err := requireColumnRole(columnID, ownerID, models.ColumnRoleOwner)
	if err != nil {
		return false, err
	}
	_ = role
	inviteeID, err := strconv.ParseInt(p.UserID, 10, 64)
	if err != nil || inviteeID <= 0 || inviteeID == ownerID {
		return false, ErrCannotInviteSelf
	}
	// 被邀请人存在且未禁用
	invitee, err := mysql.GetUserByID(inviteeID)
	if err != nil || invitee.Status == 0 {
		return false, ErrCannotContact
	}
	inv := &models.ColumnInvitation{
		InvitationID: snowflake.GenerateID(),
		ColumnID:     columnID,
		InviterID:    ownerID,
		InviteeID:    inviteeID,
		Role:         p.Role,
		ExpiresAt:    time.Now().UTC().Add(7 * 24 * time.Hour),
	}
	dup, err := mysql.CreateColumnInvitation(inv)
	if err != nil {
		return false, err
	}
	_ = mysql.CreateColumnAudit(ownerID, columnID, "member_invite", strconv.FormatInt(inviteeID, 10), requestID)
	return dup, nil
}

// AcceptInvitation 接受邀请（仅被邀请人；检查有效期与屏蔽）
func AcceptInvitation(userID, invitationID int64, requestID string) error {
	inv, err := mysql.GetColumnInvitation(invitationID)
	if err != nil {
		return err
	}
	if inv.InviteeID != userID {
		return ErrNotColumnMember
	}
	if inv.Status != models.InvitationPending {
		return nil // 幂等
	}
	if time.Now().UTC().After(inv.ExpiresAt) {
		return ErrInvitationExpired
	}
	// 屏蔽联动：任一方屏蔽则不可接受
	b1, _ := mysql.IsBlocked(inv.InviterID, userID)
	b2, _ := mysql.IsBlocked(userID, inv.InviterID)
	if b1 || b2 {
		return ErrCannotContact
	}
	ok, err := mysql.ResolveColumnInvitation(invitationID, models.InvitationPending, models.InvitationAccepted)
	if err != nil {
		return err
	}
	if ok {
		if err = mysql.AddColumnMember(inv.ColumnID, userID, inv.Role); err != nil {
			return err
		}
		_ = mysql.CreateColumnAudit(userID, inv.ColumnID, "member_accept", strconv.FormatInt(userID, 10), requestID)
	}
	return nil
}

// DeclineInvitation 拒绝邀请
func DeclineInvitation(userID, invitationID int64) error {
	inv, err := mysql.GetColumnInvitation(invitationID)
	if err != nil {
		return err
	}
	if inv.InviteeID != userID {
		return ErrNotColumnMember
	}
	_, err = mysql.ResolveColumnInvitation(invitationID, models.InvitationPending, models.InvitationDeclined)
	return nil
}

// RemoveMember 移除成员（所有者或本人退出）
func RemoveMember(actorID, columnID, targetUserID int64, requestID string) error {
	role, err := mysql.GetColumnMemberRole(columnID, actorID)
	if err != nil {
		return err
	}
	if role == "" {
		return ErrNotColumnMember
	}
	if actorID != targetUserID && role != models.ColumnRoleOwner {
		return ErrOwnerOnly
	}
	if err = mysql.RemoveColumnMember(columnID, targetUserID); err != nil {
		return err
	}
	action := "member_remove"
	if actorID == targetUserID {
		action = "member_leave"
	}
	_ = mysql.CreateColumnAudit(actorID, columnID, action, strconv.FormatInt(targetUserID, 10), requestID)
	return nil
}

// ListMembers 成员列表（仅成员可见）
func ListMembers(columnID int64, viewerID int64) ([]*models.ColumnMember, error) {
	role, err := mysql.GetColumnMemberRole(columnID, viewerID)
	if err != nil {
		return nil, err
	}
	if role == "" {
		// 非成员至少可以看公开专栏的成员列表？规格未明确；v1 仅成员可见
		return nil, ErrNotColumnMember
	}
	return mysql.ListColumnMembers(columnID)
}

// ---------- 版本历史 ----------

// SaveRevision 保存文章新版本（编辑调用；内容哈希幂等）
func SaveRevision(userID int64, articleID int64, title, body string) (*models.ArticleRevision, error) {
	a, err := mysql.GetArticleByID(articleID)
	if err != nil {
		return nil, err
	}
	if a.AuthorID != userID {
		return nil, ErrForbidden
	}
	sum := sha256.Sum256([]byte(title + "\x00" + body))
	hash := hex.EncodeToString(sum[:])
	maxNo, err := mysql.GetMaxRevisionNo(articleID)
	if err != nil {
		return nil, err
	}
	rev := &models.ArticleRevision{
		RevisionID:  snowflake.GenerateID(),
		ArticleID:   articleID,
		RevisionNo:  maxNo + 1,
		Title:       title,
		Body:        body,
		AuthorID:    userID,
		ContentHash: hash,
	}
	// 幂等：与上一版本内容相同则复用
	if maxNo > 0 {
		last, err := mysql.GetArticleRevisionByNo(articleID, maxNo)
		if err == nil && last.ContentHash == hash {
			return last, nil
		}
	}
	if err = mysql.CreateArticleRevision(rev); err != nil {
		return nil, err
	}
	return rev, nil
}

// ListRevisions 版本列表（仅作者/成员）
func ListRevisions(userID, articleID int64) ([]*models.ArticleRevision, error) {
	a, err := mysql.GetArticleByID(articleID)
	if err != nil {
		return nil, err
	}
	if a.AuthorID != userID {
		return nil, ErrForbidden
	}
	return mysql.ListArticleRevisions(articleID, 100)
}

// RestoreRevision 恢复历史版本为新草稿（B03：baseVersion CAS 校验，
// 与当前最新版本号不一致说明存在并发保存，拒绝并让用户手动合并，绝不静默覆盖）
func RestoreRevision(userID, articleID, revisionID int64, baseVersion int64) (*models.ArticleRevision, error) {
	a, err := mysql.GetArticleByID(articleID)
	if err != nil {
		return nil, err
	}
	if a.AuthorID != userID {
		return nil, ErrForbidden
	}
	rev, err := mysql.GetArticleRevision(revisionID)
	if err != nil || rev.ArticleID != articleID {
		return nil, mysql.ErrArticleNotExist
	}
	// B03 CAS：baseVersion 必须等于当前最新版本号
	maxNo, err := mysql.GetMaxRevisionNo(articleID)
	if err != nil {
		return nil, err
	}
	if baseVersion != maxNo {
		return nil, ErrRevisionConflict
	}
	// 恢复 = 以旧内容创建新版本（追加，不覆盖历史）
	return SaveRevision(userID, articleID, rev.Title, rev.Body)
}

// ---------- 审阅 ----------

// SubmitReview 提交审阅（编辑；绑定不可变版本）
func SubmitReview(userID, articleID int64, revisionIDStr string) (reviewID int64, err error) {
	a, err := mysql.GetArticleByID(articleID)
	if err != nil {
		return 0, err
	}
	if _, err = requireColumnRoleForArticle(a, userID, models.ColumnRoleOwner, models.ColumnRoleEditor); err != nil {
		return 0, err
	}
	revisionID, err := strconv.ParseInt(revisionIDStr, 10, 64)
	if err != nil || revisionID <= 0 {
		return 0, ErrArticleInvalid
	}
	rev, err := mysql.GetArticleRevision(revisionID)
	if err != nil || rev.ArticleID != articleID {
		return 0, ErrArticleInvalid
	}
	r := &models.ReviewRequest{
		ReviewID:    snowflake.GenerateID(),
		ArticleID:   articleID,
		RevisionID:  revisionID,
		InitiatorID: userID,
	}
	if err = mysql.CreateReviewRequest(r); err != nil {
		return 0, err
	}
	return r.ReviewID, nil
}

// DecideReview 审阅决定（批准/退回；禁自批；版本绑定）
func DecideReview(userID, reviewID int64, p *models.ParamReviewDecision) error {
	r, err := mysql.GetReviewByID(reviewID)
	if err != nil {
		return err
	}
	if r.State != 0 {
		return ErrStateNotAllowed
	}
	a, err := mysql.GetArticleByID(r.ArticleID)
	if err != nil {
		return err
	}
	// 审阅人 = 专栏 reviewer/editor/owner，但不能是提交者
	if _, err = requireColumnRoleForArticle(a, userID, models.ColumnRoleOwner, models.ColumnRoleEditor, models.ColumnRoleReviewer); err != nil {
		return err
	}
	if r.InitiatorID == userID {
		return ErrSelfApproval
	}
	revID, err := strconv.ParseInt(p.Revision, 10, 64)
	if err != nil || revID != r.RevisionID {
		return ErrArticleInvalid // 版本绑定不匹配
	}
	// B04：审阅决定、pending 标记清理与文章协作状态在同一事务（原为两个调用，故障时状态分叉）
	var decision int32 = 2 // changes_requested
	if p.Decision == "approve" {
		decision = int32(1)
	}
	var newCollab int32 = models.CollabChangesRequested
	if decision == 1 {
		newCollab = models.CollabApproved
	}
	ok, err := mysql.SetReviewDecisionAtomic(reviewID, revID, decision, r.ArticleID, newCollab)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAdminConflict
	}
	return nil
}

// requireColumnRoleForArticle 文章所属专栏的角色校验（无专栏文章只允许作者本人=owner 视角）
func requireColumnRoleForArticle(a *models.Article, userID int64, allowed ...string) (string, error) {
	if a.ColumnID <= 0 {
		// 无专栏文章：作者本人视为 owner
		if a.AuthorID == userID {
			return models.ColumnRoleOwner, nil
		}
		return "", ErrNotColumnMember
	}
	return requireColumnRole(a.ColumnID, userID, allowed...)
}

// ListReviewCommentsFor 成员查看审阅评论
func ListReviewCommentsFor(userID, reviewID int64) ([]*models.ReviewComment, error) {
	r, err := mysql.GetReviewByID(reviewID)
	if err != nil {
		return nil, err
	}
	a, err := mysql.GetArticleByID(r.ArticleID)
	if err != nil {
		return nil, err
	}
	if a.AuthorID != userID {
		if _, err = requireColumnRoleForArticle(a, userID, models.ColumnRoleOwner, models.ColumnRoleEditor, models.ColumnRoleReviewer); err != nil {
			return nil, err
		}
	}
	return mysql.ListReviewComments(reviewID)
}

// AddReviewComment 审阅评论（成员）
func AddReviewComment(userID, reviewID int64, p *models.ParamReviewComment) error {
	r, err := mysql.GetReviewByID(reviewID)
	if err != nil {
		return err
	}
	a, err := mysql.GetArticleByID(r.ArticleID)
	if err != nil {
		return err
	}
	if _, err = requireColumnRoleForArticle(a, userID, models.ColumnRoleOwner, models.ColumnRoleEditor, models.ColumnRoleReviewer); err != nil {
		return err
	}
	cm := &models.ReviewComment{
		CommentID: snowflake.GenerateID(),
		ReviewID:  reviewID,
		AuthorID:  userID,
		Body:      p.Body,
	}
	return mysql.CreateReviewComment(cm)
}
