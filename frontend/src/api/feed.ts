import http from './http'
import type { UserInfo } from '@/types/api'

// Feed 条目作者
export interface FeedAuthor {
  user_id: string
  username: string
  avatar_url: string
}

// Feed 条目互动数据
export interface FeedStats {
  vote_count: number
  comment_count: number
  collect_count: number
  answer_count?: number
}

// Feed 条目
export interface FeedItem {
  item_type: 'question' | 'answer' | 'article'
  item_id: string
  question_id?: string
  title: string
  excerpt: string
  author: FeedAuthor
  topic?: { topic_id: string; name: string } | null
  column?: { column_id: string; name: string } | null
  stats: FeedStats
  create_time: string
  sources?: string[]
  recommendation_reason?: string
  exposure_id?: string
}

export type FeedType = 'recommend' | 'following' | 'latest' | 'hot'

export interface PageResult<T> {
  items: T[]
  next_cursor: string
  has_more: boolean
}

export type { UserInfo }

// 获取 Feed（cursor 分页）
export function fetchFeed(type: FeedType, cursor = '', limit = 20) {
  return http.get<PageResult<FeedItem>>(`feed/${type}`, { params: { cursor, limit } })
}
