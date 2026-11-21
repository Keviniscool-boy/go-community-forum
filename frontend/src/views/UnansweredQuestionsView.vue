<script setup lang="ts">
// 待回答问题发现页（PRD §6.1）：话题筛选 + 最新/待回答时长排序 + keyset 分页
// 筛选写入 URL 可恢复；请求序号防护避免快速切换筛选时旧响应覆盖新列表
import { ref, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { fetchUnansweredQuestions } from '@/api/invitation'
import type { QuestionDetail } from '@/api/question'
import { fetchTopics, type Topic } from '@/api/notification'
import { useAuthStore } from '@/stores/auth'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const items = ref<QuestionDetail[]>([])
const topics = ref<Topic[]>([])
const loading = ref(false)
const error = ref('')
const cursor = ref('')
const hasMore = ref(false)
const firstLoaded = ref(false)

const topicId = ref((route.query.topic_id as string) || '')
const sort = ref(asSort(route.query.sort))

function asSort(v: unknown): 'latest' | 'oldest_unanswered' {
  const s = typeof v === 'string' ? v : 'latest'
  return s === 'oldest_unanswered' ? 'oldest_unanswered' : 'latest'
}

function syncUrl() {
  router.replace({
    query: {
      ...(topicId.value ? { topic_id: topicId.value } : {}),
      ...(sort.value !== 'latest' ? { sort: sort.value } : {}),
    },
  })
}

// 请求序号：只有最新一次请求允许写入列表
let requestSeq = 0

async function load(reset = false) {
  if (loading.value) return
  const seq = ++requestSeq
  loading.value = true
  error.value = ''
  try {
    if (reset) {
      cursor.value = ''
      items.value = []
    }
    const res = await fetchUnansweredQuestions({
      topic_id: topicId.value || undefined,
      sort: sort.value,
      cursor: cursor.value || undefined,
      limit: 20,
    })
    if (seq !== requestSeq) return // 过期响应丢弃
    items.value = reset ? res.items : [...items.value, ...res.items]
    cursor.value = res.next_cursor
    hasMore.value = res.has_more
    firstLoaded.value = true
  } catch (e) {
    if (seq === requestSeq) {
      error.value = e instanceof Error ? e.message : '加载失败'
    }
  } finally {
    if (seq === requestSeq) loading.value = false
  }
}

function switchSort(s: 'latest' | 'oldest_unanswered') {
  if (sort.value === s) return
  sort.value = s
  syncUrl()
}

function switchTopic(t: string) {
  if (topicId.value === t) return
  topicId.value = t
  syncUrl()
}

function resetFilters() {
  topicId.value = ''
  sort.value = 'latest'
  syncUrl()
}

// 筛选变化重置加载
watch([topicId, sort], () => load(true))

// 浏览器前进/后退
watch(
  () => route.query,
  (q) => {
    if (route.name !== 'unanswered') return
    const nextTopic = (q.topic_id as string) || ''
    const nextSort = asSort(q.sort)
    if (nextTopic !== topicId.value || nextSort !== sort.value) {
      topicId.value = nextTopic
      sort.value = nextSort
      load(true)
    }
  },
)

onMounted(async () => {
  load(true)
  try {
    topics.value = (await fetchTopics(20)) || []
  } catch {
    // 话题列表失败不阻塞页面
  }
})
</script>

<template>
  <div class="unanswered-view">
    <div class="bb-card header-card">
      <div class="header-row">
        <h3 class="view-title">等你来答</h3>
        <span class="header-sub">还没有公开回答的问题，分享你的见解</span>
      </div>
      <div class="filters">
        <div class="filter-group">
          <button class="chip" :class="{ active: topicId === '' }" @click="switchTopic('')">全部</button>
          <button
            v-for="t in topics"
            :key="t.topic_id"
            class="chip"
            :class="{ active: topicId === t.topic_id }"
            @click="switchTopic(t.topic_id)"
          >
            {{ t.name }}
          </button>
        </div>
        <div class="filter-group">
          <button class="chip" :class="{ active: sort === 'latest' }" @click="switchSort('latest')">最新</button>
          <button
            class="chip"
            :class="{ active: sort === 'oldest_unanswered' }"
            @click="switchSort('oldest_unanswered')"
          >
            待回答最久
          </button>
          <button v-if="topicId || sort !== 'latest'" class="chip reset" @click="resetFilters">重置</button>
        </div>
      </div>
    </div>

    <div v-if="loading && items.length === 0" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card"><ErrorState :text="error" @retry="load(true)" /></div>
    <div v-else-if="firstLoaded && items.length === 0" class="bb-card">
      <EmptyState text="暂时没有待回答的问题" />
      <p class="empty-hint">换个话题看看，或者
        <router-link class="link" :to="auth.isLoggedIn ? '/questions/ask' : { name: 'login', query: { redirect: route.fullPath } }">提出你的问题</router-link>
      </p>
    </div>
    <template v-else>
      <article v-for="q in items" :key="q.id" class="bb-card q-card">
        <router-link class="q-title" :to="`/questions/${q.id}`">{{ q.title }}</router-link>
        <p v-if="q.description" class="q-excerpt">{{ q.description }}</p>
        <div class="q-meta">
          <span class="q-author">{{ q.author_name || '社区用户' }}</span>
          <span>{{ q.create_time?.slice(0, 10) }}</span>
          <span>{{ q.follower_count }} 关注</span>
          <el-tag size="small" type="warning" effect="light">待回答</el-tag>
        </div>
      </article>
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

.header-row {
  display: flex;
  align-items: baseline;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.view-title {
  margin: 0;
}

.header-sub {
  color: var(--bb-text-tertiary);
  font-size: 13px;
}

.filters {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.filter-group {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.chip {
  border: 1px solid var(--bb-border);
  background: transparent;
  padding: 4px 12px;
  border-radius: 14px;
  cursor: pointer;
  color: var(--bb-text-secondary);
  font-size: 13px;

  &.active {
    background: var(--bb-primary);
    border-color: var(--bb-primary);
    color: #fff;
  }

  &.reset {
    border-style: dashed;
  }
}

.q-card {
  padding: 16px 20px;
  margin-bottom: 10px;
}

.q-title {
  display: block;
  font-size: 16px;
  font-weight: 600;
  color: var(--bb-text-main);

  &:hover {
    color: var(--bb-primary);
  }
}

.q-excerpt {
  margin: 6px 0 0;
  color: var(--bb-text-secondary);
  font-size: 13px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.q-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 8px;
  color: var(--bb-text-tertiary);
  font-size: 13px;
}

.empty-hint {
  color: var(--bb-text-tertiary);
  font-size: 13px;
  margin-top: 8px;

  .link {
    color: var(--bb-primary);
  }
}

.load-more {
  margin-bottom: 24px;
}
</style>
