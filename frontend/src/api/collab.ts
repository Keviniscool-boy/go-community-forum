import http from './http'

// 系列
export interface Series {
  id: string
  column_id: string
  title: string
  description: string
  version: number
}

export interface SeriesItem {
  article_id: string
  position: number
  title: string
  available: boolean
}

export function createSeries(columnId: string, data: { title: string; description: string }) {
  return http.post<{ series_id: string }>(`columns/${columnId}/series`, data)
}

export function listSeriesForColumn(columnId: string) {
  return http.get<Series[]>(`columns/${columnId}/series`)
}

export function fetchSeries(id: string) {
  return http.get<{ series: Series; items: SeriesItem[] }>(`series/${id}`)
}

export function updateSeries(id: string, data: { title: string; description: string; version: number }) {
  return http.patch<null>(`series/${id}`, data)
}

export function setSeriesOrder(id: string, articleIds: string[], version: number) {
  return http.put<null>(`series/${id}/order`, { article_ids: articleIds, version })
}

// ---------- 成员与邀请 ----------

export interface ColumnMember {
  column_id: string
  user_id: string
  role: string
  create_time: string
}

export function listMembers(columnId: string) {
  return http.get<ColumnMember[]>(`columns/${columnId}/members`)
}

export function inviteMember(columnId: string, userId: string, role: 'editor' | 'reviewer') {
  return http.post<null>(`columns/${columnId}/invitations`, { user_id: userId, role })
}

export function removeMember(columnId: string, userId: string) {
  return http.delete<null>(`columns/${columnId}/members/${userId}`)
}

// ---------- 版本与审阅 ----------

export interface ArticleRevision {
  id: string
  article_id: string
  revision_no: number
  title: string
  author_id: string
  create_time: string
}

export function listRevisions(articleId: string) {
  return http.get<ArticleRevision[]>(`articles/${articleId}/versions`)
}

export function restoreRevision(articleId: string, revisionId: string) {
  return http.post<{ revision_id: string; revision_no: number }>(`articles/${articleId}/versions/${revisionId}/restore`)
}

export function submitReview(articleId: string, revisionId: string) {
  return http.post<{ review_id: string }>(`articles/${articleId}/reviews`, { revision_id: revisionId })
}

export function decideReview(reviewId: string, decision: 'approve' | 'changes_requested', revision: string) {
  return http.post<null>(`reviews/${reviewId}/decision`, { decision, revision: revision })
}
