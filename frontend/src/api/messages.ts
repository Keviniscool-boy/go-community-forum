import http from './http'

export interface Conversation {
  id: string
  initiator_id: string
  state: number // 0 pending 1 accepted 2 declined 3 closed
  peer_name: string
  peer_id: string
  last_body: string
  unread: number
  update_time: string
}

export interface DirectMessage {
  id: string
  conversation_id: string
  sender_id: string
  body: string
  create_time: string
}

export function startConversation(recipientId: string, body: string, clientMessageId: string) {
  return http.post<{ conversation_id: string; message: DirectMessage }>('conversations', {
    recipient_id: recipientId,
    body,
    client_message_id: clientMessageId,
  })
}

export function listConversations() {
  return http.get<Conversation[]>('conversations')
}

export function acceptConversation(id: string) {
  return http.post<null>(`conversations/${id}/accept`)
}

export function declineConversation(id: string) {
  return http.post<null>(`conversations/${id}/decline`)
}

export function closeConversation(id: string) {
  return http.post<null>(`conversations/${id}/close`)
}

export function hideConversation(id: string) {
  return http.post<null>(`conversations/${id}/hide`)
}

export function listMessages(id: string, before = '') {
  return http.get<DirectMessage[]>(`conversations/${id}/messages`, { params: { before, limit: 30 } })
}

export function sendMessage(id: string, body: string, clientMessageId: string) {
  return http.post<DirectMessage>(`conversations/${id}/messages`, { body, client_message_id: clientMessageId })
}

export function markRead(id: string, lastReadMessageId: string) {
  return http.post<null>(`conversations/${id}/read`, { last_read_message_id: lastReadMessageId })
}

export function reportMessage(id: string, reasonCode: string, description = '') {
  return http.post<null>(`messages/${id}/report`, { reason_code: reasonCode, description })
}

// 隐私设置
export function getMessageSettings() {
  return http.get<{ incoming_policy: string; read_receipts_enabled: boolean; version: number }>('me/message-settings')
}
export function putMessageSettings(data: {
  incoming_policy: string
  read_receipts_enabled: boolean
  version: number
}) {
  return http.put<{ incoming_policy: string; read_receipts_enabled: boolean; version: number }>(
    'me/message-settings',
    data,
  )
}

export function genClientMessageId(): string {
  return 'cm-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 10)
}
