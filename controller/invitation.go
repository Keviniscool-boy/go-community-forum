package controller

// 待回答问题发现与回答邀请接口（PRD §7）
// 错误语义沿用项目 envelope：业务错误 HTTP 200 + 稳定业务码（1045/1046 追加，不重排）。

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"bluebell/logic"
	"bluebell/models"
)

// respondInviteError 邀请域统一错误映射
func respondInviteError(c *gin.Context, err error) {
	zap.L().Error("invitation op failed", zap.Error(err))
	switch {
	case errors.Is(err, logic.ErrForbidden):
		ResponseError(c, CodeForbidden)
	case errors.Is(err, logic.ErrQuestionNotExist):
		ResponseError(c, CodeQuestionNotExist)
	case errors.Is(err, logic.ErrInviteLimit):
		ResponseError(c, CodeInvitationLimit)
	case errors.Is(err, logic.ErrInviteStateOp):
		ResponseError(c, CodeInvitationState)
	case errors.Is(err, logic.ErrInviteReceiver):
		ResponseError(c, CodeForbidden)
	case errors.Is(err, logic.ErrCursorInvalid):
		ResponseError(c, CodeInvalidParams)
	default:
		ResponseError(c, CodeServerBusy)
	}
}

// ListUnansweredQuestionsHandler 待回答问题列表
// @Summary 待回答问题列表
// @Description 公开可见、未解决且当前没有公开审核通过回答的问题；游客可浏览
// @Tags 问答相关接口
// @Produce json
// @Param topic_id query int false "话题 ID（可选）"
// @Param sort query string false "latest（默认）| oldest_unanswered"
// @Param cursor query string false "分页游标"
// @Param limit query int false "每页数量（默认 20 最大 50）"
// @Success 200 {object} controller.Response{data=models.PageResult[*models.ApiQuestionDetail]}
// @Router /questions/unanswered [get]
func ListUnansweredQuestionsHandler(c *gin.Context) {
	topicID, _ := strconv.ParseInt(c.Query("topic_id"), 10, 64)
	limit := atoiDefault(c.Query("limit"), 20)
	result, err := logic.ListUnansweredQuestions(topicID, c.DefaultQuery("sort", "latest"), c.Query("cursor"), limit)
	if err != nil {
		respondInviteError(c, err)
		return
	}
	ResponseSuccess(c, result)
}

// ListInviteCandidatesHandler 邀请候选
// @Summary 邀请候选用户
// @Description 仅问题作者；按同话题公开回答数等确定性信号排序，返回粗粒度理由
// @Tags 问答相关接口
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Param q query string false "用户名过滤"
// @Success 200 {object} controller.Response{data=[]models.ApiCandidateUser}
// @Router /questions/{id}/invitation-candidates [get]
func ListInviteCandidatesHandler(c *gin.Context) {
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || questionID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	candidates, err := logic.ListInviteCandidates(userID, questionID, c.Query("q"))
	if err != nil {
		respondInviteError(c, err)
		return
	}
	ResponseSuccess(c, candidates)
}

// SendAnswerInvitationsHandler 批量邀请回答
// @Summary 批量邀请回答
// @Description 问题作者一次邀请 1-5 人；逐人返回结果（部分失败说明原因）；同事务写入通知与审计
// @Tags 问答相关接口
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Param object body models.ParamSendAnswerInvitations true "被邀请人列表"
// @Success 200 {object} controller.Response{data=[]logic.InviteeResult}
// @Router /questions/{id}/answer-invitations [post]
func SendAnswerInvitationsHandler(c *gin.Context) {
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || questionID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamSendAnswerInvitations)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	results, err := logic.SendAnswerInvitations(userID, questionID, p, c.GetString("request_id"))
	if err != nil {
		respondInviteError(c, err)
		return
	}
	ResponseSuccess(c, results)
}

