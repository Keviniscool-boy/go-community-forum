<script setup lang="ts">
// 文章列表页：公开文章 cursor 分页
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { listArticles, type ArticleDetail } from '@/api/article'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'

const router = useRouter()
const articles = ref<ArticleDetail[]>([])
const loading = ref(false)
const error = ref('')
const finished = ref(false)
const cursor = ref('')
const searchedOnce = ref(false)

async function load(reset = false) {
  if (loading.value) return
  if (!reset && finished.value) return
  loading.value = true
  error.value = ''
  try {
    if (reset) {
      cursor.value = ''
      finished.value = false
    }
    const res = await listArticles(cursor.value)
    if (reset) articles.value = []
    articles.value = [...articles.value, ...res.items]
    cursor.value = res.next_cursor
    finished.value = !res.has_more
    searchedOnce.value = true
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(() => load(true))
</script>

<template>
  <div class="articles-view">
    <div class="bb-card articles-header">
      <h3>文章</h3>
      <el-button type="primary" size="small" @click="router.push('/articles/new')">写文章</el-button>
    </div>

    <div v-if="loading && articles.length === 0" class="bb-card"><LoadingState /></div>
    <div v-else-if="error && articles.length === 0" class="bb-card">
      <ErrorState :text="error" @retry="load(true)" />
    </div>
    <div v-else-if="articles.length === 0 && searchedOnce" class="bb-card">
      <EmptyState text="还没有文章，写第一篇吧" />
    </div>
    <template v-else>
      <article v-for="a in articles" :key="a.id" class="bb-card article-card">
        <router-link class="article-title" :to="`/articles/${a.id}`">{{ a.title }}</router-link>
        <div class="article-meta">
          <router-link class="meta-author" :to="`/users/${a.author_id}`">{{ a.author_name }}</router-link>
          <el-tag v-if="a.column" size="small" effect="plain">{{ a.column.name }}</el-tag>
          <span class="meta-time">{{ a.create_time?.slice(0, 10) }}</span>
        </div>
        <p class="article-excerpt">{{ a.content }}</p>
      </article>
      <div class="load-more">
        <el-button v-if="!finished && articles.length > 0" :loading="loading" plain style="width: 100%" @click="load()">
          加载更多
        </el-button>
        <div v-else-if="finished" class="load-more-end">没有更多文章了</div>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.articles-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 20px;
  margin-bottom: 12px;
  h3 { margin: 0; }
}
.article-card { padding: 16px 20px; margin-bottom: 12px; }
.article-title {
  font-size: 17px; font-weight: 600; color: var(--bb-text-main);
  &:hover { color: var(--bb-primary); }
}
.article-meta {
  display: flex; align-items: center; gap: 12px; margin-top: 6px;
  font-size: 13px; color: var(--bb-text-tertiary);
  .meta-author { color: var(--bb-text-secondary); &:hover { color: var(--bb-primary); } }
}
.article-excerpt {
  margin: 10px 0 0; color: var(--bb-text-secondary); font-size: 14px;
  display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
}
.load-more { margin-bottom: 24px; }
.load-more-end { text-align: center; color: var(--bb-text-tertiary); font-size: 13px; padding: 8px 0; }
</style>
