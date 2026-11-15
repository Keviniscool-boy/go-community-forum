<script setup lang="ts">
// 帖子详情页
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { fetchPostDetail } from '@/api/post'
import type { PostDetail } from '@/types/api'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'

const route = useRoute()
const post = ref<PostDetail | null>(null)
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    post.value = await fetchPostDetail(route.params.id as string)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="post-detail">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card">
      <ErrorState :text="error" @retry="load" />
    </div>
    <article v-else-if="post" class="bb-card detail-card">
      <h1 class="detail-title">{{ post.title }}</h1>
      <div class="detail-meta">
        <span>{{ post.author_name }}</span>
        <span v-if="post.name">发表于「{{ post.name }}」</span>
        <span>{{ post.create_time?.replace('T', ' ').slice(0, 19) }}</span>
      </div>
      <div class="detail-content">{{ post.content }}</div>
    </article>
  </div>
</template>

<style scoped lang="less">
.detail-card {
  padding: 24px 28px;
}

.detail-title {
  margin: 0;
  font-size: 22px;
}

.detail-meta {
  display: flex;
  gap: 16px;
  margin: 12px 0 20px;
  color: var(--bb-text-tertiary);
  font-size: 13px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--bb-border);
}

.detail-content {
  white-space: pre-wrap;
  color: var(--bb-text-main);
  min-height: 100px;
}
</style>
