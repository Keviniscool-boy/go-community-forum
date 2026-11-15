<script setup lang="ts">
// 问题详情页：问题正文 + 关注 + 回答列表（投票/收藏/评论）+ 回答编辑器
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { View as ViewIcon, CaretTop, Star, ChatDotRound } from '@element-plus/icons-vue'
import { fetchQuestionDetail, fetchAnswers, createAnswer, type QuestionDetail, type AnswerDetail } from '@/api/question'
import { voteAnswer, collectAnswer, uncollectAnswer, followQuestion, unfollowQuestion, fetchQuestionInteractions, fetchAnswerInteractions, fetchComments, createComment, type CommentDetail } from '@/api/interaction'
import { followUser, unfollowUser } from '@/api/interaction'
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

// 问题互动状态
const followed = ref(false)
const authorFollowed = ref(false)

// 回答列表状态
const answers = ref<AnswerDetail[]>([])
const answersLoading = ref(false)
const answersError = ref('')
const cursor = ref('')
const hasMore = ref(false)
const order = ref<'hot' | 'latest'>('latest')
const answerCount = computed(() => question.value?.answer_count ?? answers.value.length)

// 每条回答的互动状态（vote: 1/-1/0，collected）
const voteMap = ref<Record<string, number>>({})
const collectMap = ref<Record<string, boolean>>({})

// 问题评论
const comments = ref<CommentDetail[]>([])
const commentLoading = ref(false)
const commentsFinished = ref(false)
const commentCursor = ref('')
const commentInput = ref('')
const commentSubmitting = ref(false)

// 回答编辑器状态
const answerContent = ref('')
const submitting = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    question.value = await fetchQuestionDetail(questionId)
    // 登录后拉取互动状态
    if (auth.isLoggedIn) {
      try {
        const st = await fetchQuestionInteractions(questionId)
        followed.value = st.followed
      } catch {
        // 互动状态拉取失败不阻塞页面
      }
    }
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
    // 批量拉取互动状态
    if (auth.isLoggedIn) {
      for (const a of res.items) {
        try {
          const st = await fetchAnswerInteractions(a.id)
          voteMap.value[a.id] = st.vote
          collectMap.value[a.id] = st.collected
        } catch {
          // 忽略
        }
      }
    }
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

