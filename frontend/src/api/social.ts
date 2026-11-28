import http from './http'

// 第五阶段 M1 社交关系 API

// 关注/粉丝列表条目
export interface FollowListUser {
  user_id: string
  username: string
  avatar_url: string
  bio: string
  is_following: boolean
}

// 黑名单条目
export interface BlockedUserItem {
  user_id: string
  username: string
  avatar_url: string
  create_time: string
}

// 粉丝列表（关注 targetID 的人）
export function fetchFollowers(userId: string, cursor = '', limit = 20) {
  return http.get<import('./question').PageResult<FollowListUser>>(`users/${userId}/followers`, {
    params: { cursor: cursor || undefined, limit },
  })
}

// 关注列表（targetID 关注的人）
export function fetchFollowing(userId: string, cursor = '', limit = 20) {
  return http.get<import('./question').PageResult<FollowListUser>>(`users/${userId}/following`, {
    params: { cursor: cursor || undefined, limit },
  })
}

// 拉黑 / 取消拉黑
export function blockUser(userId: string) {
  return http.post<null>(`users/${userId}/block`)
}

export function unblockUser(userId: string) {
  return http.delete<null>(`users/${userId}/block`)
}

// 我的黑名单
export function fetchBlocks(cursor = '', limit = 20) {
  return http.get<import('./question').PageResult<BlockedUserItem>>('users/blocks', {
    params: { cursor: cursor || undefined, limit },
  })
}

// 与目标用户的互动状态
export function fetchUserInteractions(userId: string) {
  return http.get<{ is_following: boolean; is_blocked: boolean }>(`users/${userId}/interactions`)
}

// 通知单条已读（幂等）
export function markNotificationRead(notificationId: string) {
  return http.patch<null>(`notifications/${notificationId}/read`)
}

// 编辑评论（5 分钟窗口内）
export function editComment(commentId: string, content: string) {
  return http.patch<unknown>(`comments/${commentId}`, { content })
}
