<script setup lang="ts">
// 聊天页：消息分页 + 10s 有界轮询 + 已读上报 + 举报
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listMessages, sendMessage, markRead, reportMessage, closeConversation,
  type DirectMessage,
} from '@/api/messages'
import { genClientMessageId } from '@/api/messages'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const convId = route.params.id as string
const messages = ref<DirectMessage[]>([])
const loading = ref(true)
const error = ref('')
const input = ref('')
const sending = ref(false)
const closed = ref(false)
// 轮询定时器
let pollTimer: ReturnType<typeof setInterval> | null = null
const POLL_INTERVAL = 10000

async function load() {
  loading.value = true
  error.value = ''
  try {
    const msgs = (await listMessages(convId)) || []
    messages.value = msgs.slice().reverse() // 接口倒序返回，页面按时间正序展示
    await markLatest()
    scrollBottom()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function markLatest() {
  if (messages.value.length === 0) return
  const last = messages.value[messages.value.length - 1]
  if (last.sender_id !== auth.user?.user_id) {
    try {
      await markRead(convId, last.id)
    } catch {
      // 已读上报失败不影响浏览
    }
  }
}

function scrollBottom() {
  nextTick(() => {
    window.scrollTo(0, document.body.scrollHeight)
  })
}

async function pollOnce() {
  if (document.hidden) return // 页面不可见时跳过本轮
  try {
    const msgs = (await listMessages(convId)) || []
    const incoming = msgs.filter((m) => !messages.value.some((x) => x.id === m.id))
    if (incoming.length > 0) {
      messages.value = [...messages.value, ...incoming].sort((a, b) =>
        a.id < b.id ? -1 : 1,
      )
      await markLatest()
      scrollBottom()
    }
  } catch {
    // 轮询失败静默，下轮重试（指数退避可后续增强）
  }
}

async function onSend() {
  const body = input.value.trim()
  if (!body) return
  sending.value = true
  const clientMessageId = genClientMessageId()
  try {
    const msg = await sendMessage(convId, body, clientMessageId)
    if (!messages.value.some((m) => m.id === msg.id)) {
      messages.value.push(msg)
    }
    input.value = ''
    scrollBottom()
  } catch (e) {
    // 失败提示；client_message_id 已生成，用户重试时同一输入可换新键
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    sending.value = false
  }
}

async function onClose() {
  try {
    await ElMessageBox.confirm('关闭后双方都不能再发送消息，历史保留。', '关闭会话', { type: 'warning' })
  } catch { return }
  await closeConversation(convId)
  closed.value = true
  ElMessage.success('会话已关闭')
}

async function onReport(m: DirectMessage) {
  try {
    await ElMessageBox.prompt('举报原因（垃圾/辱骂/侵权等）', '举报消息', { inputPattern: /.+/ })
  } catch { return }
  try {
    await reportMessage(m.id, 'abuse')
    ElMessage.success('举报已提交')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

onMounted(() => {
  load().then(() => {
    pollTimer = setInterval(pollOnce, POLL_INTERVAL) // 有界轮询（PRD 3.7）
  })
})
onBeforeUnmount(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div class="chat-view">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card"><ErrorState :text="error" @retry="load" /></div>
    <template v-else>
      <div class="bb-card chat-header">
        <el-button size="small" text @click="router.push('/messages')">返回列表</el-button>
        <el-button size="small" plain type="danger" @click="onClose">关闭会话</el-button>
      </div>
      <div v-if="closed" class="bb-card closed-tip">会话已关闭，历史消息保留。</div>
      <div class="msg-list">
        <div
          v-for="m in messages" :key="m.id"
          class="msg-row" :class="{ mine: m.sender_id === auth.user?.user_id }"
        >
          <div class="msg-bubble">
            <p class="msg-body">{{ m.body }}</p>
            <div class="msg-foot">
              <span>{{ m.create_time?.slice(11, 16) }}</span>
              <el-dropdown v-if="m.sender_id !== auth.user?.user_id" trigger="click" @command="onReport(m)">
                <span class="msg-more">…</span>
                <template #dropdown>
                  <el-dropdown-item command="">举报</el-dropdown-item>
                </template>
              </el-dropdown>
            </div>
          </div>
        </div>
      </div>
      <div class="bb-card input-row">
        <el-input
          v-model="input"
          :disabled="closed"
          maxlength="2000"
          placeholder="输入消息（纯文本，1-2000 字符）"
          @keyup.enter="onSend"
        />
        <el-button type="primary" :loading="sending" :disabled="closed" @click="onSend">发送</el-button>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.chat-header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 12px 20px; margin-bottom: 12px;
}
.closed-tip { padding: 12px 20px; margin-bottom: 12px; color: var(--bb-text-secondary); }
.msg-list { min-height: 300px; }
.msg-row {
  display: flex; margin-bottom: 10px;
  &.mine { justify-content: flex-end; }
}
.msg-bubble {
  max-width: 70%;
  background: #fff; border: 1px solid var(--bb-border);
  border-radius: var(--bb-radius); padding: 10px 14px;
  .mine & { background: #e8f3ff; border-color: #c5e0ff; }
}
.msg-body { margin: 0; white-space: pre-wrap; word-break: break-word; }
.msg-foot {
  display: flex; justify-content: space-between; align-items: center;
  color: var(--bb-text-tertiary); font-size: 11px; margin-top: 4px;
}
.msg-more { cursor: pointer; }
.input-row {
  position: sticky; bottom: 0;
  display: flex; gap: 10px; padding: 12px;
}
@media (max-width: 768px) {
  .msg-bubble { max-width: 85%; }
}
</style>
