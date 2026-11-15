<script setup lang="ts">
// 搜索页：关键词、类型筛选、排序、结果列表、空/错误状态
import { ref, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { search, type SearchResultItem } from '@/api/search'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'

const route = useRoute()
const router = useRouter()

const keyword = ref((route.query.q as string) || '')
const scope = ref('all')
const sort = ref('relevance')
const items = ref<SearchResultItem[]>([])
const loading = ref(false)
const error = ref('')
const searched = ref(false)
const finished = ref(false)
const cursor = ref('')

const typeLabels: Record<string, string> = {
  question: '问题', answer: '回答', user: '用户', topic: '话题',
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
    case 'answer': return `/questions/${item.document_id}`
    case 'user': return `/users/${item.document_id}`
    case 'topic': return `/topics/${item.document_id}`
  }
  return '/'
}

watch([scope, sort], () => load(true))

onMounted(() => {
  if (keyword.value) load(true)
})

function submit() {
  router.replace({ query: { q: keyword.value } })
  load(true)
}
</script>

<template>
  <div class="search-view">
    <div class="bb-card search-bar">
      <el-input
        v-model="keyword"
        size="large"
        placeholder="搜索问题、回答、用户、话题…"
        @keyup.enter="submit"
      />
      <el-button type="primary" size="large" :loading="loading" @click="submit">搜索</el-button>
    </div>

    <div class="bb-card filters">
      <el-radio-group v-model="scope" size="small">
        <el-radio-button value="all">全部</el-radio-button>
        <el-radio-button value="question">问题</el-radio-button>
        <el-radio-button value="answer">回答</el-radio-button>
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
      <EmptyState text="没有找到相关内容" />
    </div>
    <template v-else>
      <article v-for="item in items" :key="`${item.document_type}-${item.document_id}`" class="bb-card result-card">
        <router-link class="result-title" :to="itemLink(item)">{{ item.title }}</router-link>
        <div class="result-meta">
          <el-tag size="small" effect="plain">{{ typeLabels[item.document_type] || item.document_type }}</el-tag>
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

.result-excerpt {
  margin: 6px 0 0;
  color: var(--bb-text-secondary);
  font-size: 13px;
}

.load-more { margin-bottom: 24px; }
</style>
