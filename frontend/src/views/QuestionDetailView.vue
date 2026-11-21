<script setup lang="ts">
// 问题详情页：问题正文 + 关注 + 回答列表（投票/收藏/评论）+ 回答编辑器
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { View as ViewIcon, CaretTop, Star, ChatDotRound } from '@element-plus/icons-vue'
import { fetchQuestionDetail, fetchAnswers, createAnswer, updateQuestion, deleteQuestion, updateAnswer, deleteAnswer, acceptAnswer, unacceptAnswer, setQuestionStatus, type QuestionDetail, type AnswerDetail } from '@/api/question'
import { voteAnswer, collectAnswer, uncollectAnswer, followQuestion, unfollowQuestion, fetchQuestionInteractions, fetchAnswerInteractions, fetchComments, createComment, type CommentDetail } from '@/api/interaction'
import { followUser, unfollowUser } from '@/api/interaction'
import { addBookmarkItem, listFolders, createFolder, type BookmarkFolder } from '@/api/productivity'
import { createReport } from '@/api/productivity'
import { fetchInviteCandidates, sendAnswerInvitations, fetchQuestionInvitations, fetchMyAnswerInvitations, acceptAnswerInvitation, declineAnswerInvitation, invitationStatusLabels, type CandidateUser, type AnswerInvitation } from '@/api/invitation'
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

// ---------- 解决闭环（采纳/撤销/关闭/重开） ----------

// resolution_status: 0=open 1=solved 2=closed
const isSolved = computed(() => question.value?.resolution_status === 1)
const isClosed = computed(() => question.value?.resolution_status === 2)
const acceptedAnswerId = computed(() => question.value?.accepted_answer_id || '')

const acceptingId = ref('')

// 采纳回答（仅提问者）
async function onAccept(a: AnswerDetail) {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  try {
    await ElMessageBox.confirm('采纳后问题将标记为已解决，确定采纳该回答？', '采纳回答', { type: 'success' })
  } catch {
    return
  }
  acceptingId.value = a.id
  try {
    await acceptAnswer(questionId, a.id)
    ElMessage.success('已采纳该回答')
    await load()
    await loadAnswers(true)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    acceptingId.value = ''
  }
}

// 撤销采纳（仅提问者）
async function onUnaccept(a: AnswerDetail) {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  try {
    await ElMessageBox.confirm('撤销后问题将恢复为待解决，确定撤销？', '撤销采纳', { type: 'warning' })
  } catch {
    return
  }
  acceptingId.value = a.id
  try {
    await unacceptAnswer(questionId, a.id)
    ElMessage.success('已撤销采纳')
    await load()
    await loadAnswers(true)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    acceptingId.value = ''
  }
}

// 关闭/重开问题（仅提问者）
const statusSubmitting = ref(false)
async function onToggleClosed() {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  const closing = !isClosed.value
  try {
    await ElMessageBox.confirm(
      closing ? '关闭后将不再接受新回答，确定关闭？' : '重开后其他用户可以继续回答，确定重开？',
      closing ? '关闭问题' : '重开问题',
      { type: 'warning' },
    )
  } catch {
    return
  }
  statusSubmitting.value = true
  try {
    await setQuestionStatus(questionId, closing ? 'closed' : 'open')
    ElMessage.success(closing ? '问题已关闭' : '问题已重开')
    await load()
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    statusSubmitting.value = false
  }
}

// ---------- 邀请回答（PRD §6.2） ----------

const inviteDialogVisible = ref(false)
const candidates = ref<CandidateUser[]>([])
const candidatesLoading = ref(false)
const selectedInvitees = ref<string[]>([])
const inviteKeyword = ref('')
const inviteSubmitting = ref(false)
const inviteResults = ref<{ invitee_id: string; ok: boolean; reason?: string; username?: string }[]>([])
const questionInvitations = ref<AnswerInvitation[]>([])

