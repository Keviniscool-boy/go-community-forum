package logic

// 问答解决闭环业务逻辑（产品审计 PRD §4）
// 权限在 DAO 事务内以当前数据库状态校验（作者身份/回答可见性），这里补充角色判定与错误映射。

import (
	"errors"
	"time"

	"bluebell/dao/mysql"
	"bluebell/models"
)

// 采纳/关闭流程的业务错误（controller 映射稳定错误码）
var (
	ErrQuestionStateConflict = errors.New("问题状态已变化，请刷新后重试")
	ErrAnswerCannotAccept    = errors.New("该回答不可采纳")
	ErrQuestionClosedOp      = errors.New("问题已关闭，无法采纳回答")
	ErrQuestionTransition    = errors.New("该状态转换不允许")
)

// isAdminUser 判断用户是否管理员（角色每次查库即时生效，与 RequireRole 语义一致）
func isAdminUser(userID int64) (bool, error) {
	u, err := mysql.GetUserByID(userID)
	if err != nil {
		return false, err
	}
	return u.Role == "admin", nil
}

// AcceptAnswer 提问者采纳回答（原子事务：锁定问题 → 校验 → CAS 更新 → 通知/审计）
func AcceptAnswer(userID, questionID, answerID int64, requestID string) error {
	err := mysql.AcceptAnswerAtomic(questionID, answerID, userID, requestID)
	return mapResolutionError(err)
}

// UnacceptAnswer 提问者撤销采纳（幂等）
func UnacceptAnswer(userID, questionID, answerID int64, requestID string) error {
	err := mysql.UnacceptAnswerAtomic(questionID, answerID, userID, requestID)
	return mapResolutionError(err)
}

// SetQuestionStatus 作者关闭/重开问题（仅 open/closed 转换；solved 由采纳操作维护）
func SetQuestionStatus(userID, questionID int64, status string, requestID string) error {
	from, to, err := resolveTransition(status)
	if err != nil {
		return err
	}
	// 角色查询失败或用户不存在时一律按非管理员处理，由事务内的作者校验给出最终判定
	admin, _ := isAdminUser(userID)
	err = mysql.SetQuestionResolutionStatusAtomic(questionID, userID, from, to, admin, requestID)
	return mapResolutionError(err)
}

// resolveTransition 字符串状态 → (from, to) 转换对；solved 不允许直接设置
func resolveTransition(status string) (from, to int32, err error) {
	switch status {
	case "closed":
		return models.QuestionResolutionOpen, models.QuestionResolutionClosed, nil
	case "open":
		return models.QuestionResolutionClosed, models.QuestionResolutionOpen, nil
	default:
		return 0, 0, ErrQuestionTransition
	}
}

// mapResolutionError DAO 哨兵错误 → 业务错误（保持 errors.Is 链可判定）
func mapResolutionError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, mysql.ErrResolutionConflict):
		return ErrQuestionStateConflict
	case errors.Is(err, mysql.ErrAnswerNotAcceptable):
		return ErrAnswerCannotAccept
	case errors.Is(err, mysql.ErrQuestionClosed):
		return ErrQuestionClosedOp
	case errors.Is(err, mysql.ErrResolutionTransitionNotAllowed):
		return ErrQuestionTransition
	case errors.Is(err, mysql.ErrNotQuestionAuthor):
		return ErrForbidden
	case errors.Is(err, mysql.ErrQuestionNotExist):
		return ErrQuestionNotExist
	}
	return err
}

// ---------- 我的内容管理（GET /me/content） ----------

type myContentCursor struct {
	Time time.Time `json:"t"`
	ID   int64     `json:"i"`
}

// ListMyContent 当前用户的内容管理列表（type+status 过滤，cursor 分页；默认 20 最大 50）
func ListMyContent(userID int64, p *models.ParamMyContent) (*models.PageResult[*models.ApiMyContentItem], error) {
	limit := p.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var cursorTime time.Time
	var cursorID int64
	if p.Cursor != "" {
		c := new(myContentCursor)
		raw, err := base64Decode(p.Cursor)
		if err != nil || unmarshalJSONMust(raw, c) != nil || c.ID <= 0 {
			return nil, ErrCursorInvalid
		}
		cursorTime, cursorID = c.Time, c.ID
	}
	items, err := mysql.ListUserContent(userID, p.Type, p.Status, cursorTime, cursorID, limit)
	if err != nil {
		return nil, err
	}
	hasMore := false
	if len(items) > limit {
		hasMore = true
		items = items[:limit]
	}
	nextCursor := ""
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = base64Encode(marshalJSONMust(&myContentCursor{Time: last.CreateTime, ID: last.ContentID}))
	}
	return &models.PageResult[*models.ApiMyContentItem]{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
