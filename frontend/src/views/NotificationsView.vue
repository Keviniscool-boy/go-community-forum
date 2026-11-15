<script setup lang="ts">
// 通知中心：通知列表 + 全部已读
import { ref, onMounted } from 'vue'
import { useNotificationStore } from '@/stores/notification'
import { useAuthStore } from '@/stores/auth'
import { useRouter } from 'vue-router'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'

const store = useNotificationStore()
const auth = useAuthStore()
const router = useRouter()

const readAllLoading = ref(false)

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

// 通知跳转：关注类跳用户主页，其余按目标类型跳转
function targetLink(n: { type: number; target_type: number; target_id: string; actor_id: string }): string {
  if (n.type === 3) return `/users/${n.target_id || n.actor_id}`
  if (n.target_type === 1) return `/questions/${n.target_id}`
  return `/questions/${n.target_id}`
}

const typeIcon = (t: number) =>
  ({ 1: '👍', 2: '💬', 3: '➕', 4: '⭐' })[t] || '🔔'
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
          <span class="notif-text"><strong>{{ n.actor_name }}</strong> {{ n.excerpt }}</span>
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