const canInvite = computed(() =>
  isQuestionAuthor.value && !isClosed.value && !isSolved.value && (question.value?.answer_count ?? 0) === 0,
)

async function openInviteDialog() {
  inviteDialogVisible.value = true
  inviteResults.value = []
  selectedInvitees.value = []
  await loadCandidates()
  loadQuestionInvitations()
}

async function loadCandidates() {
  candidatesLoading.value = true
  try {
    candidates.value = (await fetchInviteCandidates(questionId, inviteKeyword.value.trim())) || []
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    candidatesLoading.value = false
  }
}

function toggleInvitee(userId: string) {
  const idx = selectedInvitees.value.indexOf(userId)
  if (idx >= 0) {
    selectedInvitees.value.splice(idx, 1)
  } else if (selectedInvitees.value.length < 5) {
    selectedInvitees.value.push(userId)
  } else {
    ElMessage.warning('一次最多邀请 5 人')
  }
}

async function submitInvitations() {
  if (selectedInvitees.value.length === 0) return ElMessage.warning('请先选择要邀请的用户')
  inviteSubmitting.value = true
  try {
    const results = (await sendAnswerInvitations(questionId, selectedInvitees.value)) || []
    const nameMap = new Map(candidates.value.map((c) => [c.user_id, c.username]))
    inviteResults.value = results.map((r) => ({
      ...r,
      username: nameMap.get(r.invitee_id) || r.invitee_id,
    }))
    selectedInvitees.value = []
    const okCount = results.filter((r) => r.ok).length
    if (okCount > 0) ElMessage.success(`已向 ${okCount} 位用户发送邀请`)
    loadQuestionInvitations()
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    inviteSubmitting.value = false
  }
}

async function loadQuestionInvitations() {
  try {
    questionInvitations.value = (await fetchQuestionInvitations(questionId)) || []
  } catch {
    // 非作者或加载失败忽略
  }
}

// 被邀请横幅：当前登录用户在该问题上有待处理邀请
const myPendingInvitation = ref<AnswerInvitation | null>(null)
const inviteResponding = ref(false)

async function loadMyPendingInvitation() {
  if (!auth.isLoggedIn) return
  try {
    const res = (await fetchMyAnswerInvitations({ status: 'pending', limit: 50 })) || []
    myPendingInvitation.value = res.items.find((it) => it.question_id === questionId) || null
  } catch {
    // 忽略
  }
}

async function respondMyInvitation(decision: 'accept' | 'decline') {
  if (!myPendingInvitation.value) return
  inviteResponding.value = true
  try {
    if (decision === 'accept') {
      await acceptAnswerInvitation(myPendingInvitation.value.id)
      ElMessage.success('已接受邀请，欢迎写下你的回答')
    } else {
      await declineAnswerInvitation(myPendingInvitation.value.id)
      ElMessage.success('已婉拒邀请')
    }
    myPendingInvitation.value = null
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    inviteResponding.value = false
  }
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
  loadMyPendingInvitation()
})
</script>

