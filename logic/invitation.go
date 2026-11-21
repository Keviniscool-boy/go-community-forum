package logic

// 回答邀请业务层（待回答问题发现与回答邀请 PRD §7）
// 权限在 DAO 事务内以当前数据库状态校验；这里补充输入整形、游标编解码与错误映射。

import (
	"errors"
	"strconv"
	"time"

	"bluebell/dao/mysql"
	"bluebell/models"
)

// 邀请业务错误（controller 映射稳定业务码；项目约定业务错误走 HTTP 200 + envelope）
var (
	ErrInviteLimit   = errors.New("邀请数已达上限")
	ErrInviteStateOp = errors.New("邀请已处理或已失效")
	ErrInviteReceiver = errors.New("仅邀请接收者可以操作")
)

// InviteeResult 邀请发送结果（PRD §6.2：部分失败逐人说明）
type InviteeResult struct {
	InviteeID    string `json:"invitee_id,string"`
	InvitationID string `json:"invitation_id,omitempty"`
	OK           bool   `json:"ok"`
	Reason       string `json:"reason,omitempty"`
}

// SendAnswerInvitations 提问者批量邀请（1-5 人）
func SendAnswerInvitations(inviterID, questionID int64, p *models.ParamSendAnswerInvitations, requestID string) ([]InviteeResult, error) {
	if len(p.InviteeIDs) == 0 || len(p.InviteeIDs) > models.InviteBatchMax {
		return nil, mysql.ErrInvalidParams
	}
	inviteeIDs := make([]int64, 0, len(p.InviteeIDs))
	seen := make(map[int64]struct{}, len(p.InviteeIDs))
	for _, s := range p.InviteeIDs {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return nil, mysql.ErrInvalidParams
		}
		if _, dup := seen[id]; dup {
			continue // 去重：同一人重复出现在同一次请求只邀请一次
		}
		seen[id] = struct{}{}
		inviteeIDs = append(inviteeIDs, id)
	}
	if len(inviteeIDs) == 0 {
		return nil, mysql.ErrInvalidParams
	}
	results, err := mysql.SendAnswerInvitations(inviterID, questionID, inviteeIDs, requestID)
	if err != nil {
		return nil, mapInviteError(err)
	}
	out := make([]InviteeResult, 0, len(results))
	for _, r := range results {
		out = append(out, InviteeResult{
			InviteeID:    strconv.FormatInt(r.InviteeID, 10),
			InvitationID: r.InvitationID,
			OK:           r.OK,
			Reason:       r.Reason,
		})
	}
	return out, nil
}

// ListUnansweredQuestions 待回答问题列表（公开；sort=latest|oldest_unanswered；keyset 游标）
func ListUnansweredQuestions(topicID int64, sort, cursor string, limit int) (*models.PageResult[*models.Question], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if sort != "latest" && sort != "oldest_unanswered" {
		sort = "latest"
	}
	type unansweredCursor struct {
		Time time.Time `json:"t"`
		ID   int64     `json:"i"`
	}
	var cursorTime time.Time
	var cursorID int64
	if cursor != "" {
		c := new(unansweredCursor)
		raw, err := base64Decode(cursor)
		if err != nil || unmarshalJSONMust(raw, c) != nil || c.ID <= 0 {
			return nil, ErrCursorInvalid
		}
		cursorTime, cursorID = c.Time, c.ID
	}
	items, err := mysql.ListUnansweredQuestions(&mysql.UnansweredPageParams{
		TopicID:    topicID,
		Sort:       sort,
		CursorTime: cursorTime,
		CursorID:   cursorID,
		Limit:      limit,
	})
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
		next = base64Encode(marshalJSONMust(&unansweredCursor{Time: last.CreateTime, ID: last.ID}))
	}
	return &models.PageResult[*models.Question]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// ListInviteCandidates 邀请候选（仅问题作者；最多 20 人单页）
