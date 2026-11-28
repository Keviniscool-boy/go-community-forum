import http from './http'
import type { PageResult } from './feed'

// 通知详情
export interface NotificationDetail {
  id: string
  actor_id: string
  actor_name: string
  type: number // 1 赞同 2 评论 3 关注 4 收藏 5 私信 6 采纳 7 撤销采纳 8 新回答 9 邀请回答 10/11 邀请响应 12 赞同评论 13 回复评论
  target_type: number // 1 回答 2 问题 3 文章 0 用户
  target_id: string // 目标为回答时是回答 ID
  question_id?: string // 跳转用：回答类为所属问题 ID；邀请类（type 9）为问题 ID
  is_read: boolean
  create_time: string
  excerpt: string
  agg_count?: number // 聚合条数（>1 时显示"某某等 N 人"）
}

export function fetchNotifications(cursor = '', limit = 20) {
  return http.get<PageResult<NotificationDetail>>('notifications', { params: { cursor, limit } })
}

export function fetchUnreadCount() {
  return http.get<{ count: number }>('notifications/unread-count')
}

export function markAllRead() {
  return http.post<null>('notifications/read-all')
}

// 话题
export interface Topic {
  topic_id: string
  name: string
  description: string
  avatar_url: string
  follower_count: number
}

// 话题下的问题（后端 models.Question）
export interface TopicQuestion {
  id: string
  title: string
  description: string
  author_id: string
  answer_count: number
  comment_count: number
  follower_count: number
  view_count: number
  create_time: string
}

export function fetchTopics(limit = 20) {
  return http.get<Topic[]>('topics', { params: { limit } })
}

export function fetchTopicDetail(id: string) {
  return http.get<{ topic: Topic; questions: TopicQuestion[] }>(`topics/${id}`)
}

export function followTopic(id: string) {
  return http.post<null>(`topics/${id}/follow`)
}

export function unfollowTopic(id: string) {
  return http.delete<null>(`topics/${id}/follow`)
}

// 通知单条已读（第五阶段 F3；幂等）
export function markNotificationRead(notificationId: string) {
  return http.patch<null>(`notifications/${notificationId}/read`)
}