// 投票（同一回答再次点击同一方向 = 取消）
async function onVote(a: AnswerDetail, direction: 1 | -1) {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  const current = voteMap.value[a.id] || 0
  const next = current === direction ? 0 : direction
  try {
    await voteAnswer(a.id, next as 1 | -1 | 0)
    voteMap.value[a.id] = next
    // 本地同步计数
    const delta = (next === 1 ? 1 : next === -1 ? -1 : 0) - (current === 1 ? 1 : current === -1 ? -1 : 0)
    a.vote_count += delta
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

async function onCollect(a: AnswerDetail) {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  const collected = collectMap.value[a.id]
  try {
    if (collected) {
      await uncollectAnswer(a.id)
      a.collect_count -= 1
      collectMap.value[a.id] = false
    } else {
      await collectAnswer(a.id)
      a.collect_count += 1
      collectMap.value[a.id] = true
      ElMessage.success('已收藏')
    }
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

async function onFollowQuestion() {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  try {
    if (followed.value) {
      await unfollowQuestion(questionId)
      followed.value = false
      if (question.value) question.value.follower_count -= 1
    } else {
      await followQuestion(questionId)
      followed.value = true
      if (question.value) question.value.follower_count += 1
      ElMessage.success('已关注问题')
    }
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

async function onFollowAuthor() {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  if (!question.value) return
  try {
    if (authorFollowed.value) {
      await unfollowUser(question.value.author_id)
      authorFollowed.value = false
    } else {
      await followUser(question.value.author_id)
      authorFollowed.value = true
      ElMessage.success('已关注')
    }
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

// 评论
async function loadComments(reset = false) {
  if (commentLoading.value) return
  if (!reset && commentsFinished.value) return
  commentLoading.value = true
  try {
    if (reset) {
      commentCursor.value = ''
      comments.value = []
    }
    const res = await fetchComments('question', questionId, commentCursor.value)
    if (reset) comments.value = []
    comments.value = [...comments.value, ...res.items]
    commentCursor.value = res.next_cursor
    commentsFinished.value = !res.has_more
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    commentLoading.value = false
  }
}

async function submitComment() {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  if (!commentInput.value.trim()) return ElMessage.warning('请输入评论内容')
  commentSubmitting.value = true
  try {
    await createComment('question', questionId, commentInput.value.trim())
    commentInput.value = ''
    ElMessage.success('评论已发布')
    await loadComments(true)
    if (question.value) question.value.comment_count += 1
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    commentSubmitting.value = false
  }
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
  loadComments(true)
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
          <el-button :type="followed ? 'default' : 'primary'" size="small" round @click="onFollowQuestion">
            {{ followed ? '已关注' : '关注问题' }} · {{ question.follower_count }}
          </el-button>
          <el-button size="small" round @click="onFollowAuthor">
            {{ authorFollowed ? '已关注作者' : '关注作者' }}
          </el-button>
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
            <button class="vote-btn" :class="{ active: voteMap[a.id] === 1 }" @click="onVote(a, 1)">
              <el-icon><CaretTop /></el-icon>
            </button>
            <span class="vote-count">{{ a.vote_count }}</span>
          </div>
          <div class="a-body">
            <div class="a-author-row">
              <router-link class="a-author" :to="`/users/${a.author_id}`">{{ a.author_name }}</router-link>
            </div>
            <div class="a-content">{{ a.content }}</div>
            <div class="a-footer">
              <span class="a-meta">{{ a.create_time?.slice(0, 10) }}</span>
              <button class="footer-btn" :class="{ active: collectMap[a.id] }" @click="onCollect(a)">
                <el-icon><Star /></el-icon>{{ collectMap[a.id] ? '已收藏' : '收藏' }} · {{ a.collect_count }}
              </button>
            </div>
          </div>
        </article>
        <div class="load-more">
          <el-button v-if="hasMore" :loading="answersLoading" plain style="width: 100%" @click="loadAnswers()">
            加载更多回答
          </el-button>
        </div>
      </template>

      <!-- 问题评论 -->
      <div class="bb-card comments-card">
        <h3 class="comments-title"><el-icon><ChatDotRound /></el-icon>{{ question.comment_count }} 条评论</h3>
        <div class="comment-input-row">
          <el-input
            v-model="commentInput"
            placeholder="写下你的评论…"
            @keyup.enter="submitComment"
          />
          <el-button type="primary" :loading="commentSubmitting" @click="submitComment">发送</el-button>
        </div>
        <div v-if="commentLoading && comments.length === 0" class="comment-loading"><LoadingState text="加载评论中…" /></div>
        <div v-else-if="comments.length === 0" class="comments-empty">还没有评论</div>
        <ul v-else class="comment-list">
          <li v-for="cm in comments" :key="cm.id" class="comment-item">
            <span class="comment-user">{{ cm.username }}</span>
            <span class="comment-content">{{ cm.content }}</span>
            <span class="comment-time">{{ cm.create_time?.slice(0, 10) }}</span>
          </li>
        </ul>
        <el-button v-if="!commentsFinished" text size="small" :loading="commentLoading" @click="loadComments()">
          加载更多评论
        </el-button>
      </div>

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
  display: flex;
  gap: 10px;
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
  gap: 4px;
  flex-shrink: 0;
  padding-top: 4px;
}

.vote-btn {
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--bb-text-tertiary);
  font-size: 20px;
  padding: 2px 8px;
  border-radius: 4px;
  transition: all 0.15s;

  &:hover {
    background: #e8f3ff;
    color: var(--bb-primary);
  }

  &.active {
    color: var(--bb-primary);
    background: #e8f3ff;
  }
}

.vote-count {
  font-weight: 600;
  color: var(--bb-primary);
}

.a-author-row {
  display: flex;
  align-items: center;
}

.a-author {
  font-weight: 600;
  font-size: 14px;

  &:hover {
    color: var(--bb-primary);
  }
}

.a-content {
  white-space: pre-wrap;
  color: var(--bb-text-main);
  margin: 8px 0;
}

.a-footer {
  display: flex;
  align-items: center;
  gap: 16px;
}

.a-meta {
  color: var(--bb-text-tertiary);
  font-size: 12px;
}

.footer-btn {
  border: none;
  background: transparent;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  color: var(--bb-text-tertiary);
  padding: 4px 8px;
  border-radius: 4px;

  &:hover {
    color: var(--bb-primary);
    background: var(--bb-bg-page);
  }

  &.active {
    color: #ff9500;
  }
}

.comments-card {
  padding: 20px 24px;
  margin-bottom: 12px;
}

.comments-title {
  margin: 0 0 12px;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 16px;
}

.comment-input-row {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
}

.comments-empty {
  color: var(--bb-text-tertiary);
  font-size: 13px;
  padding: 8px 0 16px;
}

.comment-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.comment-item {
  padding: 10px 0;
  border-bottom: 1px solid var(--bb-border);
  font-size: 14px;
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.comment-user {
  font-weight: 600;
  color: var(--bb-text-secondary);
  flex-shrink: 0;
}

.comment-content {
  color: var(--bb-text-main);
  flex: 1;
}

.comment-time {
  color: var(--bb-text-tertiary);
  font-size: 12px;
  flex-shrink: 0;
}

.comment-loading {
  padding: 8px 0;
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

@media (max-width: 768px) {
  .q-card,
  .answer-editor,
  .comments-card {
    padding: 16px;
  }
}
</style>
