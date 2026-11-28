<script setup lang="ts">
// 创作者中心：概览统计 + 内容管理
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { creatorOverview, creatorContents, type CreatorOverview } from '@/api/personalization'
import { useAuthStore } from '@/stores/auth'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'

const auth = useAuthStore()
const router = useRouter()

const overview = ref<CreatorOverview | null>(null)
const loading = ref(true)
const error = ref('')
const contentType = ref<'question' | 'answer' | 'article'>('question')
const contents = ref<Array<Record<string, unknown>>>([])
const listLoading = ref(false)

const modLabels: Record<number, string> = {
  0: '审核中', 1: '公开', 2: '已驳回', 3: '已隔离',
}

async function loadOverview() {
  try {
    overview.value = await creatorOverview()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  }
}

async function loadContents() {
  listLoading.value = true
  try {
    contents.value = (await creatorContents(contentType.value)) || []
  } finally {
    listLoading.value = false
  }
}

function switchType(t: 'question' | 'answer' | 'article') {
  contentType.value = t
  loadContents()
}

function contentLink(row: Record<string, unknown>): string {
  const id = row.id as string
  switch (contentType.value) {
    case 'question': return `/questions/${id}`
    case 'article': return `/articles/${id}`
    default: return `/questions/${row.question_id || id}`
  }
}

onMounted(async () => {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: '/creator' } })
    return
  }
  await loadOverview()
  await loadContents()
  loading.value = false
})
</script>

<template>
  <div class="creator-view">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card"><ErrorState :text="error" @retry="loadOverview" /></div>
    <template v-else>
      <h2 class="page-title">创作者中心</h2>
      <p class="as-of">数据截至 {{ overview?.as_of?.slice(0, 19).replace('T', ' ') }} UTC（近实时，可能有延迟）</p>

      <div class="metric-grid">
        <div v-for="(label, key) in { followers: '关注者', views: '文章阅读', likes_received: '收到赞同', comments_received: '收到评论', questions: '我的问题', answers: '我的回答', articles: '我的文章' }" :key="key" class="bb-card metric-card">
          <div class="metric-value">{{ overview?.[key as keyof CreatorOverview] ?? 0 }}</div>
          <div class="metric-label">{{ label }}</div>
        </div>
      </div>

      <div class="bb-card contents-card">
        <div class="type-tabs">
          <button class="type-btn" :class="{ active: contentType === 'question' }" @click="switchType('question')">问题</button>
          <button class="type-btn" :class="{ active: contentType === 'answer' }" @click="switchType('answer')">回答</button>
          <button class="type-btn" :class="{ active: contentType === 'article' }" @click="switchType('article')">文章</button>
        </div>
        <div v-if="listLoading" class="pad"><LoadingState /></div>
        <div v-else-if="contents.length === 0" class="pad empty">暂无内容</div>
        <div v-else>
          <div v-for="(row, i) in contents" :key="i" class="content-row">
            <router-link class="content-link" :to="contentLink(row)">
              {{ (row.title as string) || '(无标题)' }}
            </router-link>
            <el-tag size="small" :type="(row.moderation_status as number) === 1 ? 'success' : (row.moderation_status as number) === 0 ? 'warning' : 'danger'">
              {{ modLabels[row.moderation_status as number] || '未知' }}
            </el-tag>
            <span class="row-time">{{ (row.update_time as string)?.slice(0, 10) }}</span>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.page-title { margin: 0 0 8px; }
.as-of { color: var(--bb-text-tertiary); font-size: 12px; margin: 0 0 12px; }
.metric-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 12px; margin-bottom: 16px; }
.metric-card { padding: 16px; text-align: center; }
.metric-value { font-size: 24px; font-weight: 700; color: var(--bb-primary); }
.metric-label { color: var(--bb-text-secondary); font-size: 13px; margin-top: 4px; }
.contents-card { padding: 16px 20px; }
.type-tabs { display: flex; gap: 8px; margin-bottom: 10px; }
.type-btn {
  border: none; background: transparent; padding: 6px 14px;
  color: var(--bb-text-secondary); cursor: pointer; border-radius: 4px;
  &.active { color: var(--bb-primary); background: #e8f3ff; font-weight: 600; }
}
.pad { padding: 8px 0; }
.empty { color: var(--bb-text-tertiary); font-size: 13px; }
.content-row {
  display: flex; align-items: center; gap: 10px;
  padding: 10px 0; border-bottom: 1px solid var(--bb-border);
}
.content-link { flex: 1; color: var(--bb-text-main); &:hover { color: var(--bb-primary); } }
.row-time { color: var(--bb-text-tertiary); font-size: 12px; flex-shrink: 0; }
</style>
