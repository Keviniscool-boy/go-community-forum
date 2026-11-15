import http from './http'
import type { PostDetail } from '@/types/api'

export interface PostListParams {
  page?: number
  size?: number
  order?: 'time' | 'score'
}

// 帖子列表（旧接口，迁移期保留；新 Feed 上线后前端切换到 /feed/*）
export function fetchPostList(params: PostListParams) {
  return http.get<PostDetail[]>('posts2/', { params })
}

// 帖子详情
export function fetchPostDetail(id: string) {
  return http.get<PostDetail>(`post/${id}`)
}

// 发布帖子
export function createPost(data: { title: string; content: string; community_id: string }) {
  return http.post<null>('post', data)
}
