<script setup lang="ts">
// 评论组件：问题与回答共用。一级评论列表（最新在前）+ 楼中楼回复（最早在前）+ 点赞 / 删除 / 举报
// total 由父组件持有（回答 / 问题的 comment_count），本组件只通过 change 事件通知增减
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  fetchComments,
  fetchCommentReplies,
  createComment,
  likeComment,
  unlikeComment,
  deleteComment,
  editComment,
  type CommentDetail } from '@/api/interaction'
import { createReport } from '@/api/productivity'
import { ApiError } from '@/api/http'
import { useAuthStore } from '@/stores/auth'
import LoadingState from './LoadingState.vue'
import CommentItem from './CommentItem.vue'

const props = withDefaults(
  defineProps<{
    targetType: 'answer' | 'question'
    targetId: string
    total: number
    // 回答下的评论：默认折叠，由父组件控制挂载
    compact?: boolean
  }>(),
  { compact: false },
)
const emit = defineEmits<{ (e: 'change', delta: number): void }>()

const auth = useAuthStore()
const router = useRouter()

// 一级评论列表
const items = ref<CommentDetail[]>([])
const loading = ref(false)
const finished = ref(true)
const cursor = ref('')
const input = ref('')
const submitting = ref(false)

// 楼中楼：按一级评论 ID 分组
interface Thread {
  open: boolean
  loaded: boolean
  loading: boolean
  hasMore: boolean
  cursor: string
  replies: CommentDetail[]
}
const threads = reactive<Record<string, Thread>>({})
const newThread = (): Thread => ({ open: false, loaded: false, loading: false, hasMore: false, cursor: '', replies: [] })
const threadOf = (rootId: string): Thread => (threads[rootId] ??= newThread())

// 回复框：同一时间只对一条评论展开
const replyTo = ref<CommentDetail | null>(null)
const replyInput = ref('')
const replySubmitting = ref(false)
// parent_id 为 '0'（一级评论）时根就是自身，否则是其 parent_id
const rootIdOf = (cm: CommentDetail) => (cm.parent_id === '0' ? cm.id : cm.parent_id)

// 举报
const reportTarget = ref<CommentDetail | null>(null)
const reportVisible = ref(false)
const reportReason = ref<'spam' | 'abuse' | 'copyright' | 'unsafe' | 'other'>('spam')
const reportDesc = ref('')
const reportSubmitting = ref(false)

function showError(e: unknown) {
  if (e instanceof ApiError) ElMessage.error(e.message)
}

// 未登录时跳登录页并在登录后回到当前页
function requireLogin(): boolean {
  if (auth.isLoggedIn) return true
  router.push({ name: 'login', query: { redirect: router.currentRoute.value.fullPath } })
  return false
}

async function load(reset = false) {
  if (loading.value) return
  if (!reset && finished.value) return
  loading.value = true
  try {
    if (reset) {
      cursor.value = ''
      finished.value = false
    }
    const res = await fetchComments(props.targetType, props.targetId, cursor.value)
    items.value = reset ? res.items : [...items.value, ...res.items]
    cursor.value = res.next_cursor
    finished.value = !res.has_more
  } catch (e) {
    showError(e)
  } finally {
    loading.value = false
  }
}

async function submit() {
  const content = input.value.trim()
  if (!content) return ElMessage.warning('请输入评论内容')
  if (!requireLogin()) return
  submitting.value = true
  try {
    await createComment(props.targetType, props.targetId, content)
    input.value = ''
    ElMessage.success('评论已发布')
    await load(true)
    emit('change', 1)
  } catch (e) {
    showError(e)
  } finally {
    submitting.value = false
  }
}

async function loadReplies(rootId: string, reset = false) {
  const th = threadOf(rootId)
  if (th.loading) return
  if (!reset && th.loaded && !th.hasMore) return
  th.loading = true
  try {
    if (reset) {
      th.cursor = ''
      th.hasMore = false
    }
    const res = await fetchCommentReplies(rootId, th.cursor)
    th.replies = reset ? res.items : [...th.replies, ...res.items]
    th.cursor = res.next_cursor
    th.hasMore = res.has_more
    th.loaded = true
  } catch (e) {
    showError(e)
  } finally {
    th.loading = false
  }
}

