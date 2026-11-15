import http from './http'
import type { PageResult } from './feed'
import type { NotificationDetail, Topic } from './notification'

// 用户主页数据
export interface UserProfile {
  user_id: string
  username: string
  avatar_url: string
  bio: string
  question_count: number
  answer_count: number
  follower_count: number
  following_count: number
  is_following: boolean
}

// 用户主页数据
export function fetchUserProfile(id: string) {
  return http.get<UserProfile>(`users/${id}`)
}

// 用户的提问列表
export function fetchUserQuestions(id: string) {
  return http.get<import('./question').QuestionDetail[]>(`users/${id}/questions`)
}

// 用户的回答列表
export function fetchUserAnswers(id: string) {
  return http.get<
    Array<{
      id: string
      question_id: string
      content: string
      vote_count: number
      comment_count: number
      collect_count: number
      create_time: string
      question_title?: string
    }>
  >(`users/${id}/answers`)
}

export type { PageResult, NotificationDetail, Topic }
