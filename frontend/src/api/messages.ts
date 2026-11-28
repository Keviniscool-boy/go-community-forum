import http from './http'

// 私信 API（第五阶段 F6）
export interface MessageThread {
  id: string
  peer_id: string
  peer_name: string
  peer_avatar: string
  unread_count: number
  last_message_at: string
  last_excerpt?: string
}

export interface MessageDetail {
  id: string
  thread_id: string
  sender_id: string
  sender_name: string
  content: string
  create_time: string
}

export function fetchThreads(cursor = '', limit = 20) {
  return http.get<import('./question').PageResult<MessageThread>>('messages/threads', {
    params: { cursor: cursor || undefined, limit },
  })
}

export function fetchThreadMessages(threadId: string, cursor = '', limit = 30) {
  return http.get<import('./question').PageResult<MessageDetail>>(`messages/threads/${threadId}`, {
    params: { cursor: cursor || undefined, limit },
  })
}

export function openDirectMessage(userId: string, content: string) {
  return http.post<{ thread_id: string; message: MessageDetail }>(`messages/users/${userId}`, { content })
}

export function sendMessage(threadId: string, content: string) {
  return http.post<{ thread_id: string; message: MessageDetail }>(`messages/threads/${threadId}`, { content })
}

export function deleteThread(threadId: string) {
  return http.delete<null>(`messages/threads/${threadId}`)
}

export function fetchMessageUnreadCount() {
  return http.get<{ count: number }>('messages/unread-count')
}