<template>
  <div class="question-detail">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card">
      <ErrorState :text="error" @retry="load" />
    </div>
    <template v-else-if="question">
      <!-- 被邀请横幅（PRD §6.3） -->
      <div v-if="myPendingInvitation" class="bb-card invite-banner">
        <span>
          <strong>{{ myPendingInvitation.inviter_name }}</strong> 邀请你回答这个问题
        </span>
        <span class="invite-banner-actions">
          <el-button type="primary" size="small" :loading="inviteResponding" @click="respondMyInvitation('accept')">
            接受邀请
          </el-button>
          <el-button size="small" :disabled="inviteResponding" @click="respondMyInvitation('decline')">婉拒</el-button>
        </span>
      </div>

      <!-- 问题正文 -->
      <article class="bb-card q-card">
        <h1 class="q-title">{{ question.title }}</h1>
        <div class="q-meta">
          <el-tag v-if="isSolved" type="success" effect="light" size="small" class="q-status-tag">已解决</el-tag>
          <el-tag v-else-if="isClosed" type="info" effect="light" size="small" class="q-status-tag">已关闭</el-tag>
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
            <el-button
              v-if="!isSolved"
              size="small"
              plain
              :loading="statusSubmitting"
              @click="onToggleClosed"
            >
              {{ isClosed ? '重开问题' : '关闭问题' }}
            </el-button>
            <el-button size="small" plain type="danger" @click="onDeleteQuestion">删除</el-button>
          </template>
          <el-button v-if="canInvite" size="small" plain type="success" @click="openInviteDialog">邀请回答</el-button>
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

      <!-- 邀请回答对话框（PRD §6.2） -->
      <el-dialog v-model="inviteDialogVisible" title="邀请回答" width="90%" style="max-width: 560px">
        <p class="invite-hint">将通知所选用户（一次最多 5 人）；候选按同话题贡献排序</p>
        <el-input
          v-model="inviteKeyword"
          placeholder="按用户名搜索候选"
          clearable
          @keyup.enter="loadCandidates"
        />
        <div v-if="candidatesLoading" class="invite-loading"><LoadingState /></div>
        <ul v-else class="candidate-list">
          <li v-for="c in candidates" :key="c.user_id" class="candidate-item">
            <label class="candidate-label">
              <input
                type="checkbox"
                :checked="selectedInvitees.includes(c.user_id)"
                @change="toggleInvitee(c.user_id)"
              />
              <span class="candidate-name">{{ c.username }}</span>
              <el-tag size="small" effect="plain">{{ c.reason }}</el-tag>
            </label>
          </li>
          <li v-if="candidates.length === 0" class="candidate-empty">暂无可邀请的候选用户</li>
        </ul>
        <div v-if="inviteResults.length" class="invite-results">
          <div
            v-for="r in inviteResults"
            :key="r.invitee_id"
            class="invite-result-item"
            :class="{ failed: !r.ok }"
          >
            {{ r.username }}：{{ r.ok ? '邀请已送达' : r.reason }}
          </div>
        </div>
        <template #footer>
          <el-button @click="inviteDialogVisible = false">关闭</el-button>
          <el-button
            type="primary"
            :loading="inviteSubmitting"
            :disabled="selectedInvitees.length === 0"
            @click="submitInvitations"
          >
            发送邀请（{{ selectedInvitees.length }}/5）
          </el-button>
        </template>
      </el-dialog>

      <!-- 邀请记录（仅作者可见） -->
      <div v-if="isQuestionAuthor && questionInvitations.length" class="bb-card invite-record-card">
        <h3 class="invite-record-title">邀请记录</h3>
        <ul class="invite-record-list">
          <li v-for="inv in questionInvitations" :key="inv.id" class="invite-record-item">
            <span class="invite-record-name">{{ inv.invitee_name }}</span>
            <el-tag size="small" effect="light">{{ invitationStatusLabels[inv.status] || inv.status }}</el-tag>
            <span class="invite-record-time">{{ inv.create_time?.slice(0, 10) }}</span>
          </li>
        </ul>
      </div>

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
        <article v-for="a in answers" :key="a.id" class="bb-card answer-card" :class="{ 'answer-accepted': a.accepted }">
          <div class="a-vote">
            <button class="vote-btn" :class="{ active: voteMap[a.id] === 1 }" @click="onVote(a, 1)">
              <el-icon><CaretTop /></el-icon>
            </button>
            <span class="vote-count">{{ a.vote_count }}</span>
          </div>
          <div class="a-body">
            <div class="a-author-row">
              <el-tag v-if="a.accepted" type="success" size="small" effect="light" class="accepted-tag">✓ 已采纳</el-tag>
              <router-link class="a-author" :to="`/users/${a.author_id}`">{{ a.author_name }}</router-link>
            </div>
            <div class="a-content">{{ a.content }}</div>
            <div class="a-footer">
              <span class="a-meta">{{ a.create_time?.slice(0, 10) }}</span>
              <button class="footer-btn" :class="{ active: collectMap[a.id] }" @click="onCollect(a)">
                <el-icon><Star /></el-icon>{{ collectMap[a.id] ? '已收藏' : '收藏' }} · {{ a.collect_count }}
              </button>
              <!-- 仅提问者可采纳/撤销；closed 状态不显示采纳 -->
              <template v-if="isQuestionAuthor && !isClosed">
                <button
                  v-if="a.accepted"
                  class="footer-btn accepted-btn"
                  :disabled="acceptingId === a.id"
                  @click="onUnaccept(a)"
                >
                  取消采纳
                </button>
                <button
                  v-else
                  class="footer-btn"
                  :disabled="acceptingId === a.id || !!acceptedAnswerId"
                  @click="onAccept(a)"
                >
                  采纳
                </button>
              </template>
              <template v-if="auth.user?.user_id === a.author_id">
                <button class="footer-btn" @click="openEditAnswer(a)">编辑</button>
                <button class="footer-btn danger" @click="onDeleteAnswer(a)">删除</button>
              </template>
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

        <el-dialog v-model="editAnswerDialogVisible" title="编辑回答" width="90%" style="max-width: 640px">
          <el-input v-model="editAnswerContent" type="textarea" :rows="8" maxlength="100000" />
          <template #footer>
            <el-button @click="editAnswerDialogVisible = false">取消</el-button>
            <el-button type="primary" :loading="editAnswerSubmitting" @click="submitEditAnswer">保存</el-button>
          </template>
        </el-dialog>

      <!-- 回答编辑器（已关闭的问题不接受新回答） -->
      <div v-if="isClosed" class="bb-card answer-editor answer-editor-closed">
        该问题已关闭，不再接受新回答。
      </div>
      <div v-else class="bb-card answer-editor">
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

