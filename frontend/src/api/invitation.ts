import http from './http'
import type { PageResult, QuestionDetail } from './question'

// 邀请状态（与后端 models.InviteStatus* 对应）
export type InvitationStatus = 0 | 1 | 2 | 3 | 4 | 5

export const invitationStatusLabels: Record<number, string> = {
  0: '待处理',
  1: '已接受',
  2: '已婉拒',
  3: '已回答',
  4: '已撤回',
  5: '已失效',
}

// 邀请记录
export interface AnswerInvitation {
  id: string
  question_id: string
  inviter_id: string
  invitee_id: string
  status: InvitationStatus
  expires_at: string
  responded_at?: string
  answered_at?: string
  create_time: string
  invitee_name?: string
  inviter_name?: string
  question_title?: string
}

// 邀请候选（粗粒度理由）
export interface CandidateUser {
  user_id: string
  username: string
  avatar_url: string
  reason: string
}

// 逐人发送结果（PRD §6.2 部分失败逐人说明）
export interface InviteeResult {
  invitee_id: string
  invitation_id?: string
  ok: boolean
  reason?: string
}

// 待回答问题列表（公开）
export function fetchUnansweredQuestions(params: {
  topic_id?: string
  sort?: 'latest' | 'oldest_unanswered'
  cursor?: string
  limit?: number
}) {
  return http.get<PageResult<QuestionDetail>>('questions/unanswered', { params })
}

// 邀请候选（仅问题作者，最多 20 人单页）
export function fetchInviteCandidates(questionId: string, q = '') {
  return http.get<CandidateUser[]>(`questions/${questionId}/invitation-candidates`, {
    params: q ? { q } : {},
  })
}

// 批量邀请回答（1-5 人）
export function sendAnswerInvitations(questionId: string, inviteeIds: string[]) {
  return http.post<InviteeResult[]>(`questions/${questionId}/answer-invitations`, {
    invitee_ids: inviteeIds,
  })
}

// 问题的邀请记录（仅作者，最近 20 条）
export function fetchQuestionInvitations(questionId: string) {
  return http.get<AnswerInvitation[]>(`questions/${questionId}/answer-invitations`)
}

// 我收到的邀请
export function fetchMyAnswerInvitations(params: {
  status?: 'pending' | 'accepted' | 'answered' | 'declined' | 'expired'
  cursor?: string
  limit?: number
}) {
  return http.get<PageResult<AnswerInvitation>>('me/answer-invitations', { params })
}

// 接受 / 婉拒
export function acceptAnswerInvitation(invitationId: string) {
  return http.post<{ invitation_id: string; status: string }>(`answer-invitations/${invitationId}/accept`)
}

export function declineAnswerInvitation(invitationId: string) {
  return http.post<{ invitation_id: string; status: string }>(`answer-invitations/${invitationId}/decline`)
}

// 接收偏好
export function fetchAnswerInvitationPreference() {
  return http.get<{ enabled: boolean }>('me/answer-invitation-preference')
}

export function setAnswerInvitationPreference(enabled: boolean) {
  return http.put<{ enabled: boolean }>('me/answer-invitation-preference', { enabled })
}