function toggleReplies(root: CommentDetail) {
  const th = threadOf(root.id)
  th.open = !th.open
  if (th.open && !th.loaded) loadReplies(root.id)
}

function startReply(cm: CommentDetail) {
  if (!requireLogin()) return
  replyTo.value = cm
  replyInput.value = ''
}

function cancelReply() {
  replyTo.value = null
  replyInput.value = ''
}

async function submitReply() {
  const target = replyTo.value
  if (!target) return
  const content = replyInput.value.trim()
  if (!content) return ElMessage.warning('请输入回复内容')
  replySubmitting.value = true
  try {
    // parent_id 传被回复的那条评论；后端负责挂到所属一级评论下并记录「回复 @谁」
    await createComment(props.targetType, props.targetId, content, target.id)
    replyInput.value = ''
    replyTo.value = null
    ElMessage.success('回复已发布')
    const rootId = rootIdOf(target)
    threadOf(rootId).open = true
    await loadReplies(rootId, true)
    const root = items.value.find((i) => i.id === rootId)
    if (root) root.reply_count += 1
    emit('change', 1)
  } catch (e) {
    showError(e)
  } finally {
    replySubmitting.value = false
  }
}

async function toggleLike(cm: CommentDetail) {
  if (!requireLogin()) return
  const wasLiked = cm.liked
  // 乐观更新，失败回滚
  cm.liked = !wasLiked
  cm.like_count += wasLiked ? -1 : 1
  try {
    if (wasLiked) await unlikeComment(cm.id)
    else await likeComment(cm.id)
  } catch (e) {
    cm.liked = wasLiked
    cm.like_count += wasLiked ? 1 : -1
    showError(e)
  }
}

