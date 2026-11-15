import http from './http'
import type { UserInfo } from '@/types/api'

// 话题简要信息
export interface TopicBrief {
  topic_id: string
  name: string
}

// 问题详情
export interface QuestionDetail {
  id: string
  title: string
  description: string
  author_id: string
  author_name: string
  answer_count: number
  follower_count: number
  view_count: number
  status: number
  create_time: string
  topics: TopicBrief[]
}

// 回答详情
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
