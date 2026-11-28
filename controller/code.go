package controller

// 业务错误码：显式分配，不再依赖 iota 排列，防止在中间插入新码导致已发布数字变化。
// 已对齐 frontend/src/api/http.ts 与文档中的错误码契约。
type ResCode int64

const (
	CodeSuccess            ResCode = 1000
	CodeInvalidParams      ResCode = 1001
	CodeUserExist          ResCode = 1002
	CodeUserNotExist       ResCode = 1003
	CodeInvalidCredentials ResCode = 1004
	CodePasswordMismatch   ResCode = 1005
	CodeServerBusy         ResCode = 1006
	CodePostNotExist       ResCode = 1007
	CodeCommunityNotExist  ResCode = 1008

	CodeInvalidAuth ResCode = 1009
	CodeNeedLogin   ResCode = 1010

	CodeVoteTimeExpire ResCode = 1011
	CodeRepeatedVote   ResCode = 1012

	CodeEmailExist        ResCode = 1013
	CodeEmailNotExist     ResCode = 1014
	CodeSendTooFrequent   ResCode = 1015
	CodeVerifyCodeInvalid ResCode = 1016
	CodeAccountBanned     ResCode = 1017

	CodeQuestionNotExist ResCode = 1018
	CodeAnswerNotExist   ResCode = 1019
	CodeTopicExist       ResCode = 1020

	CodeRegisterFailed   ResCode = 1021
	CodeSendCodeFailed   ResCode = 1022
	CodeVerifyCodeLocked ResCode = 1023

	CodeFeedRefreshing ResCode = 1024
	CodeFeedExpired    ResCode = 1025
	CodeForbidden      ResCode = 1026

	CodeResetTokenInvalid ResCode = 1027
	CodeEmailConflict     ResCode = 1028
	CodeDraftConflict     ResCode = 1029
	CodeArticleNotExist   ResCode = 1030
	CodeColumnNotExist    ResCode = 1031
	CodePreferenceConflict ResCode = 1032
	CodeAppealConflict    ResCode = 1033
	CodeSeriesOrderConflict ResCode = 1034
	CodeReviewPending     ResCode = 1035
	CodeRevisionConflict  ResCode = 1038
	CodeDMWaiting         ResCode = 1039
	CodeDMCooldown        ResCode = 1040
	CodeQuestionStateConflict ResCode = 1041
	CodeAnswerNotAcceptable   ResCode = 1042
	CodeQuestionClosedState   ResCode = 1043
	CodeQuestionTransition    ResCode = 1044
	CodeInvitationLimit       ResCode = 1045
	CodeInvitationState       ResCode = 1046
	CodeCommentNotExist       ResCode = 1047
)

var codeMsgMap = map[ResCode]string{
	CodeSuccess:            "成功",
	CodeInvalidParams:      "请求参数错误",
	CodeUserExist:          "用户名已存在",
	CodeUserNotExist:       "用户不存在",
	CodeInvalidCredentials: "用户名或密码错误",
	CodePasswordMismatch:   "密码不匹配",
	CodeServerBusy:         "服务器繁忙",
	CodePostNotExist:       "帖子不存在",
	CodeCommunityNotExist:  "社区不存在",
	CodeInvalidAuth:        "认证失败",
	CodeNeedLogin:          "需要登录",
	CodeVoteTimeExpire:     "投票时间已过",
	CodeRepeatedVote:       "请勿重复投票",
	CodeEmailExist:         "邮箱已被注册",
	CodeEmailNotExist:      "该邮箱尚未注册",
	CodeSendTooFrequent:    "发送过于频繁，请稍后再试",
	CodeVerifyCodeInvalid:  "验证码错误或已过期",
	CodeAccountBanned:      "账号已被禁用",
	CodeQuestionNotExist:   "问题不存在",
	CodeAnswerNotExist:     "回答不存在",
	CodeTopicExist:         "话题已存在",
	CodeRegisterFailed:     "注册失败，请检查填写信息或验证码",
	CodeSendCodeFailed:     "验证码发送失败，请稍后重试",
	CodeVerifyCodeLocked:   "错误次数过多，请重新获取验证码",
	CodeFeedRefreshing:     "内容更新中，请稍后重试",
	CodeFeedExpired:        "列表已更新，请刷新重试",
	CodeForbidden:          "无权操作该内容",
	CodeResetTokenInvalid:  "重置凭据无效或已过期",
	CodeEmailConflict:      "该邮箱已被其他账号使用",
	CodeDraftConflict:      "草稿版本冲突，请刷新后重试",
	CodeArticleNotExist:    "文章不存在",
	CodeColumnNotExist:     "专栏不存在",
	CodePreferenceConflict: "设置版本冲突，请刷新后重试",
	CodeAppealConflict:     "该内容已有进行中的申诉",
	CodeSeriesOrderConflict: "目录版本冲突，请刷新后重试",
	CodeReviewPending:      "该版本已有进行中的审阅",
	CodeRevisionConflict:   "版本冲突，请刷新后重试",
	CodeDMWaiting:          "首条消息已发送，等待对方接受请求",
	CodeDMCooldown:         "对方已拒绝过请求，请 7 天后再试",
	CodeQuestionStateConflict: "问题状态已变化，请刷新后重试",
	CodeAnswerNotAcceptable:   "该回答不可采纳",
	CodeQuestionClosedState:   "问题已关闭，无法采纳回答",
	CodeQuestionTransition:    "该状态转换不允许（solved 由采纳操作维护）",
	CodeInvitationLimit:       "邀请数已达上限",
	CodeInvitationState:       "邀请已处理或已失效",
	CodeCommentNotExist:       "评论不存在或已被删除",
}

// 创建一个函数，传入code返回msg
func (c ResCode) Msg() string {
	msg, ok := codeMsgMap[c]
	if !ok {
		return "未知错误"
	}
	return msg
}
