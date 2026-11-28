<script setup lang="ts">
// 私信（PRD F6）：/messages 会话列表 + /messages/:id 聊天页（30s 轮询）
import { ref, onMounted, onBeforeUnmount, nextTick, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  fetchThreads, fetchThreadMessages, openDirectMessage, sendMessage, deleteThread,
  type MessageThread, type MessageDetail,
} from '@/api/messages'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

// ---------- 会话列表 ----------
const threads = ref<MessageThread[]>([])
const listLoading = ref(false)
const listError = ref('')
let pollTimer: number | undefined

async function loadThreads(reset = false) {
  if (listLoading.value) return
  listLoading.value = true
  listError.value = ''
  try {
    if (reset) threads.value = []
    const res = await fetchThreads('')
    threads.value = reset ? res.items : [...threads.value, ...res.items]
  } catch (e) {
    listError.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    listLoading.value = false
  }
}

async function onDeleteThread(t: MessageThread) {
  try {
    await deleteThread(t.id)
    ElMessage.success('会话已删除')
    threads.value = threads.value.filter((x) => x.id !== t.id)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

// ---------- 聊天页 ----------
const activeThreadId = computed(() => (route.name === 'chat' ? (route.params.id as string) : ''))
// 新会话模式：/messages/new:<userId>（从用户主页发起；首条消息发出后跳转到正式会话）
const composePeer = computed(() =>
  activeThreadId.value.startsWith('new:') ? activeThreadId.value.slice(4) : '',
)
const messages = ref<MessageDetail[]>([])
const chatLoading = ref(false)
const chatError = ref('')
const hasOlder = ref(false)
const input = ref('')
const sending = ref(false)
const chatBox = ref<HTMLElement | null>(null)

async function loadMessages(reset = false) {
  if (!activeThreadId.value || composePeer.value) return
  if (chatLoading.value) return
  chatLoading.value = true
  chatError.value = ''
  try {
    if (reset) {
      messages.value = []
      hasOlder.value = false
    }
    const res = await fetchThreadMessages(activeThreadId.value, reset ? '' : oldestCursor())
    if (reset) {
      messages.value = res.items
      hasOlder.value = res.has_more
    } else if (res.items.length > 0) {
      // 向上翻页：更早的消息插到前面
      messages.value = [...res.items, ...messages.value]
      hasOlder.value = res.has_more
    }
    await nextTick()
    if (reset && chatBox.value) chatBox.value.scrollTop = chatBox.value.scrollHeight
  } catch (e) {
    chatError.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    chatLoading.value = false
  }
}

function oldestCursor(): string {
  const first = messages.value[0]
  if (!first) return ''
  // 后端约定：cursor = create_time|id（encodeTimeCursor 生成，前端透传）
  return `${first.create_time}|${first.id}`
}

async function pollLatest() {
  if (!activeThreadId.value || composePeer.value) return
  try {
    const res = await fetchThreadMessages(activeThreadId.value, '', 30)
    for (const m of res.items) {
      if (!messages.value.some((x) => x.id === m.id)) messages.value.push(m)
    }
    await nextTick()
    if (chatBox.value) chatBox.value.scrollTop = chatBox.value.scrollHeight
  } catch {
    // 轮询失败静默
  }
}

async function send() {
  const content = input.value.trim()
  if (!content) return
  sending.value = true
  try {
    let msg: MessageDetail
    if (activeThreadId.value) {
      const res = await sendMessage(activeThreadId.value, content)
      msg = res.message
    } else {
      // 从用户主页发起
      const res = await openDirectMessage(route.params.id as string, content)
      msg = res.message
      router.replace(`/messages/${res.thread_id}`)
    }
    messages.value.push(msg)
    input.value = ''
    await nextTick()
    if (chatBox.value) chatBox.value.scrollTop = chatBox.value.scrollHeight
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    sending.value = false
  }
}

let listPollTimer: number | undefined

onMounted(() => {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  loadThreads(true)
  if (activeThreadId.value && !composePeer.value) loadMessages(true)
  // PRD：30s 轮询（会话列表 + 会话页）
  listPollTimer = window.setInterval(() => loadThreads(), 30000)
  pollTimer = window.setInterval(pollLatest, 30000)
})

onBeforeUnmount(() => {
  if (listPollTimer) window.clearInterval(listPollTimer)
  if (pollTimer) window.clearInterval(pollTimer)
})
</script>

<template>
  <div class="messages-view">
    <!-- 会话列表 -->
    <div v-if="!activeThreadId" class="thread-list">
      <div class="bb-card header-card">
        <h3 class="view-title">私信</h3>
      </div>
      <div v-if="listLoading && threads.length === 0" class="bb-card"><LoadingState /></div>
      <div v-else-if="listError" class="bb-card"><ErrorState :text="listError" @retry="loadThreads(true)" /></div>
      <div v-else-if="threads.length === 0" class="bb-card">
        <EmptyState text="还没有会话，去别人主页发起私信吧" />
      </div>
      <template v-else>
        <div v-for="t in threads" :key="t.id" class="bb-card thread-row" @click="router.push(`/messages/${t.id}`)">
          <el-avatar :size="40" :src="t.peer_avatar || undefined">{{ t.peer_name?.slice(0, 1) }}</el-avatar>
          <div class="thread-main">
            <div class="thread-top">
              <span class="thread-name">{{ t.peer_name }}</span>
              <span class="thread-time">{{ t.last_message_at?.slice(0, 16).replace('T', ' ') }}</span>
            </div>
            <div class="thread-bottom">
              <span class="thread-excerpt">{{ t.last_excerpt || '…' }}</span>
              <el-badge v-if="t.unread_count > 0" :value="t.unread_count" :max="99" />
            </div>
          </div>
          <el-button size="small" text type="danger" @click.stop="onDeleteThread(t)">删除</el-button>
        </div>
      </template>
    </div>

    <!-- 聊天页 -->
    <div v-else class="chat-view bb-card">
      <div class="chat-header">
        <el-button text @click="router.push('/messages')">← 返回</el-button>
        <span class="chat-title">{{ composePeer ? '新私信' : '会话' }}</span>
      </div>
      <div v-if="chatLoading && messages.length === 0" class="chat-body"><LoadingState /></div>
      <div v-else-if="chatError" class="chat-body"><ErrorState :text="chatError" @retry="loadMessages(true)" /></div>
      <div v-else ref="chatBox" class="chat-body">
        <el-button v-if="hasOlder" size="small" text :loading="chatLoading" @click="loadMessages()">加载更早消息</el-button>
        <div
          v-for="m in messages"
          :key="m.id"
          class="msg-row"
          :class="{ mine: m.sender_id === auth.user?.user_id }"
        >
          <div class="msg-bubble">
            <div class="msg-meta">{{ m.sender_name }} · {{ m.create_time?.slice(11, 16) }}</div>
            <div class="msg-content">{{ m.content }}</div>
          </div>
        </div>
        <div v-if="messages.length === 0" class="cs-hint">还没有消息，打个招呼吧</div>
      </div>
      <div class="chat-input-row">
        <el-input v-model="input" placeholder="输入消息…" maxlength="2000" @keyup.enter="send" />
        <el-button type="primary" :loading="sending" @click="send">发送</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="less">
.thread-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 20px;
  margin-bottom: 10px;
  cursor: pointer;
}

.thread-main {
  flex: 1;
  min-width: 0;
}

.thread-top,
.thread-bottom {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.thread-name {
  font-weight: 600;
  color: var(--bb-text-main);
}

.thread-excerpt {
  color: var(--bb-text-tertiary);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}

.chat-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--bb-border);
}

.chat-body {
  height: 55vh;
  overflow-y: auto;
  padding: 16px;
}

.msg-row {
  display: flex;
  margin-bottom: 12px;

  &.mine {
    justify-content: flex-end;

    .msg-bubble {
      background: var(--bb-primary);
      color: #fff;

      .msg-meta {
        color: rgba(255, 255, 255, 0.8);
      }
    }
  }
}

.msg-bubble {
  max-width: 70%;
  background: var(--bb-bg-page);
  border-radius: 8px;
  padding: 8px 12px;

  .msg-meta {
    font-size: 12px;
    color: var(--bb-text-tertiary);
    margin-bottom: 4px;
  }

  .msg-content {
    white-space: pre-wrap;
    word-break: break-word;
  }
}

.chat-input-row {
  display: flex;
  gap: 10px;
  padding: 12px 16px;
  border-top: 1px solid var(--bb-border);
}
</style>
