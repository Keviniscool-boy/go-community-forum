<script setup lang="ts">
// 问题详情页：问题正文 + 回答列表（cursor 分页）+ 回答编辑器
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { View as ViewIcon, CaretTop } from '@element-plus/icons-vue'
import { fetchQuestionDetail, fetchAnswers, createAnswer, type QuestionDetail, type AnswerDetail } from '@/api/question'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const questionId = route.params.id as string

const question = ref<QuestionDetail | null>(null)
const loading = ref(false)
const error = ref('')

// 回答列表状态
const answers = ref<AnswerDetail[]>([])
const answersLoading = ref(false)
const answersError = ref('')
const cursor = ref('')
const hasMore = ref(false)
const order = ref<'hot' | 'latest'>('latest')
const answerCount = computed(() => question.value?.answer_count ?? answers.value.length)

// 回答编辑器状态
const answerContent = ref('')
const submitting = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    question.value = await fetchQuestionDetail(questionId)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function loadAnswers(reset = false) {
  if (answersLoading.value) return
  answersLoading.value = true
  answersError.value = ''
  try {
    if (reset) {
      cursor.value = ''
      answers.value = []
    }
    const res = await fetchAnswers(questionId, {
      cursor: cursor.value || undefined,
      order: order.value,
      limit: 20,
    })
    answers.value = [...answers.value, ...res.items]
    cursor.value = res.next_cursor
    hasMore.value = res.has_more
  } catch (e) {
    answersError.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    answersLoading.value = false
  }
}

function switchOrder(o: 'hot' | 'latest') {
  if (order.value === o) return
  order.value = o
  loadAnswers(true)
}

async function submitAnswer() {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  if (!answerContent.value.trim()) return ElMessage.warning('请输入回答内容')
  submitting.value = true
  try {
    await createAnswer(questionId, answerContent.value.trim())
    ElMessage.success('回答发布成功')
    answerContent.value = ''
    await load()
    await loadAnswers(true)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  load()
  loadAnswers(true)
})
</script>

<template>
  <div class="question-detail">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card">
      <ErrorState :text="error" @retry="load" />
    </div>
    <template v-else-if="question">
      <!-- 问题正文 -->
      <article class="bb-card q-card">
        <h1 class="q-title">{{ question.title }}</h1>
        <div class="q-meta">
          <span class="q-author">{{ question.author_name }}</span>
          <span>发布于 {{ question.create_time?.slice(0, 10) }}</span>
          <span class="q-views"><el-icon><ViewIcon /></el-icon>{{ question.view_count }} 次浏览</span>
        </div>
        <p v-if="question.description" class="q-description">{{ question.description }}</p>
        <div v-if="question.topics?.length" class="q-topics">
          <el-tag v-for="t in question.topics" :key="t.topic_id" size="small" effect="plain">{{ t.name }}</el-tag>
        </div>
        <div class="q-actions">
          <el-button plain size="small" @click="ElMessage.info('关注功能即将上线')">关注问题</el-button>
        </div>
      </article>

      <!-- 回答区 -->
      <div class="answers-header">
        <span class="answers-count">{{ answerCount }} 个回答</span>
        <div class="answers-order">
          <button class="order-btn" :class="{ active: order === 'latest' }" @click="switchOrder('latest')">最新</button>
          <button class="order-btn" :class="{ active: order === 'hot' }" @click="switchOrder('hot')">热门</button>
        </div>
      </div>

      <template v-if="answersLoading && answers.length === 0">
        <div class="bb-card"><LoadingState text="加载回答中…" /></div>
      </template>
      <div v-else-if="answersError && answers.length === 0" class="bb-card">
        <ErrorState :text="answersError" @retry="loadAnswers(true)" />
      </div>
      <div v-else-if="answers.length === 0" class="bb-card">
        <EmptyState text="还没有回答，写下你的答案" />
      </div>
      <template v-else>
        <article v-for="a in answers" :key="a.id" class="bb-card answer-card">
          <div class="a-vote">
            <el-icon><CaretTop /></el-icon>
            <span>{{ a.vote_count }}</span>
          </div>
          <div class="a-body">
            <div class="a-author">{{ a.author_name }}</div>
            <div class="a-content">{{ a.content }}</div>
            <div class="a-meta">{{ a.create_time?.slice(0, 10) }}</div>
          </div>
        </article>
        <div class="load-more">
          <el-button v-if="hasMore" :loading="answersLoading" plain style="width: 100%" @click="loadAnswers()">
            加载更多回答
          </el-button>
        </div>
      </template>

      <!-- 回答编辑器 -->
      <div class="bb-card answer-editor">
        <h3>写下你的回答</h3>
        <el-input
          v-model="answerContent"
          type="textarea"
          :rows="6"
          maxlength="100000"
          show-word-limit
          placeholder="分享你的见解、经验或思考…"
        />
        <el-button
          type="primary"
          style="margin-top: 12px"
          :loading="submitting"
          @click="submitAnswer"
        >
          发布回答
        </el-button>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.q-card {
  padding: 24px 28px;
}

.q-title {
  margin: 0;
  font-size: 22px;
}

.q-meta {
  display: flex;
  gap: 16px;
  align-items: center;
  margin: 12px 0;
  color: var(--bb-text-tertiary);
  font-size: 13px;
}

.q-views {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.q-description {
  white-space: pre-wrap;
  color: var(--bb-text-main);
  padding: 12px 0;
  border-top: 1px solid var(--bb-border);
  min-height: 20px;
}

.q-topics {
  display: flex;
  gap: 8px;
}

.q-actions {
  padding-top: 12px;
}

.answers-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 16px 0 8px;
}

.answers-count {
  font-weight: 600;
}

.answers-order {
  display: flex;
  gap: 8px;
}

.order-btn {
  border: none;
  background: transparent;
  padding: 4px 12px;
  font-size: 14px;
  color: var(--bb-text-secondary);
  cursor: pointer;
  border-radius: 4px;

  &.active {
    color: var(--bb-primary);
    font-weight: 600;
  }
}

.answer-card {
  display: flex;
  gap: 16px;
  padding: 16px 20px;
  margin-bottom: 12px;
}

.a-vote {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  color: var(--bb-primary);
  flex-shrink: 0;
  padding-top: 4px;
}

.a-author {
  font-weight: 600;
  font-size: 14px;
}

.a-content {
  white-space: pre-wrap;
  color: var(--bb-text-main);
  margin: 8px 0;
}

.a-meta {
  color: var(--bb-text-tertiary);
  font-size: 12px;
}

.load-more {
  margin-bottom: 12px;
}

.answer-editor {
  padding: 20px 24px;
  margin-bottom: 24px;

  h3 {
    margin-top: 0;
  }
}
</style>
