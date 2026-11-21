<script setup lang="ts">
// 我的内容管理：按类型与状态筛选自己的问题/回答/文章，展示审核与删除状态（PRD §4.2/§4.3）
import { ref, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { fetchMyContent, type MyContentItem } from '@/api/question'
import { useAuthStore } from '@/stores/auth'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const contentType = ref<'question' | 'answer' | 'article'>(
  (route.query.type as 'question' | 'answer' | 'article') || 'question',
)
const statusFilter = ref<'published' | 'pending' | 'hidden' | 'deleted' | ''>(
  (route.query.status as 'published' | 'pending' | 'hidden' | 'deleted') || '',
)

const items = ref<MyContentItem[]>([])
const loading = ref(false)
const error = ref('')
const cursor = ref('')
const hasMore = ref(false)

const typeTabs = [
  { key: 'question', label: '问题' },
  { key: 'answer', label: '回答' },
  { key: 'article', label: '文章' },
] as const

const statusOptions = [
  { key: '', label: '全部' },
  { key: 'published', label: '公开' },
  { key: 'pending', label: '审核中' },
  { key: 'hidden', label: '退回/隐藏' },
  { key: 'deleted', label: '已删除' },
] as const

// 状态 → 展示标签
function stateLabel(it: MyContentItem): string {
  if (it.status === 0) return '已删除'
  if (it.moderation_status === 0) return '审核中'
  if (it.moderation_status === 2) return '审核退回'
  if (it.moderation_status === 3) return '已隔离'
  if (contentType.value === 'question' && it.resolution_status === 1) return '已解决'
  if (contentType.value === 'question' && it.resolution_status === 2) return '已关闭'
  return '公开'
}

function stateTagType(it: MyContentItem): 'success' | 'info' | 'warning' | 'danger' {
  if (it.status === 0) return 'info'
  if (it.moderation_status === 0 || it.moderation_status === 2) return 'warning'
  if (it.moderation_status === 3) return 'danger'
  if (contentType.value === 'question' && it.resolution_status === 1) return 'success'
  if (contentType.value === 'question' && it.resolution_status === 2) return 'info'
  return 'success'
}

function itemLink(it: MyContentItem): string {
  if (it.content_type === 'question') return `/questions/${it.content_id}`
  if (it.content_type === 'article') return `/articles/${it.content_id}`
  // 回答跳到所属问题（列表接口已带所属问题标题，但链接只有回答 id；
  // 后端 /me/content 未返回 question_id，回答条目跳转到创作者个人主页对应的回答上下文）
  return ''
}

function switchType(t: 'question' | 'answer' | 'article') {
  if (contentType.value === t) return
  contentType.value = t
  syncUrl()
}

function switchStatus(s: 'published' | 'pending' | 'hidden' | 'deleted' | '') {
  if (statusFilter.value === s) return
  statusFilter.value = s
  syncUrl()
}

function syncUrl() {
  router.replace({
    query: {
      type: contentType.value,
      ...(statusFilter.value ? { status: statusFilter.value } : {}),
    },
  })
}

async function load(reset = false) {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    if (reset) {
      cursor.value = ''
      items.value = []
    }
    const res = await fetchMyContent({
      type: contentType.value,
      status: (statusFilter.value || undefined) as 'published' | 'pending' | 'hidden' | 'deleted' | undefined,
      cursor: cursor.value || undefined,
      limit: 20,
    })
    items.value = [...items.value, ...res.items]
    cursor.value = res.next_cursor
    hasMore.value = res.has_more
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

watch([contentType, statusFilter], () => load(true))
onMounted(() => load(true))
</script>

<template>
  <div class="my-content-view">
    <div class="bb-card header-card">
      <h3 class="view-title">我的内容</h3>
      <div class="type-tabs">
        <button
          v-for="t in typeTabs"
          :key="t.key"
          class="type-tab"
          :class="{ active: contentType === t.key }"
          @click="switchType(t.key)"
        >
          {{ t.label }}
        </button>
      </div>
      <div class="status-filter">
        <button
          v-for="s in statusOptions"
          :key="s.key"
          class="status-chip"
          :class="{ active: statusFilter === s.key }"
          @click="switchStatus(s.key)"
        >
          {{ s.label }}
        </button>
      </div>
    </div>

    <div v-if="loading && items.length === 0" class="bb-card"><LoadingState /></div>
    <div v-else-if="error && items.length === 0" class="bb-card">
      <ErrorState :text="error" @retry="load(true)" />
    </div>
    <div v-else-if="items.length === 0" class="bb-card">
      <EmptyState text="这里还没有内容" />
    </div>
    <template v-else>
      <div v-for="it in items" :key="`${it.content_type}-${it.content_id}`" class="bb-card content-item">
        <div class="item-main">
          <component
            :is="itemLink(it) ? 'router-link' : 'div'"
            :to="itemLink(it) || undefined"
            class="item-title"
          >
            {{ it.title || it.excerpt || '（无标题）' }}
          </component>
          <p v-if="it.title && it.excerpt" class="item-excerpt">{{ it.excerpt }}</p>
        </div>
        <div class="item-side">
          <el-tag :type="stateTagType(it)" size="small" effect="light">{{ stateLabel(it) }}</el-tag>
          <span class="item-time">{{ it.create_time?.slice(0, 10) }}</span>
        </div>
      </div>
      <div class="load-more">
        <el-button v-if="hasMore" :loading="loading" plain style="width: 100%" @click="load()">
          加载更多
        </el-button>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.header-card {
  padding: 20px 24px;
}

.view-title {
  margin: 0 0 12px;
}

.type-tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}

.type-tab {
  border: 1px solid var(--bb-border);
  background: transparent;
  padding: 6px 16px;
  border-radius: 16px;
  cursor: pointer;
  color: var(--bb-text-secondary);

  &.active {
    background: var(--bb-primary);
    border-color: var(--bb-primary);
    color: #fff;
  }
}

.status-filter {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.status-chip {
  border: none;
  background: transparent;
  padding: 4px 10px;
  border-radius: 12px;
  cursor: pointer;
  color: var(--bb-text-tertiary);
  font-size: 13px;

  &.active {
    background: var(--bb-primary);
    color: #fff;
  }
}

.content-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 16px 24px;
}

.item-title {
  color: var(--bb-text-main);
  font-weight: 600;
  text-decoration: none;
}

.item-excerpt {
  margin: 6px 0 0;
  color: var(--bb-text-tertiary);
  font-size: 13px;
}

.item-side {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
  flex-shrink: 0;
}

.item-time {
  color: var(--bb-text-tertiary);
  font-size: 12px;
}
</style>
