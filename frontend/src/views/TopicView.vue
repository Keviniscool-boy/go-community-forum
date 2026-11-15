<script setup lang="ts">
// 话题详情页：话题信息 + 话题下的问题
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { fetchTopicDetail, followTopic, unfollowTopic, type Topic, type TopicQuestion } from '@/api/notification'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const topicId = route.params.id as string

const topic = ref<Topic | null>(null)
const questions = ref<TopicQuestion[]>([])
const loading = ref(false)
const error = ref('')
const followed = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await fetchTopicDetail(topicId)
    topic.value = res.topic
    questions.value = res.questions || []
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function onFollow() {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  if (!topic.value) return
  try {
    if (followed.value) {
      await unfollowTopic(topicId)
      followed.value = false
      topic.value.follower_count -= 1
    } else {
      await followTopic(topicId)
      followed.value = true
      topic.value.follower_count += 1
      ElMessage.success('已关注话题')
    }
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

onMounted(load)
</script>

<template>
  <div class="topic-view">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card">
      <ErrorState :text="error" @retry="load" />
    </div>
    <template v-else-if="topic">
      <div class="bb-card topic-header">
        <div class="topic-info">
          <h2 class="topic-name"># {{ topic.name }}</h2>
          <p class="topic-desc">{{ topic.description || '暂无话题简介' }}</p>
          <span class="topic-count">{{ topic.follower_count }} 人关注 · {{ questions.length }} 个问题</span>
        </div>
        <el-button :type="followed ? 'default' : 'primary'" round @click="onFollow">
          {{ followed ? '已关注' : '+ 关注话题' }}
        </el-button>
      </div>

      <div v-if="questions.length === 0" class="bb-card"><EmptyState text="该话题下还没有问题" /></div>
      <article v-for="q in questions" :key="q.id" class="bb-card content-card">
        <router-link class="content-title" :to="`/questions/${q.id}`">{{ q.title }}</router-link>
        <div class="content-meta">{{ q.answer_count }} 回答 · {{ q.view_count }} 浏览 · {{ q.create_time?.slice(0, 10) }}</div>
      </article>
    </template>
  </div>
</template>

<style scoped lang="less">
.topic-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 24px 28px;
  margin-bottom: 12px;
}

.topic-name {
  margin: 0;
  color: var(--bb-primary);
}

.topic-desc {
  margin: 8px 0;
  color: var(--bb-text-secondary);
  font-size: 14px;
}

.topic-count {
  color: var(--bb-text-tertiary);
  font-size: 13px;
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

.content-meta {
  color: var(--bb-text-tertiary);
  font-size: 13px;
}
</style>
