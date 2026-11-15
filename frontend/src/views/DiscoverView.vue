<script setup lang="ts">
// 发现页：话题列表
import { ref, onMounted } from 'vue'
import { fetchTopics, type Topic } from '@/api/notification'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'

const topics = ref<Topic[]>([])
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    topics.value = (await fetchTopics(50)) || []
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="discover-view">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card">
      <ErrorState :text="error" @retry="load" />
    </div>
    <div v-else-if="topics.length === 0" class="bb-card"><EmptyState text="还没有话题" /></div>
    <div v-else class="bb-card topic-list">
      <h3 class="list-title">热门话题</h3>
      <router-link v-for="t in topics" :key="t.topic_id" class="topic-item" :to="`/topics/${t.topic_id}`">
        <span class="topic-name"># {{ t.name }}</span>
        <span class="topic-count">{{ t.follower_count }} 人关注</span>
      </router-link>
    </div>
  </div>
</template>

<style scoped lang="less">
.topic-list {
  padding: 20px 24px;
}

.list-title {
  margin: 0 0 12px;
}

.topic-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 4px;
  border-bottom: 1px solid var(--bb-border);
  color: var(--bb-text-main);

  &:last-child {
    border-bottom: none;
  }

  &:hover .topic-name {
    color: var(--bb-primary);
  }
}

.topic-name {
  font-weight: 500;
}

.topic-count {
  color: var(--bb-text-tertiary);
  font-size: 13px;
}
</style>
