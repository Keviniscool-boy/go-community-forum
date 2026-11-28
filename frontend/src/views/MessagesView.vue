<script setup lang="ts">
// 私信：会话列表（pending 请求可接受/拒绝）
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { listConversations, acceptConversation, declineConversation, closeConversation, type Conversation } from '@/api/messages'
import { useAuthStore } from '@/stores/auth'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'

const auth = useAuthStore()
const router = useRouter()
const conversations = ref<Conversation[]>([])
const loading = ref(true)

const stateLabels: Record<number, string> = {
  0: '待处理请求', 1: '进行中', 2: '已拒绝', 3: '已关闭',
}

async function load() {
  loading.value = true
  try {
    conversations.value = (await listConversations()) || []
  } finally {
    loading.value = false
  }
}

async function onAccept(c: Conversation) {
  try {
    await acceptConversation(c.id)
    ElMessage.success('已接受')
    load()
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message)
  }
}

async function onDecline(c: Conversation) {
  try {
    await declineConversation(c.id)
    ElMessage.success('已拒绝')
    load()
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message)
  }
}

async function onClose(c: Conversation) {
  try {
    await closeConversation(c.id)
    ElMessage.success('会话已关闭')
    await load()
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message)
  }
}

function openChat(c: Conversation) {
  if (c.state === 0 && c.initiator_id === auth.user?.user_id) {
    ElMessage.info('等待对方接受请求')
    return
  }
  if (c.state !== 1) return
  router.push(`/messages/${c.id}`)
}

onMounted(() => {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: '/messages' } })
    return
  }
  load()
})
</script>

<template>
  <div class="messages-view">
    <div class="bb-card header-row">
      <h3>私信</h3>
      <el-button size="small" plain @click="router.push('/settings/privacy')">隐私设置</el-button>
    </div>
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="conversations.length === 0" class="bb-card"><EmptyState text="暂无会话" /></div>
    <template v-else>
      <article
        v-for="c in conversations" :key="c.id"
        class="bb-card conv-card" :class="{ pending: c.state === 0 }"
        @click="c.state === 1 ? openChat(c) : undefined"
      >
        <div class="conv-main">
          <div class="conv-title">
            <strong>{{ c.peer_name }}</strong>
            <el-tag size="small" :type="c.state === 0 ? 'warning' : c.state === 1 ? 'success' : 'info'">
              {{ stateLabels[c.state] || '未知' }}
            </el-tag>
            <el-badge v-if="c.unread > 0 && c.state === 1" :value="c.unread" :max="99" />
          </div>
          <p class="conv-last">{{ c.last_body || '（无消息）' }}</p>
        </div>
        <div class="conv-actions" @click.stop>
          <template v-if="c.state === 0 && c.initiator_id !== auth.user?.user_id">
            <el-button size="small" type="primary" @click="onAccept(c)">接受</el-button>
            <el-button size="small" plain @click="onDecline(c)">拒绝</el-button>
          </template>
          <template v-else-if="c.state === 1">
            <el-button size="small" plain @click="openChat(c)">打开</el-button>
            <el-button size="small" text type="danger" @click.stop="onClose(c)">关闭</el-button>
          </template>
        </div>
      </article>
    </template>
  </div>
</template>

<style scoped lang="less">
.header-row {
  display: flex; justify-content: space-between; align-items: center;
  padding: 14px 20px; margin-bottom: 12px;
  h3 { margin: 0; }
}
.conv-card {
  display: flex; justify-content: space-between; align-items: center; gap: 12px;
  padding: 14px 20px; margin-bottom: 10px; cursor: pointer;
  &.pending { background: #fffbeb; }
}
.conv-main { flex: 1; min-width: 0; }
.conv-title { display: flex; align-items: center; gap: 8px; }
.conv-last {
  margin: 4px 0 0; color: var(--bb-text-tertiary); font-size: 13px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.conv-actions { display: flex; gap: 8px; flex-shrink: 0; }
</style>
