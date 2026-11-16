import http from './http'

// 兴趣
export function getInterests() {
  return http.get<{ topic_ids: string[] }>('me/interests')
}
export function putInterests(topicIds: string[]) {
  return http.put<{ topic_ids: string[] }>('me/interests', { topic_ids: topicIds })
}

// 推荐偏好
export function getPreferences() {
  return http.get<{ personalization_enabled: boolean; version: number }>('me/recommendation-preferences')
}
export function putPreferences(enabled: boolean, version: number) {
  return http.put<{ personalization_enabled: boolean; version: number }>('me/recommendation-preferences', {
    personalization_enabled: enabled,
    version,
  })
}

// 负反馈
export interface FeedbackItem {
  kind: string
  target_type: string
  target_id: string
  create_time: string
}
export function listFeedback() {
  return http.get<FeedbackItem[]>('me/recommendation-feedback')
}
export function addFeedback(kind: 'not_interested' | 'reduce_topic', targetType: 'content' | 'topic', targetId: string) {
  return http.post<null>('me/recommendation-feedback', { kind, target_type: targetType, target_id: targetId })
}
export function removeFeedback(kind: string, type: string, id: string) {
  return http.delete<null>(`me/recommendation-feedback/${kind}/${type}/${id}`)
}

// 屏蔽
export interface BlockedUser {
  user_id: string
  username: string
  avatar_url: string
}
export function listBlocks() {
  return http.get<BlockedUser[]>('me/blocks')
}
export function blockUser(id: string) {
  return http.post<null>(`users/${id}/block`)
}
export function unblockUser(id: string) {
  return http.delete<null>(`users/${id}/block`)
}

// 创作者中心
export interface CreatorOverview {
  questions: number
  answers: number
  articles: number
  likes_received: number
  comments_received: number
  followers: number
  views: number
  as_of: string
}
export function creatorOverview() {
  return http.get<CreatorOverview>('creator/overview')
}
export function creatorContents(type: string, status = '1') {
  return http.get<Array<Record<string, unknown>>>('creator/contents', { params: { type, status } })
}
export function creatorMetrics(type: string, id: string) {
  return http.get<{ daily: Array<Record<string, unknown>>; as_of: string }>(`creator/contents/${type}/${id}/metrics`)
}
