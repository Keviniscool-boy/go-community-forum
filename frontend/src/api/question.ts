import http from './http'
import type { UserInfo } from '@/types/api'

// 话题简要信息
export interface TopicBrief {
  topic_id: string
  name: string
}

// 问题详情（resolution_status: 0=open 1=solved 2=closed）
export interface QuestionDetail {
  id: string
  title: string
  description: string
  author_id: string
  author_name: string
  answer_count: number
  comment_count: number
  follower_count: number
  view_count: number
  status: number
  resolution_status: number
  accepted_answer_id: string
  create_time: string
  update_time?: string
  topics: TopicBrief[]
}

// 回答详情（accepted 由服务端按采纳关系派生）
export interface AnswerDetail {
  id: string
  question_id: string
  author_id: string
  author_name: string
  content: string
  vote_count: number
  comment_count: number
  collect_count: number
  status: number
  accepted?: boolean
  create_time: string
}

// cursor 分页响应
export interface PageResult<T> {
  items: T[]
  next_cursor: string
  has_more: boolean
}

export function getCurrentUser(): UserInfo | null {
  return JSON.parse(localStorage.getItem('bb_user') || 'null')
}

// 创建问题
export function createQuestion(data: { title: string; description: string; topic_ids?: string[] }) {
  return http.post<{ question_id: string }>('questions', data)
}

// 问题详情
export function fetchQuestionDetail(id: string) {
  return http.get<QuestionDetail>(`questions/${id}`)
}

// 创建回答
export function createAnswer(questionId: string, content: string) {
  return http.post<{ answer_id: string }>(`questions/${questionId}/answers`, { content })
}

// 回答列表（cursor 分页）
export function fetchAnswers(
  questionId: string,
  params: { cursor?: string; limit?: number; order?: 'hot' | 'latest' },
) {
  return http.get<PageResult<AnswerDetail>>(`questions/${questionId}/answers`, { params })
}

// 编辑问题（仅作者）
export function updateQuestion(id: string, data: { title: string; description: string }) {
  return http.put<null>(`questions/${id}`, data)
}

// 删除问题（软删除，仅作者）
export function deleteQuestion(id: string) {
  return http.delete<null>(`questions/${id}`)
}

// 编辑回答（仅作者）
export function updateAnswer(id: string, content: string) {
  return http.put<null>(`answers/${id}`, { content })
}

// 删除回答（软删除，仅作者）
export function deleteAnswer(id: string) {
  return http.delete<null>(`answers/${id}`)
}

// 采纳回答（仅问题作者；同问题唯一；幂等）
export function acceptAnswer(questionId: string, answerId: string) {
  return http.post<{ question_id: string; accepted_answer_id: string; status: string }>(
    `questions/${questionId}/answers/${answerId}/accept`,
  )
}

// 撤销采纳（仅问题作者；幂等）
export function unacceptAnswer(questionId: string, answerId: string) {
  return http.delete<{ question_id: string; status: string }>(
    `questions/${questionId}/answers/${answerId}/accept`,
  )
}

// 问题关闭/重开（open/closed 转换；solved 由采纳维护）
export function setQuestionStatus(id: string, status: 'open' | 'closed') {
  return http.patch<{ question_id: string; status: string }>(`questions/${id}/status`, { status })
}

// 我的内容管理条目（跨问题/回答/文章统一形状）
export interface MyContentItem {
  content_type: 'question' | 'answer' | 'article'
  content_id: string
  title: string
  excerpt: string
  status: number // 1=存在 0=已软删除
  moderation_status: number // 0 待审 1 通过 2 拒绝 3 隔离
  resolution_status: number
  create_time: string
}

// 我的内容管理列表（type+status 过滤，cursor 分页）
export function fetchMyContent(params: {
  type: 'question' | 'answer' | 'article'
  status?: 'published' | 'pending' | 'hidden' | 'deleted'
  cursor?: string
  limit?: number
}) {
  return http.get<PageResult<MyContentItem>>('me/content', { params })
}
