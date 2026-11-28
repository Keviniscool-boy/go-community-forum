<script setup lang="ts">
// 用户主页：基本信息、内容统计、关注按钮、提问与回答列表
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { fetchUserProfile, fetchUserQuestions, fetchUserAnswers, type UserProfile } from '@/api/user'
import { blockUser, unblockUser, fetchUserInteractions } from '@/api/social'
import { ElMessageBox } from 'element-plus'
import { followUser, unfollowUser } from '@/api/interaction'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const userId = route.params.id as string

const profile = ref<UserProfile | null>(null)
const loading = ref(false)
const error = ref('')
const tab = ref<'questions' | 'answers'>('questions')

const questions = ref<Array<{ id: string; title: string; answer_count: number; view_count: number; create_time: string }>>([])
const answers = ref<Array<{ id: string; question_id: string; content: string; vote_count: number; create_time: string; question_title?: string }>>([])
const listLoading = ref(false)

const defaultAvatar = computed(
  () =>
    'data:image/svg+xml;utf8,' +
    encodeURIComponent(
      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40"><circle cx="20" cy="20" r="20" fill="#d5e7fb"/><circle cx="20" cy="15" r="7" fill="#7db5e8"/><path d="M6 36a14 14 0 0 1 28 0z" fill="#7db5e8"/></svg>`,
    ),
)

const isSelf = computed(() => auth.user?.user_id === userId)

// 拉黑/取消拉黑（PRD F2）：与目标用户的互动状态在加载时获取
const isBlocked = ref(false)

async function loadInteractions() {
  if (!auth.isLoggedIn || isSelf.value) return
  try {
    const st = await fetchUserInteractions(userId)
    isBlocked.value = st.is_blocked
  } catch {
    // 忽略
  }
}