.q-status-tag {
  flex-shrink: 0;
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

  &.answer-accepted {
    border-left: 3px solid var(--bb-primary, #409eff);
    background: color-mix(in srgb, var(--bb-primary, #409eff) 4%, transparent);
  }
}

.accepted-tag {
  margin-right: 8px;
  flex-shrink: 0;
}

.accepted-btn {
  color: var(--bb-primary, #409eff);
}

.invite-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 14px 20px;
  margin-bottom: 12px;
  border-left: 3px solid var(--bb-primary);
}

.invite-banner-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.invite-hint {
  color: var(--bb-text-tertiary);
  font-size: 13px;
  margin: 0 0 10px;
}

.invite-loading {
  padding: 16px 0;
}

.candidate-list {
  list-style: none;
  margin: 12px 0;
  padding: 0;
  max-height: 280px;
  overflow-y: auto;
}

.candidate-item {
  padding: 8px 0;
  border-bottom: 1px solid var(--bb-border);
}

.candidate-label {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
}

.candidate-name {
  flex: 1;
  color: var(--bb-text-main);
}

.candidate-empty {
  color: var(--bb-text-tertiary);
  padding: 12px 0;
}

.invite-results {
  margin-top: 10px;
}

.invite-result-item {
  font-size: 13px;
  color: var(--bb-text-secondary);
  padding: 3px 0;

  &.failed {
    color: var(--el-color-danger, #f56c6c);
  }
}

.invite-record-card {
  padding: 16px 20px;
  margin-bottom: 12px;
}

.invite-record-title {
  margin: 0 0 10px;
  font-size: 15px;
}

.invite-record-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.invite-record-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 0;
  font-size: 14px;
}

.invite-record-name {
  flex: 1;
  color: var(--bb-text-main);
}

.invite-record-time {
  color: var(--bb-text-tertiary);
  font-size: 12px;
}

.answer-editor-closed {
  color: var(--bb-text-tertiary);
  text-align: center;
  padding: 20px;
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
