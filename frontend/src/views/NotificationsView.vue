<script setup lang="ts">
// 通知中心：通知列表 + 全部已读 + 回答邀请接受/婉拒（PRD §6.3）
import { ref, onMounted } from 'vue'
import { useNotificationStore } from '@/stores/notification'
import { useAuthStore } from '@/stores/auth'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { acceptAnswerInvitation, declineAnswerInvitation } from '@/api/invitation'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'

interface InviteNotification {
  id: string
  type: number
  target_id: string // type 9 时为邀请 ID
  question_id?: string
  is_read: boolean
}

const store = useNotificationStore()
const auth = useAuthStore()
const router = useRouter()

const readAllLoading = ref(false)
const respondingId = ref('')

onMounted(() => {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: '/notifications' } })
    return
  }
  store.load(true)
})

async function onReadAll() {
  readAllLoading.value = true
  try {
    await store.readAll()
  } finally {
    readAllLoading.value = false
  }
}

// 通知跳转：关注类跳用户主页；邀请类用 question_id；其余按目标跳转
function targetLink(n: { type: number; target_type: number; target_id: string; actor_id: string; question_id?: string }): string {
  if (n.type === 3) return `/users/${n.target_id || n.actor_id}`
  if (n.type === 9 && n.question_id) return `/questions/${n.question_id}`
  return `/questions/${n.target_id}`
}

const typeIcon = (t: number) =>
  ({ 1: '👍', 2: '💬', 3: '➕', 4: '⭐', 6: '✅', 7: '↩️', 8: '📝', 9: '✉️', 10: '🙋', 11: '🙏' })[t] || '🔔'

// 邀请通知（type 9）内联接受/婉拒
async function respondInvite(inv: InviteNotification, decision: 'accept' | 'decline') {
  respondingId.value = inv.id
  try {
    if (decision === 'accept') {
      await acceptAnswerInvitation(inv.target_id)
      ElMessage.success('已接受邀请')
    } else {
      await declineAnswerInvitation(inv.target_id)
      ElMessage.success('已婉拒邀请')
    }
    await store.load(true)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    respondingId.value = ''
  }
}
</script>

<template>
  <div class="notifications-view">
    <div class="bb-card notif-header">
      <h3 class="notif-title">通知</h3>
      <el-button size="small" text :loading="readAllLoading" @click="onReadAll">全部标记已读</el-button>
    </div>

    <div v-if="store.loading && store.list.length === 0" class="bb-card"><LoadingState /></div>
    <div v-else-if="store.list.length === 0" class="bb-card"><EmptyState text="暂无通知" /></div>
    <template v-else>
      <div
        v-for="n in store.list"
        :key="n.id"
        class="bb-card notif-item"
        :class="{ unread: !n.is_read }"
        @click="router.push(targetLink(n))"
      >
        <span class="notif-icon">{{ typeIcon(n.type) }}</span>
        <div class="notif-body">
          <span class="notif-text">
            <strong>{{ n.actor_name }}</strong>{{ (n.agg_count || 1) > 1 ? ` 等 ${n.agg_count} 人` : '' }} {{ n.excerpt }}
          </span>
          <span v-if="n.type === 9" class="invite-actions">
            <el-button size="small" type="primary" :loading="respondingId === n.id" @click.stop="respondInvite(n, 'accept')">
              接受
            </el-button>
            <el-button size="small" :disabled="respondingId === n.id" @click.stop="respondInvite(n, 'decline')">
              婉拒
            </el-button>
            <el-button v-if="n.question_id" size="small" text @click.stop="router.push(`/questions/${n.question_id}`)">
              查看问题
            </el-button>
          </span>
          <span class="notif-time">{{ n.create_time?.slice(0, 16).replace('T', ' ') }}</span>
        </div>
        <span v-if="!n.is_read" class="unread-dot"></span>
      </div>
      <div class="load-more">
        <el-button v-if="!store.finished" :loading="store.loading" plain style="width: 100%" @click="store.load()">
          加载更多
        </el-button>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.invite-actions {
  display: flex;
  gap: 8px;
  margin-top: 6px;
}

.notif-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 20px;
  margin-bottom: 12px;
}

.notif-title {
  margin: 0;
}

.notif-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 20px;
  margin-bottom: 8px;
  cursor: pointer;

  &:hover {
    background: #fafafa;
  }

  &.unread {
    background: #f0f7ff;
  }
}

.notif-icon {
  font-size: 18px;
}

.notif-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.notif-text {
  font-size: 14px;
  color: var(--bb-text-main);
}

.notif-time {
  font-size: 12px;
  color: var(--bb-text-tertiary);
}

.unread-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--bb-primary);
  flex-shrink: 0;
}

.load-more {
  margin-bottom: 24px;
}
</style>
