import http from './http'

// 搜索结果条目
export interface SearchResultItem {
  document_type: 'question' | 'answer' | 'article' | 'column' | 'user' | 'topic'
  document_id: string
  title: string
  excerpt: string
  author_id?: string
  solved?: boolean // question：是否已解决
  question_id?: string // answer：所属问题 id（跳转用）
  create_time: number
}

export interface SearchResult {
  items: SearchResultItem[]
  next_cursor: string
  has_more: boolean
}

export function search(keyword: string, scope = 'all', sort = 'relevance', cursor = '', limit = 20) {
  return http.get<SearchResult>('search', { params: { q: keyword, scope, sort, cursor, limit } })
}

export function searchSuggestions(prefix: string) {
  return http.get<string[]>('search/suggestions', { params: { q: prefix } })
}