func ListInviteCandidates(inviterID, questionID int64, keyword string) ([]*models.ApiCandidateUser, error) {
	// 先确认是本人公开且待回答的问题（越权统一不可见）
	q, err := mysql.GetQuestionByID(questionID)
	if err != nil {
		if errors.Is(err, mysql.ErrQuestionNotExist) {
			return nil, ErrQuestionNotExist
		}
		return nil, err
	}
	if q.AuthorID != inviterID {
		return nil, ErrForbidden
	}
	if q.ResolutionStatus != models.QuestionResolutionOpen {
		return nil, ErrInviteStateOp
	}
	return mysql.ListInviteCandidates(questionID, inviterID, keyword, 20)
}

// ListQuestionInvitations 问题作者的邀请记录（最近 20 条）
func ListQuestionInvitations(inviterID, questionID int64) ([]*models.AnswerInvitation, error) {
	q, err := mysql.GetQuestionByID(questionID)
	if err != nil {
		if errors.Is(err, mysql.ErrQuestionNotExist) {
			return nil, ErrQuestionNotExist
		}
		return nil, err
	}
	if q.AuthorID != inviterID {
		return nil, ErrForbidden
	}
	return mysql.ListInvitationsForQuestion(questionID, 20)
}

// ListMyAnswerInvitations 我收到的邀请（cursor 分页；返回前把过期 pending 惰性转 expired）
func ListMyAnswerInvitations(inviteeID int64, statusFilter string, cursor string, limit int) (*models.PageResult[*models.AnswerInvitation], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var filter *int32
	switch statusFilter {
	case "pending":
		f := models.InviteStatusPending
		filter = &f
	case "accepted":
		f := models.InviteStatusAccepted
		filter = &f
	case "answered":
		f := models.InviteStatusAnswered
		filter = &f
	case "declined":
		f := models.InviteStatusDeclined
		filter = &f
	case "expired":
		f := models.InviteStatusExpired
		filter = &f
	case "":
	default:
		return nil, mysql.ErrInvalidParams
	}
	type inviteCursor struct {
		Time time.Time `json:"t"`
		ID   int64     `json:"i"`
	}
	var cursorTime time.Time
	var cursorID int64
	if cursor != "" {
		c := new(inviteCursor)
		raw, err := base64Decode(cursor)
		if err != nil || unmarshalJSONMust(raw, c) != nil || c.ID <= 0 {
			return nil, ErrCursorInvalid
		}
		cursorTime, cursorID = c.Time, c.ID
	}
	items, err := mysql.ListInvitationsForInvitee(inviteeID, filter, cursorTime, cursorID, limit)
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
		next = base64Encode(marshalJSONMust(&inviteCursor{Time: last.CreateTime, ID: last.InvitationID}))
	}
	return &models.PageResult[*models.AnswerInvitation]{
		Items:      items,
		NextCursor: next,
		HasMore:    hasMore,
	}, nil
}

// RespondAnswerInvitation 接受/婉拒（仅接收者；幂等；过期/失效返回状态错误）
func RespondAnswerInvitation(inviteeID, invitationID int64, decision string, requestID string) error {
	var to int32
	switch decision {
	case "accept":
		to = models.InviteStatusAccepted
	case "decline":
		to = models.InviteStatusDeclined
	default:
		return mysql.ErrInvalidParams
	}
	err := mysql.RespondInvitation(invitationID, inviteeID, to, requestID)
	return mapInviteError(err)
}

// GetAnswerInvitePreference / SetAnswerInvitePreference 接收偏好
func GetAnswerInvitePreference(userID int64) (bool, error) {
	return mysql.GetAnswerInvitePreference(userID)
}

func SetAnswerInvitePreference(userID int64, enabled bool) error {
	return mysql.SetAnswerInvitePreference(userID, enabled)
}

// mapInviteError DAO 哨兵 → 业务错误
func mapInviteError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, mysql.ErrInviteState):
		return ErrInviteStateOp
	case errors.Is(err, mysql.ErrInviteNotReceiver):
		return ErrInviteReceiver
	case errors.Is(err, mysql.ErrInviteQuestionState):
		return ErrInviteStateOp
	case errors.Is(err, mysql.ErrNotQuestionAuthor):
		return ErrForbidden
	case errors.Is(err, mysql.ErrQuestionNotExist):
		return ErrQuestionNotExist
	case errors.Is(err, mysql.ErrInvitationNotExist):
		return ErrInviteStateOp
	case errors.Is(err, mysql.ErrUserNotExist):
		return ErrForbidden
	}
	return err
}
