<script setup lang="ts">
// 问题详情页：问题正文 + 关注 + 回答列表（投票/收藏/楼中楼评论）+ 回答编辑器
import { ref, onMounted, computed, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { View as ViewIcon, CaretTop, Star } from '@element-plus/icons-vue'
import { fetchQuestionDetail, fetchAnswers, createAnswer, updateQuestion, deleteQuestion, updateAnswer, deleteAnswer, type QuestionDetail, type AnswerDetail } from '@/api/question'
import { voteAnswer, collectAnswer, uncollectAnswer, followQuestion, unfollowQuestion, fetchQuestionInteractions, fetchAnswerInteractions } from '@/api/interaction'
import { followUser, unfollowUser } from '@/api/interaction'
import { addBookmarkItem, listFolders, createFolder, type BookmarkFolder } from '@/api/productivity'
import { createReport } from '@/api/productivity'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'
import CommentSection from '@/components/CommentSection.vue'

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

// 评论：问题与每条回答各有一个评论区（CommentSection），这里只持有计数与「展开哪条回答的评论」
const openCommentsFor = ref('')

function onQuestionCommentsChange(delta: number) {
  if (question.value) question.value.comment_count = Math.max(0, question.value.comment_count + delta)
}

function toggleAnswerComments(a: AnswerDetail) {
  openCommentsFor.value = openCommentsFor.value === a.id ? '' : a.id
}

// 回答编辑器状态
const answerContent = ref('')
const submitting = ref(false)

// 编辑/删除权限：仅作者本人
const isQuestionAuthor = computed(() => !!auth.user && question.value?.author_id === auth.user.user_id)
const editDialogVisible = ref(false)
const editTitle = ref('')
const editDescription = ref('')
const editSubmitting = ref(false)
const editAnswerId = ref('')
const editAnswerContent = ref('')
const editAnswerSubmitting = ref(false)

function openEditQuestion() {
  if (!question.value) return
  editTitle.value = question.value.title
  editDescription.value = question.value.description
  editDialogVisible.value = true
}

async function submitEditQuestion() {
  if (!editTitle.value.trim()) return ElMessage.warning('标题不能为空')
  editSubmitting.value = true
  try {
    await updateQuestion(questionId, { title: editTitle.value.trim(), description: editDescription.value.trim() })
    editDialogVisible.value = false
    ElMessage.success('问题已更新')
    await load()
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    editSubmitting.value = false
  }
}

async function onDeleteQuestion() {
  try {
    await ElMessageBox.confirm('删除后问题将不再展示，确定删除？', '删除问题', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteQuestion(questionId)
    ElMessage.success('问题已删除')
    router.push('/')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

function openEditAnswer(a: AnswerDetail) {
  editAnswerId.value = a.id
  editAnswerContent.value = a.content
  editAnswerDialogVisible.value = true
}
const editAnswerDialogVisible = ref(false)

async function submitEditAnswer() {
  if (!editAnswerContent.value.trim()) return ElMessage.warning('内容不能为空')
  editAnswerSubmitting.value = true
  try {
    await updateAnswer(editAnswerId.value, editAnswerContent.value.trim())
    editAnswerDialogVisible.value = false
    ElMessage.success('回答已更新')
    await loadAnswers(true)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    editAnswerSubmitting.value = false
  }
}

async function onDeleteAnswer(a: AnswerDetail) {
  try {
    await ElMessageBox.confirm('删除后回答将不再展示，确定删除？', '删除回答', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteAnswer(a.id)
    ElMessage.success('回答已删除')
    await load()
    await loadAnswers(true)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    question.value = await fetchQuestionDetail(questionId)
    // 治理重定向（PRD F8）：A 详情自动跳转到 B
    if (question.value.redirect_to) {
      router.replace(`/questions/${question.value.redirect_to}?merged=${questionId}`)
      return
    }
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

// 举报对话框
const reportVisible = ref(false)
const reportReason = ref<'spam' | 'abuse' | 'copyright' | 'unsafe' | 'other'>('spam')
const reportDesc = ref('')
const reportSubmitting = ref(false)

function openReport() {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  reportVisible.value = true
}

async function submitReport() {
  reportSubmitting.value = true
  try {
    await createReport({
      target_type: 'question',
      target_id: questionId,
      reason_code: reportReason.value,
      description: reportDesc.value.trim(),
    })
    ElMessage.success('举报已提交，我们会尽快处理')
    reportVisible.value = false
    reportDesc.value = ''
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    reportSubmitting.value = false
  }
}

// 收藏到收藏夹（默认"我的收藏"）
const bookmarkLoading = ref(false)
async function onBookmark() {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  bookmarkLoading.value = true
  try {
    const folders = (await listFolders()) || []
    let folder: BookmarkFolder | undefined = folders.find((f) => f.name === '我的收藏')
    if (!folder) {
      const created = await createFolder('我的收藏')
      folder = { folder_id: created.folder_id, name: '我的收藏', is_private: true }
    }
    await addBookmarkItem(folder.folder_id, 'question', questionId)
    ElMessage.success('已收藏')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    bookmarkLoading.value = false
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

onMounted(async () => {
  load()
  await loadAnswers(true)
  await scrollToHashAnswer()
})

// 从通知等入口带 #answer-<id> 进入时：定位并短暂高亮对应回答
async function scrollToHashAnswer() {
  const hash = route.hash
  if (!hash) return
  await nextTick()
  const el = document.getElementById(hash.slice(1))
  if (!el) return
  el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  el.classList.add('highlight')
  setTimeout(() => el.classList.remove('highlight'), 2000)
}
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
          <template v-if="isQuestionAuthor">
            <el-button size="small" plain @click="openEditQuestion">编辑</el-button>
            <el-button size="small" plain type="danger" @click="onDeleteQuestion">删除</el-button>
          </template>
          <el-button size="small" plain :loading="bookmarkLoading" @click="onBookmark">收藏</el-button>
          <el-button size="small" plain type="warning" @click="openReport">举报</el-button>
        </div>
        <el-dialog v-model="editDialogVisible" title="编辑问题" width="90%" style="max-width: 640px">
          <el-form label-position="top">
            <el-form-item label="标题">
              <el-input v-model="editTitle" maxlength="200" show-word-limit />
            </el-form-item>
            <el-form-item label="补充说明">
              <el-input v-model="editDescription" type="textarea" :rows="6" maxlength="10000" />
            </el-form-item>
          </el-form>
          <template #footer>
            <el-button @click="editDialogVisible = false">取消</el-button>
            <el-button type="primary" :loading="editSubmitting" @click="submitEditQuestion">保存</el-button>
          </template>
        </el-dialog>
      </article>

      <!-- 举报对话框 -->
      <el-dialog v-model="reportVisible" title="举报问题" width="90%" style="max-width: 480px">
        <el-form label-position="top">
          <el-form-item label="举报原因">
            <el-select v-model="reportReason" style="width: 100%">
              <el-option label="垃圾营销" value="spam" />
              <el-option label="辱骂攻击" value="abuse" />
              <el-option label="侵权" value="copyright" />
              <el-option label="不安全内容" value="unsafe" />
              <el-option label="其他" value="other" />
            </el-select>
          </el-form-item>
          <el-form-item label="补充说明（可选）">
            <el-input v-model="reportDesc" type="textarea" :rows="3" maxlength="1000" />
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="reportVisible = false">取消</el-button>
          <el-button type="primary" :loading="reportSubmitting" @click="submitReport">提交举报</el-button>
        </template>
      </el-dialog>

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
        <article v-for="a in answers" :id="`answer-${a.id}`" :key="a.id" class="bb-card answer-card">
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
              <button class="footer-btn" :class="{ active: openCommentsFor === a.id }" @click="toggleAnswerComments(a)">
                {{ openCommentsFor === a.id ? '收起评论' : '评论' }} · {{ a.comment_count }}
              </button>
              <template v-if="auth.user?.user_id === a.author_id">
                <button class="footer-btn" @click="openEditAnswer(a)">编辑</button>
                <button class="footer-btn danger" @click="onDeleteAnswer(a)">删除</button>
              </template>
            </div>
            <!-- 回答下的楼中楼评论区：默认折叠，点击「评论」时才挂载 -->
            <CommentSection
              v-if="openCommentsFor === a.id"
              class="a-comments"
              target-type="answer"
              :target-id="a.id"
              :total="a.comment_count"
              compact
              @change="a.comment_count = Math.max(0, a.comment_count + $event)"
            />
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
        <CommentSection
          target-type="question"
          :target-id="questionId"
          :total="question.comment_count"
          @change="onQuestionCommentsChange"
        />
      </div>

        <el-dialog v-model="editAnswerDialogVisible" title="编辑回答" width="90%" style="max-width: 640px">
          <el-input v-model="editAnswerContent" type="textarea" :rows="8" maxlength="100000" />
          <template #footer>
            <el-button @click="editAnswerDialogVisible = false">取消</el-button>
            <el-button type="primary" :loading="editAnswerSubmitting" @click="submitEditAnswer">保存</el-button>
          </template>
        </el-dialog>

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

.footer-btn.danger:hover {
  color: #f56c6c;
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

// 通知跳转定位到的回答：短暂高亮
.answer-card.highlight {
  box-shadow: 0 0 0 2px var(--bb-primary);
  transition: box-shadow 0.3s;
}

.a-comments {
  margin-top: 8px;
  padding-top: 4px;
  border-top: 1px dashed var(--bb-border);
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
