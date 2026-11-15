import http from './http'
import type { PageResult } from './feed'

// ---------- 草稿 ----------

export interface Draft {
  id: string
  user_id: string
  draft_type: number // 0 question 1 answer
  question_id: string
  title: string
  content: string
  version: number
  update_time: string
}

export function createDraft(data: { draft_type: 'question' | 'answer'; question_id?: string; title: string; content: string }) {
  return http.post<{ draft_id: string; version: number }>('drafts', data)
}

export function listDrafts(type: 'question' | 'answer') {
  return http.get<Draft[]>('drafts', { params: { type } })
}

export function updateDraft(id: string, data: { title: string; content: string; version: number }) {
  return http.patch<{ version: number }>(`drafts/${id}`, data)
}

export function deleteDraft(id: string) {
  return http.delete<null>(`drafts/${id}`)
}

export function publishDraft(id: string) {
  return http.post<{ question_id?: string; answer_id?: string }>(`drafts/${id}/publish`)
}

// ---------- 收藏夹 ----------

export interface BookmarkFolder {
  folder_id: string
  name: string
  is_private: boolean
}

export function listFolders() {
  return http.get<BookmarkFolder[]>('bookmark-folders')
}

export function createFolder(name: string) {
  return http.post<{ folder_id: string }>('bookmark-folders', { name })
}

export function deleteFolder(id: string) {
  return http.delete<null>(`bookmark-folders/${id}`)
}

export interface BookmarkItemVO {
  target_type: number
  target_id: string
  created_at: string
  available: boolean
}

export function listFolderItems(id: string) {
  return http.get<BookmarkItemVO[]>(`bookmark-folders/${id}/items`)
}

export function addBookmarkItem(folderId: string, targetType: 'question' | 'answer', targetId: string) {
  return http.post<null>(`bookmark-folders/${folderId}/items`, { target_type: targetType, target_id: targetId })
}

// ---------- 历史 ----------

export function listHistory() {
  return http.get<BookmarkItemVO[]>('history')
}

export function clearHistory() {
  return http.delete<null>('history')
}

// ---------- 举报 ----------

export function createReport(data: {
  target_type: 'question' | 'answer' | 'comment' | 'user'
  target_id: string
  reason_code: 'spam' | 'abuse' | 'copyright' | 'unsafe' | 'other'
  description?: string
}) {
  return http.post<null>('reports', data)
}

// ---------- 账号安全 ----------

export function logoutAllDevices() {
  return http.post<null>('auth/logout-all')
}

// ---------- 管理后台 ----------

export interface ReportDetail {
  id: string
  reporter_id: string
  reporter_name: string
  target_type: number
  target_id: string
  reason_code: string
  description: string
  status: number
  review_note: string
  created_at: string
}

export function listReports(status: string, cursor = '') {
  return http.get<PageResult<ReportDetail>>('admin/reports', { params: { status, cursor, limit: 50 } })
}

export function resolveReport(id: string, decision: 'resolve' | 'dismiss', note = '') {
  return http.post<null>(`admin/reports/${id}/resolve`, { decision, note })
}

export function moderateContent(type: 'question' | 'answer', id: string, action: 'approve' | 'reject' | 'quarantine') {
  return http.post<null>(`admin/content/${type}/${id}/${action}`)
}

export function setUserEnabled(id: string, enable: boolean) {
  return http.post<null>(`admin/users/${id}/disable`, { enable })
}

export interface AuditLogItem {
  id: string
  actor_id: string
  action: string
  target_type: string
  target_id: string
  request_id: string
  created_at: string
}

export function listAuditLogs(cursor = '') {
  return http.get<PageResult<AuditLogItem>>('admin/audit-logs', { params: { cursor, limit: 50 } })
}

export function metricsOverview() {
  return http.get<Record<string, number>>('admin/metrics/overview')
}

// ---------- Markdown / 媒体 ----------

export function previewMarkdown(markdown: string) {
  return http.post<{ html: string }>('markdown/preview', { markdown })
}