// 编辑评论（PRD F4）：保存后本地更新内容与已编辑标记；窗口外由后端拒绝
async function saveEdit(cm: CommentDetail, content: string) {
  try {
    await editComment(cm.id, content)
    cm.content = content
    cm.edited = true
    cm.can_edit = false // 编辑后窗口按创建时间计算，保守关闭入口
    ElMessage.success('评论已更新')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

async function remove(cm: CommentDetail) {
  const isTopLevel = cm.parent_id === '0'
  const tip = isTopLevel ? '删除后这条评论将不再展示，其下已有的回复会保留。' : '删除后这条回复将不再展示。'
  try {
    await ElMessageBox.confirm(`${tip}确定删除？`, '删除评论', { type: 'warning' })
  } catch {
    return
  }
  try {
    await deleteComment(cm.id)
    ElMessage.success('评论已删除')
    if (replyTo.value && replyTo.value.id === cm.id) cancelReply()
    if (isTopLevel) {
      // 一级评论：重新拉取（已删除但仍有回复的会以占位形式返回）
      delete threads[cm.id]
      await load(true)
    } else {
      const th = threads[cm.parent_id]
      if (th) th.replies = th.replies.filter((r) => r.id !== cm.id)
      const root = items.value.find((i) => i.id === cm.parent_id)
      if (root) root.reply_count = Math.max(0, root.reply_count - 1)
    }
    emit('change', -1)
  } catch (e) {
    showError(e)
  }
}

function openReport(cm: CommentDetail) {
  if (!requireLogin()) return
  reportTarget.value = cm
  reportReason.value = 'spam'
  reportDesc.value = ''
  reportVisible.value = true
}

async function submitReport() {
  if (!reportTarget.value) return
  reportSubmitting.value = true
  try {
    await createReport({
      target_type: 'comment',
      target_id: reportTarget.value.id,
      reason_code: reportReason.value,
      description: reportDesc.value.trim(),
    })
    ElMessage.success('举报已提交，我们会尽快处理')
    reportVisible.value = false
  } catch (e) {
    showError(e)
  } finally {
    reportSubmitting.value = false
  }
}

onMounted(() => {
  load(true)
})

defineExpose({ reload: () => load(true) })
</script>

<template>
  <div class="cs" :class="{ 'cs-compact': compact }">
    <div class="cs-head">{{ total }} 条评论</div>

    <div class="cs-input-row">
      <el-input v-model="input" placeholder="写下你的评论…" maxlength="2000" @keyup.enter="submit" />
      <el-button type="primary" :loading="submitting" @click="submit">发送</el-button>
    </div>

    <div v-if="loading && items.length === 0" class="cs-hint"><LoadingState text="加载评论中…" /></div>
    <div v-else-if="items.length === 0" class="cs-hint">还没有评论，来说两句吧</div>

    <div v-for="cm in items" :key="cm.id" class="cs-thread">
      <CommentItem
        :comment="cm"
        @like="toggleLike(cm)"
        @reply="startReply(cm)"
        @delete="remove(cm)"
        @report="openReport(cm)"
        @edit="(content) => saveEdit(cm, content)"
      />

      <!-- 楼中楼 -->
      <div v-if="cm.reply_count > 0 || threads[cm.id]?.open" class="cs-replies-wrap">
        <button class="cs-toggle" type="button" @click="toggleReplies(cm)">
          {{ threads[cm.id]?.open ? '收起回复' : `查看 ${cm.reply_count} 条回复` }}
        </button>
        <div v-if="threads[cm.id]?.open" class="cs-replies">
          <CommentItem
            v-for="r in threads[cm.id]?.replies"
            :key="r.id"
            class="cs-reply"
            :comment="r"
            @like="toggleLike(r)"
            @reply="startReply(r)"
            @delete="remove(r)"
            @report="openReport(r)"
            @edit="(content) => saveEdit(r, content)"
          />
          <div v-if="threads[cm.id]?.loading" class="cs-hint"><LoadingState text="加载回复中…" /></div>
          <div v-else-if="threads[cm.id]?.loaded && threads[cm.id]?.replies.length === 0" class="cs-hint">回复已全部删除</div>
          <button v-if="threads[cm.id]?.hasMore" class="cs-toggle" type="button" @click="loadReplies(cm.id)">加载更多回复</button>
        </div>
      </div>

      <!-- 回复输入框：挂在被回复评论所属的一级评论下方 -->
      <div v-if="replyTo && rootIdOf(replyTo) === cm.id" class="cs-reply-row">
        <span class="cs-reply-hint">回复 @{{ replyTo.username }}</span>
        <el-input v-model="replyInput" size="small" maxlength="2000" placeholder="写下你的回复…" @keyup.enter="submitReply" />
        <el-button type="primary" size="small" :loading="replySubmitting" @click="submitReply">发送</el-button>
        <el-button size="small" text @click="cancelReply">取消</el-button>
      </div>
    </div>

    <el-button v-if="!finished" text size="small" :loading="loading" @click="load()">加载更多评论</el-button>

    <el-dialog v-model="reportVisible" title="举报评论" width="90%" style="max-width: 480px">
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
  </div>
</template>

<style scoped lang="less">
.cs-head {
  font-size: 16px;
  font-weight: 600;
  margin: 0 0 12px;
}

.cs-compact .cs-head {
  font-size: 14px;
}

.cs-input-row {
  display: flex;
  gap: 10px;
  margin-bottom: 8px;
}

.cs-hint {
  color: var(--bb-text-tertiary);
  font-size: 13px;
  padding: 8px 0;
}

.cs-thread {
  border-bottom: 1px solid var(--bb-border);

  &:last-of-type {
    border-bottom: none;
  }
}

.cs-replies-wrap {
  padding: 0 0 10px 16px;
  background: var(--bb-bg-page);
  border-radius: 6px;
}

.cs-toggle {
  border: none;
  background: transparent;
  cursor: pointer;
  display: block;
  width: 100%;
  text-align: left;
  padding: 6px 12px;
  font-size: 13px;
  color: var(--bb-primary);

  &:hover {
    text-decoration: underline;
  }
}

.cs-replies {
  padding: 0 12px;

  .cs-hint {
    padding: 4px 0;
  }
}

.cs-reply-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 0 12px 16px;
}

.cs-reply-hint {
  font-size: 13px;
  color: var(--bb-primary);
  white-space: nowrap;
}
</style>
