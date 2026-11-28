<script setup lang="ts">
// 关注/粉丝列表页（PRD F1）：/users/:id/followers 与 /users/:id/following 共用
import { ref, onMounted, computed } from 'vue'
import { useRoute } from 'vue-router'
import { fetchFollowers, fetchFollowing, type FollowListUser } from '@/api/social'
import { followUser, unfollowUser } from '@/api/interaction'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const auth = useAuthStore()

const userId = route.params.id as string
const mode = computed(() => (route.name === 'user-following' ? 'following' : 'followers'))

const items = ref<FollowListUser[]>([])
const loading = ref(false)
const error = ref('')
const cursor = ref('')
const hasMore = ref(false)
const firstLoaded = ref(false)
const actionUserId = ref('')

async function load(reset = false) {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    if (reset) {
      cursor.value = ''
      items.value = []
    }
    const res =
      mode.value === 'followers'
        ? await fetchFollowers(userId, cursor.value)
        : await fetchFollowing(userId, cursor.value)
    items.value = reset ? res.items : [...items.value, ...res.items]
    cursor.value = res.next_cursor
    hasMore.value = res.has_more
    firstLoaded.value = true
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function toggleFollow(it: FollowListUser) {
  if (!auth.isLoggedIn) return
  actionUserId.value = it.user_id
  try {
    if (it.is_following) {
      await unfollowUser(it.user_id)
      it.is_following = false
    } else {
      await followUser(it.user_id)
      it.is_following = true
    }
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    actionUserId.value = ''
  }
}

onMounted(() => load(true))
</script>

<template>
  <div class="follow-list-view">
    <div class="bb-card header-card">
      <h3 class="view-title">{{ mode === 'followers' ? '关注者' : '关注了' }}</h3>
      <div class="mode-switch">
        <router-link class="mode-link" :class="{ active: mode === 'followers' }" :to="`/users/${userId}/followers`">
          关注者
        </router-link>
        <router-link class="mode-link" :class="{ active: mode === 'following' }" :to="`/users/${userId}/following`">
          关注了
        </router-link>
      </div>
    </div>

    <div v-if="loading && items.length === 0" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card"><ErrorState :text="error" @retry="load(true)" /></div>
    <div v-else-if="firstLoaded && items.length === 0" class="bb-card">
      <EmptyState :text="mode === 'followers' ? '还没有关注者' : '还没有关注任何人'" />
    </div>
    <template v-else>
      <div v-for="it in items" :key="it.user_id" class="bb-card user-row">
        <router-link class="user-link" :to="`/users/${it.user_id}`">
          <el-avatar :size="40" :src="it.avatar_url || undefined">{{ it.username?.slice(0, 1) }}</el-avatar>
          <span class="user-info">
            <span class="user-name">{{ it.username }}</span>
            <span class="user-bio">{{ it.bio || '暂无简介' }}</span>
          </span>
        </router-link>
        <el-button
          v-if="auth.isLoggedIn && auth.user?.user_id !== it.user_id"
          size="small"
          :type="it.is_following ? 'default' : 'primary'"
          round
          :loading="actionUserId === it.user_id"
          @click="toggleFollow(it)"
        >
          {{ it.is_following ? '已关注' : '+ 关注' }}
        </el-button>
      </div>
      <div class="load-more">
        <el-button v-if="hasMore" :loading="loading" plain style="width: 100%" @click="load()">加载更多</el-button>
        <el-button v-else-if="items.length > 0" disabled plain style="width: 100%">没有更多了</el-button>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.header-card {
  padding: 20px 24px;
  margin-bottom: 12px;
}

.view-title {
  margin: 0 0 10px;
}

.mode-switch {
  display: flex;
  gap: 8px;
}

.mode-link {
  padding: 6px 16px;
  border-radius: 16px;
  border: 1px solid var(--bb-border);
  color: var(--bb-text-secondary);
  text-decoration: none;
  font-size: 14px;

  &.active {
    background: var(--bb-primary);
    border-color: var(--bb-primary);
    color: #fff;
  }
}

.user-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 20px;
  margin-bottom: 10px;
}

.user-link {
  display: flex;
  align-items: center;
  gap: 12px;
  text-decoration: none;
  flex: 1;
  min-width: 0;
}

.user-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.user-name {
  color: var(--bb-text-main);
  font-weight: 600;
}

.user-bio {
  color: var(--bb-text-tertiary);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 420px;
}

.load-more {
  margin-bottom: 24px;
}
</style>
