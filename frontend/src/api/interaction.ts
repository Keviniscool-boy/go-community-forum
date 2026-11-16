import http from './http'

// 对回答投票：1 赞同，-1 反对，0 取消
export function voteAnswer(answerId: string, direction: 1 | -1 | 0) {
  return http.post<null>(`answers/${answerId}/vote`, { direction })
}

// 收藏 / 取消收藏回答
export function collectAnswer(answerId: string) {
  return http.post<null>(`answers/${answerId}/collect`)
}
export function uncollectAnswer(answerId: string) {
  return http.delete<null>(`answers/${answerId}/collect`)
}

// 回答互动状态（当前用户投票方向与收藏状态）
export function fetchAnswerInteractions(answerId: string) {
  return http.get<{ vote: number; collected: boolean }>(`answers/${answerId}/interactions`)
}

// 关注 / 取消关注用户
export function followUser(userId: string) {
  return http.post<null>(`users/${userId}/follow`)
}
export function unfollowUser(userId: string) {
  return http.delete<null>(`users/${userId}/follow`)
}

// 关注 / 取消关注问题
export function followQuestion(questionId: string) {
  return http.post<null>(`questions/${questionId}/follow`)
}
export function unfollowQuestion(questionId: string) {
  return http.delete<null>(`questions/${questionId}/follow`)
}

// 问题互动状态
export function fetchQuestionInteractions(questionId: string) {
  return http.get<{ followed: boolean }>(`questions/${questionId}/interactions`)
}

// 评论
export interface CommentDetail {
  id: string
  user_id: string
  username: string
  avatar_url: string
  parent_id: string
  content: string
  create_time: string
}

export function fetchComments(targetType: 'answer' | 'question' | 'article', targetId: string, cursor = '', limit = 20) {
  return http.get<import('./feed').PageResult<CommentDetail>>('comments', {
    params: { target_type: targetType, target_id: targetId, cursor, limit },
  })
}

export function createComment(targetType: 'answer' | 'question' | 'article', targetId: string, content: string, parentId?: string) {
  return http.post<{ comment_id: string }>('comments', {
    target_type: targetType,
    target_id: targetId,
    parent_id: parentId,
    content,
  })
}