// ListQuestionInvitationsHandler 问题作者的邀请记录
// @Summary 问题的邀请记录
// @Description 仅问题作者；最近 20 条含状态
// @Tags 问答相关接口
// @Security ApiKeyAuth
// @Param id path int true "问题 ID"
// @Success 200 {object} controller.Response{data=[]models.AnswerInvitation}
// @Router /questions/{id}/answer-invitations [get]
func ListQuestionInvitationsHandler(c *gin.Context) {
	questionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || questionID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	list, err := logic.ListQuestionInvitations(userID, questionID)
	if err != nil {
		respondInviteError(c, err)
		return
	}
	ResponseSuccess(c, list)
}

// ListMyAnswerInvitationsHandler 我收到的邀请
// @Summary 我收到的回答邀请
// @Description 当前用户收到的邀请（可按状态过滤；返回前惰性把过期 pending 转为 expired）
// @Tags 用户相关接口
// @Security ApiKeyAuth
// @Param status query string false "pending|accepted|answered|declined|expired"
// @Param cursor query string false "分页游标"
// @Param limit query int false "每页数量（默认 20 最大 50）"
// @Success 200 {object} controller.Response{data=models.PageResult[*models.AnswerInvitation]}
// @Router /me/answer-invitations [get]
func ListMyAnswerInvitationsHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	limit := atoiDefault(c.Query("limit"), 20)
	result, err := logic.ListMyAnswerInvitations(userID, c.Query("status"), c.Query("cursor"), limit)
	if err != nil {
		respondInviteError(c, err)
		return
	}
	ResponseSuccess(c, result)
}

// AcceptAnswerInvitationHandler 接受邀请
// @Summary 接受回答邀请
// @Description 仅邀请接收者；幂等；过期/失效返回业务码 1046
// @Tags 问答相关接口
// @Security ApiKeyAuth
// @Param id path int true "邀请 ID"
// @Success 200 {object} controller.Response
// @Router /answer-invitations/{id}/accept [post]
func AcceptAnswerInvitationHandler(c *gin.Context) {
	respondInvitation(c, "accept")
}

// DeclineAnswerInvitationHandler 婉拒邀请
// @Summary 婉拒回答邀请
// @Description 仅邀请接收者；幂等
// @Tags 问答相关接口
// @Security ApiKeyAuth
// @Param id path int true "邀请 ID"
// @Success 200 {object} controller.Response
// @Router /answer-invitations/{id}/decline [post]
func DeclineAnswerInvitationHandler(c *gin.Context) {
	respondInvitation(c, "decline")
}

func respondInvitation(c *gin.Context, decision string) {
	invitationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || invitationID <= 0 {
		ResponseError(c, CodeInvalidParams)
		return
	}
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	if err := logic.RespondAnswerInvitation(userID, invitationID, decision, c.GetString("request_id")); err != nil {
		respondInviteError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{"invitation_id": strconv.FormatInt(invitationID, 10), "status": decision + "ed"})
}

// GetAnswerInvitationPreferenceHandler 读取邀请接收偏好
// @Summary 读取回答邀请接收偏好
// @Tags 用户相关接口
// @Security ApiKeyAuth
// @Success 200 {object} controller.Response{data=map[string]bool}
// @Router /me/answer-invitation-preference [get]
func GetAnswerInvitationPreferenceHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	enabled, err := logic.GetAnswerInvitePreference(userID)
	if err != nil {
		respondInviteError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{"enabled": enabled})
}

// SetAnswerInvitationPreferenceHandler 设置邀请接收偏好
// @Summary 设置回答邀请接收偏好
// @Description 关闭后不出现在候选、不能收到新邀请；不影响已接受邀请的回答入口
// @Tags 用户相关接口
// @Security ApiKeyAuth
// @Param object body models.ParamAnswerInvitationPreference true "偏好"
// @Success 200 {object} controller.Response
// @Router /me/answer-invitation-preference [put]
func SetAnswerInvitationPreferenceHandler(c *gin.Context) {
	userID, err := getCurrentUserID(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	p := new(models.ParamAnswerInvitationPreference)
	if err := c.ShouldBindJSON(p); err != nil {
		ResponseErrorWithMsg(c, CodeInvalidParams, Translate(err))
		return
	}
	if err := logic.SetAnswerInvitePreference(userID, *p.Enabled); err != nil {
		respondInviteError(c, err)
		return
	}
	ResponseSuccess(c, gin.H{"enabled": *p.Enabled})
}
