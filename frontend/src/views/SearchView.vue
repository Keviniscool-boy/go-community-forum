<script setup lang="ts">
// 搜索页：关键词、类型筛选（含文章/专栏）、排序、结果列表、空/错误状态
// 搜索条件（q/scope/sort）写入 URL，可复制、浏览器返回可用（PRD §5.1）
import { ref, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { search, type SearchResultItem } from '@/api/search'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'

const route = useRoute()
const router = useRouter()

// 搜索历史（PRD F9）：localStorage 最近 10 条，聚焦下拉、逐条删除、清空
const HISTORY_KEY = 'bb_search_history'
const history = ref<string[]>([])
const historyOpen = ref(false)
try {
  history.value = JSON.parse(localStorage.getItem(HISTORY_KEY) || '[]')
} catch {
  history.value = []
}

function saveHistory(kw: string) {
  if (!kw) return
  const next = [kw, ...history.value.filter((h) => h !== kw)].slice(0, 10)
  history.value = next
  localStorage.setItem(HISTORY_KEY, JSON.stringify(next))
}

function removeHistory(kw: string) {
  history.value = history.value.filter((h) => h !== kw)
  localStorage.setItem(HISTORY_KEY, JSON.stringify(history.value))
}

function clearHistory() {
  history.value = []
  localStorage.removeItem(HISTORY_KEY)
  historyOpen.value = false
}

function useHistory(kw: string) {
  keyword.value = kw
  historyOpen.value = false
  submit()
}

const keyword = ref((route.query.q as string) || '')
const scope = ref(asScope(route.query.scope))
const sort = ref(asSort(route.query.sort))
const items = ref<SearchResultItem[]>([])
const loading = ref(false)
const error = ref('')
const searched = ref(false)
const finished = ref(false)
const cursor = ref('')

const typeLabels: Record<string, string> = {
  question: '问题', answer: '回答', article: '文章', column: '专栏', user: '用户', topic: '话题',
}

function asScope(v: unknown): string {
  const s = typeof v === 'string' ? v : 'all'
  return ['all', 'question', 'answer', 'article', 'column', 'user', 'topic'].includes(s) ? s : 'all'
}

function asSort(v: unknown): string {
  const s = typeof v === 'string' ? v : 'relevance'
  return s === 'latest' ? 'latest' : 'relevance'
}

function syncUrl() {
  router.replace({
    query: {
      ...(keyword.value.trim() ? { q: keyword.value.trim() } : {}),
      ...(scope.value !== 'all' ? { scope: scope.value } : {}),
      ...(sort.value !== 'relevance' ? { sort: sort.value } : {}),
    },
  })
}

async function load(reset = false) {
  if (!keyword.value.trim()) return
  loading.value = true
  error.value = ''
  try {
    if (reset) {
      cursor.value = ''
      items.value = []
      finished.value = false
      searched.value = true
    }
    const res = await search(keyword.value.trim(), scope.value, sort.value, cursor.value)
    items.value = reset ? res.items : [...items.value, ...res.items]
    cursor.value = res.next_cursor
    finished.value = !res.has_more
  } catch (e) {
    error.value = e instanceof Error ? e.message : '搜索失败'
  } finally {
    loading.value = false
  }
}

function itemLink(item: SearchResultItem): string {
  switch (item.document_type) {
    case 'question': return `/questions/${item.document_id}`
    case 'answer': return `/questions/${item.question_id || item.document_id}`
    case 'article': return `/articles/${item.document_id}`
    case 'column': return `/columns/${item.document_id}`
    case 'user': return `/users/${item.document_id}`
    case 'topic': return `/topics/${item.document_id}`
  }
  return '/'
}

// scope/sort 变化：同步 URL 并重查
watch([scope, sort], () => {
  syncUrl()
  load(true)
})

// 浏览器前进/后退：URL 变化驱动查询
watch(
  () => route.query,
  (q) => {
    if (route.name !== 'search') return
    const nextScope = asScope(q.scope)
    const nextSort = asSort(q.sort)
    const nextKeyword = (q.q as string) || ''
    if (nextKeyword !== keyword.value || nextScope !== scope.value || nextSort !== sort.value) {
      keyword.value = nextKeyword
      scope.value = nextScope
      sort.value = nextSort
      if (keyword.value) load(true)
    }
  },
)

onMounted(() => {
  if (keyword.value) load(true)
})

function submit() {
  syncUrl()
  saveHistory(keyword.value.trim())
  historyOpen.value = false
  load(true)
}
</script>

<template>
  <div class="search-view">
    <div class="bb-card search-bar">
      <div class="search-input-wrap">
        <el-input
          v-model="keyword"
          size="large"
          placeholder="搜索问题、回答、文章、专栏、用户、话题…"
          @keyup.enter="submit"
          @focus="historyOpen = history.length > 0"
          @blur="historyOpen = false"
        />
        <div v-if="historyOpen && history.length" class="history-dropdown">
          <div class="history-head">
            <span>搜索历史</span>
            <button class="history-clear" type="button" @mousedown.prevent="clearHistory">清空</button>
          </div>
          <div v-for="h in history" :key="h" class="history-item" @mousedown.prevent="useHistory(h)">
            <span class="history-kw">{{ h }}</span>
            <button class="history-del" type="button" @mousedown.stop.prevent="removeHistory(h)">×</button>
          </div>
        </div>
      </div>
      <el-button type="primary" size="large" :loading="loading" @click="submit">搜索</el-button>
    </div>

    <div class="bb-card filters">
      <el-radio-group v-model="scope" size="small">
        <el-radio-button value="all">全部</el-radio-button>
        <el-radio-button value="question">问题</el-radio-button>
        <el-radio-button value="answer">回答</el-radio-button>
        <el-radio-button value="article">文章</el-radio-button>
        <el-radio-button value="column">专栏</el-radio-button>
        <el-radio-button value="user">用户</el-radio-button>
        <el-radio-button value="topic">话题</el-radio-button>
      </el-radio-group>
      <el-radio-group v-model="sort" size="small">
        <el-radio-button value="relevance">相关度</el-radio-button>
        <el-radio-button value="latest">最新</el-radio-button>
      </el-radio-group>
    </div>

    <div v-if="loading && items.length === 0" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card"><ErrorState :text="error" @retry="load(true)" /></div>
    <div v-else-if="searched && items.length === 0" class="bb-card">
      <EmptyState text="没有找到相关内容">
        <p class="empty-hint">试试更换关键词，或切换到"全部"类型再搜索</p>
      </EmptyState>
    </div>
    <template v-else>
      <article v-for="item in items" :key="`${item.document_type}-${item.document_id}`" class="bb-card result-card">
        <router-link class="result-title" :to="itemLink(item)">{{ item.title }}</router-link>
        <div class="result-meta">
          <el-tag size="small" effect="plain">{{ typeLabels[item.document_type] || item.document_type }}</el-tag>
          <el-tag v-if="item.solved" size="small" type="success" effect="light">已解决</el-tag>
          <p class="result-excerpt">{{ item.excerpt }}</p>
        </div>
      </article>
      <div class="load-more">
        <el-button v-if="finished && items.length > 0" disabled plain style="width: 100%">没有更多结果</el-button>
        <el-button v-else-if="items.length > 0" :loading="loading" plain style="width: 100%" @click="load()">加载更多</el-button>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.search-input-wrap {
  position: relative;
  flex: 1;
}

.history-dropdown {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  z-index: 20;
  background: #fff;
  border: 1px solid var(--bb-border);
  border-radius: 6px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
  padding: 6px 0;
}

.history-head {
  display: flex;
  justify-content: space-between;
  padding: 4px 12px;
  color: var(--bb-text-tertiary);
  font-size: 12px;
}

.history-clear {
  border: none;
  background: transparent;
  color: var(--bb-primary);
  cursor: pointer;
  font-size: 12px;
}

.history-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 12px;
  cursor: pointer;
  font-size: 14px;
  color: var(--bb-text-main);

  &:hover {
    background: var(--bb-bg-page);
  }
}

.history-del {
  border: none;
  background: transparent;
  color: var(--bb-text-tertiary);
  cursor: pointer;
  font-size: 14px;
}

.search-bar {
  display: flex;
  gap: 12px;
  padding: 16px 20px;
  margin-bottom: 12px;
}

.filters {
  display: flex;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px;
  padding: 10px 16px;
  margin-bottom: 12px;
}

.result-card {
  padding: 14px 20px;
  margin-bottom: 10px;
}

.result-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--bb-text-main);

  &:hover { color: var(--bb-primary); }
}

.result-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
}

.result-excerpt {
  margin: 0;
  color: var(--bb-text-secondary);
  font-size: 13px;
}

.empty-hint {
  color: var(--bb-text-tertiary);
  font-size: 13px;
  margin-top: 8px;
}

.load-more { margin-bottom: 24px; }
</style>