async function onToggleBlock() {
  try {
    if (isBlocked.value) {
      await unblockUser(userId)
      isBlocked.value = false
      ElMessage.success('已取消拉黑')
    } else {
      await ElMessageBox.confirm('拉黑后你们互相不可见、无法关注和私信，确定拉黑？', '拉黑用户', { type: 'warning' })
      await blockUser(userId)
      isBlocked.value = true
      ElMessage.success('已拉黑')
    }
    await load()
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    profile.value = await fetchUserProfile(userId)
    await loadTab()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function loadTab() {
  listLoading.value = true
  try {
    if (tab.value === 'questions') {
      questions.value = (await fetchUserQuestions(userId)) || []
    } else {
      answers.value = (await fetchUserAnswers(userId)) || []
    }
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    listLoading.value = false
  }
}

function switchTab(t: 'questions' | 'answers') {
  if (tab.value === t) return
  tab.value = t
  loadTab()
}

async function onFollow() {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  if (!profile.value) return
  try {
    if (profile.value.is_following) {
      await unfollowUser(userId)
      profile.value.is_following = false
      profile.value.follower_count -= 1
    } else {
      await followUser(userId)
      profile.value.is_following = true
      profile.value.follower_count += 1
      ElMessage.success('已关注')
    }
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

onMounted(() => {
  load()
  loadInteractions()
})
</script>

<template>
  <div class="user-profile">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card">
      <ErrorState :text="error" @retry="load" />
    </div>
    <template v-else-if="profile">
      <!-- 个人信息卡 -->
      <div class="bb-card profile-card">
        <el-avatar :size="72" :src="profile.avatar_url || defaultAvatar" />
        <div class="profile-info">
          <h2 class="profile-name">{{ profile.username }}</h2>
          <p class="profile-bio">{{ profile.bio || '这个人很懒，什么都没写' }}</p>
          <div class="profile-stats">
            <router-link class="stat-link" :to="`/users/${userId}/followers`">
              <strong>{{ profile.follower_count }}</strong> 关注者
            </router-link>
            <router-link class="stat-link" :to="`/users/${userId}/following`">
              <strong>{{ profile.following_count }}</strong> 关注了
            </router-link>
            <span><strong>{{ profile.question_count }}</strong> 提问</span>
            <span><strong>{{ profile.answer_count }}</strong> 回答</span>
          </div>
        </div>
        <div class="profile-action">
          <el-button
            v-if="!isSelf"
            :type="profile.is_following ? 'default' : 'primary'"
            round
            @click="onFollow"
          >
            {{ profile.is_following ? '已关注' : '+ 关注' }}
          </el-button>
          <el-button v-if="!isSelf" size="round" plain :type="isBlocked ? 'danger' : 'default'" @click="onToggleBlock">
            {{ isBlocked ? '已拉黑' : '拉黑' }}
          </el-button>
        </div>
      </div>

      <!-- 内容 Tab -->
      <div class="profile-tabs bb-card">
        <button class="tab-btn" :class="{ active: tab === 'questions' }" @click="switchTab('questions')">
          提问 {{ profile.question_count }}
        </button>
        <button class="tab-btn" :class="{ active: tab === 'answers' }" @click="switchTab('answers')">
          回答 {{ profile.answer_count }}
        </button>
      </div>

      <div v-if="listLoading" class="bb-card"><LoadingState /></div>
      <div v-else-if="tab === 'questions'">
        <div v-if="questions.length === 0" class="bb-card"><EmptyState text="还没有提问" /></div>
        <article v-for="q in questions" :key="q.id" class="bb-card content-card">
          <router-link class="content-title" :to="`/questions/${q.id}`">{{ q.title }}</router-link>
          <div class="content-meta">{{ q.answer_count }} 回答 · {{ q.view_count }} 浏览 · {{ q.create_time?.slice(0, 10) }}</div>
        </article>
      </div>
      <div v-else>
        <div v-if="answers.length === 0" class="bb-card"><EmptyState text="还没有回答" /></div>
        <article v-for="a in answers" :key="a.id" class="bb-card content-card">
          <router-link class="content-title" :to="`/questions/${a.question_id}`">
            {{ a.question_title || '回答了问题' }}
          </router-link>
          <p class="content-excerpt">{{ a.content }}</p>
          <div class="content-meta">{{ a.vote_count }} 赞同 · {{ a.create_time?.slice(0, 10) }}</div>
        </article>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.profile-card {
  display: flex;
  gap: 20px;
  padding: 24px 28px;
  margin-bottom: 12px;
  align-items: flex-start;
}

.profile-info {
  flex: 1;
  min-width: 0;
}

.profile-name {
  margin: 0;
  font-size: 20px;
}

.profile-bio {
  margin: 6px 0;
  color: var(--bb-text-secondary);
  font-size: 14px;
}

.stat-link {
  color: var(--bb-text-tertiary);
  text-decoration: none;

  &:hover {
    color: var(--bb-primary);
  }
}

.profile-stats {
  display: flex;
  gap: 20px;
  color: var(--bb-text-tertiary);
  font-size: 13px;

  strong {
    color: var(--bb-text-main);
    margin-right: 2px;
  }
}

.profile-action {
  flex-shrink: 0;
}

.profile-tabs {
  display: flex;
  padding: 4px;
  margin-bottom: 12px;
}

.tab-btn {
  flex: 1;
  padding: 10px 0;
  border: none;
  background: transparent;
  font-size: 15px;
  color: var(--bb-text-secondary);
  cursor: pointer;
  border-radius: 6px;

  &.active {
    color: var(--bb-primary);
    font-weight: 600;
    background: #e8f3ff;
  }
}

.content-card {
  padding: 16px 20px;
  margin-bottom: 12px;
}

.content-title {
  display: block;
  font-size: 16px;
  font-weight: 600;
  color: var(--bb-text-main);

  &:hover {
    color: var(--bb-primary);
  }
}

.content-excerpt {
  margin: 8px 0;
  color: var(--bb-text-secondary);
  font-size: 14px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.content-meta {
  color: var(--bb-text-tertiary);
  font-size: 13px;
}
</style>
