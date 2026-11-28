import http from './http'
import type { PageResult } from './feed'

export interface ArticleDetail {
  id: string
  author_id: string
  column_id: string
  title: string
  content: string
  view_count: number
  create_time: string
  update_time: string
  author_name: string
  column?: { column_id: string; name: string } | null
}

export function listArticles(cursor = '', limit = 20) {
  return http.get<PageResult<ArticleDetail>>('articles', { params: { cursor, limit } })
}

export function fetchArticle(id: string) {
  return http.get<ArticleDetail>(`articles/${id}`)
}

export function createArticle(data: { title: string; content: string; column_id?: string }) {
  return http.post<{ article_id: string }>('articles', data)
}

export function updateArticle(id: string, data: { title: string; content: string }) {
  return http.patch<null>(`articles/${id}`, data)
}

export function deleteArticle(id: string) {
  return http.delete<null>(`articles/${id}`)
}

// ---------- 专栏 ----------

export interface ColumnDetail {
  id: string
  owner_id: string
  name: string
  description: string
  owner_name: string
  article_count: number
  create_time: string
}

export function listColumns(limit = 20) {
  return http.get<ColumnDetail[]>('columns', { params: { limit } })
}

export function fetchColumn(id: string) {
  return http.get<ColumnDetail>(`columns/${id}`)
}

export function createColumn(data: { name: string; description: string }) {
  return http.post<{ column_id: string }>('columns', data)
}

export function updateColumn(id: string, data: { name: string; description: string }) {
  return http.patch<null>(`columns/${id}`, data)
}

export function deleteColumn(id: string) {
  return http.delete<null>(`columns/${id}`)
}

export function fetchColumnArticles(id: string) {
  return http.get<ArticleDetail[]>(`columns/${id}/articles`)
}
