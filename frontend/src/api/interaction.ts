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

// 评论（两级结构：parent_id = '0' 为一级评论，其余为挂在一级评论下的回复）
export interface CommentDetail {
  id: string
  user_id: string
  username: string
  avatar_url: string
  parent_id: string
  content: string
  create_time: string
  like_count: number
  reply_count: number // 直接回复数（仅一级评论有效）
  liked: boolean // 当前用户是否已点赞
  can_delete: boolean // 当前用户是否可删除（作者或管理员/审核员）
  is_deleted: boolean // 已删除但保留占位（下方仍有可见回复）
  reply_to_user?: string // 回复另一条回复时的被回复人（「回复 @xxx」）
}

export function fetchComments(targetType: 'answer' | 'question', targetId: string, cursor = '', limit = 20) {
  return http.get<import('./feed').PageResult<CommentDetail>>('comments', {
    params: { target_type: targetType, target_id: targetId, cursor, limit },
  })
}

// 某条一级评论下的回复（最早在前）
export function fetchCommentReplies(commentId: string, cursor = '', limit = 20) {
  return http.get<import('./feed').PageResult<CommentDetail>>(`comments/${commentId}/replies`, {
    params: { cursor, limit },
  })
}

// parentId：被回复的评论 ID（可以是一级评论，也可以是其下的一条回复）
export function createComment(targetType: 'answer' | 'question', targetId: string, content: string, parentId?: string) {
  return http.post<{ comment_id: string }>('comments', {
    target_type: targetType,
    target_id: targetId,
    parent_id: parentId,
    content,
  })
}

export function likeComment(commentId: string) {
  return http.post<null>(`comments/${commentId}/like`)
}

export function unlikeComment(commentId: string) {
  return http.delete<null>(`comments/${commentId}/like`)
}

// 删除评论（软删除，仅作者或管理员/审核员；其下回复保留）
export function deleteComment(commentId: string) {
  return http.delete<null>(`comments/${commentId}`)
}
