<script setup lang="ts">
// 文章详情：安全 Markdown 服务端渲染内容 + 作者操作 + 评论区（复用评论接口 target=article）
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { View as ViewIcon } from '@element-plus/icons-vue'
import { fetchArticle, deleteArticle, type ArticleDetail } from '@/api/article'
import { previewMarkdown } from '@/api/productivity'
import { fetchComments, createComment, type CommentDetail } from '@/api/interaction'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const articleId = route.params.id as string
const article = ref<ArticleDetail | null>(null)
const loading = ref(false)
const error = ref('')
const renderedHtml = ref('')

const isAuthor = computed(() => !!auth.user && article.value?.author_id === auth.user.user_id)

// 评论
const comments = ref<CommentDetail[]>([])
const commentInput = ref('')
const commentLoading = ref(false)
const commentSubmitting = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    article.value = await fetchArticle(articleId)
    const res = await previewMarkdown(article.value.content)
    renderedHtml.value = res.html
    await loadComments()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function loadComments() {
  commentLoading.value = true
  try {
    const res = await fetchComments('article', articleId)
    comments.value = res.items
  } catch {
    // 评论加载失败不阻塞正文
  } finally {
    commentLoading.value = false
  }
}

async function submitComment() {
  if (!auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  if (!commentInput.value.trim()) return ElMessage.warning('请输入评论')
  commentSubmitting.value = true
  try {
    await createComment('article', articleId, commentInput.value.trim())
    commentInput.value = ''
    ElMessage.success('评论已发布')
    loadComments()
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    commentSubmitting.value = false
  }
}

async function onDelete() {
  try {
    await ElMessageBox.confirm('删除后文章将不再展示，确定删除？', '删除文章', { type: 'warning' })
  } catch { return }
  try {
    await deleteArticle(articleId)
    ElMessage.success('文章已删除')
    router.push('/articles')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

onMounted(load)
</script>

<template>
  <div class="article-detail">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card"><ErrorState :text="error" @retry="load" /></div>
    <template v-else-if="article">
      <article class="bb-card a-card">
        <h1 class="a-title">{{ article.title }}</h1>
        <div class="a-meta">
          <router-link class="a-author" :to="`/users/${article.author_id}`">{{ article.author_name }}</router-link>
          <el-tag v-if="article.column" size="small" effect="plain">
            <router-link :to="`/columns/${article.column.column_id}`">{{ article.column.name }}</router-link>
          </el-tag>
          <span>{{ article.create_time?.slice(0, 10) }}</span>
          <span class="a-views"><el-icon><ViewIcon /></el-icon>{{ article.view_count }} 次阅读</span>
        </div>
        <div v-if="isAuthor" class="a-actions">
          <el-button size="small" plain @click="router.push(`/articles/${articleId}/edit`)">编辑</el-button>
          <el-button size="small" plain type="danger" @click="onDelete">删除</el-button>
        </div>
        <!-- 服务端安全渲染的 HTML（安全子集，无原始 HTML/脚本） -->
        <div class="a-content" v-html="renderedHtml"></div>
      </article>

      <div class="bb-card comments-card">
        <h3>评论</h3>
        <div class="comment-input-row">
          <el-input v-model="commentInput" placeholder="写下你的评论…" @keyup.enter="submitComment" />
          <el-button type="primary" :loading="commentSubmitting" @click="submitComment">发送</el-button>
        </div>
        <div v-if="comments.length === 0" class="comments-empty">还没有评论</div>
        <ul v-else class="comment-list">
          <li v-for="cm in comments" :key="cm.id" class="comment-item">
            <span class="comment-user">{{ cm.username }}</span>
            <span class="comment-content">{{ cm.content }}</span>
            <span class="comment-time">{{ cm.create_time?.slice(0, 10) }}</span>
          </li>
        </ul>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.a-card { padding: 24px 28px; }
.a-title { margin: 0; font-size: 24px; }
.a-meta {
  display: flex; align-items: center; gap: 12px; margin: 12px 0;
  color: var(--bb-text-tertiary); font-size: 13px;
  .a-author { color: var(--bb-text-secondary); font-weight: 500; &:hover { color: var(--bb-primary); } }
}
.a-views { display: inline-flex; align-items: center; gap: 4px; }
.a-actions { margin-bottom: 12px; }
.a-content { border-top: 1px solid var(--bb-border); padding-top: 16px; line-height: 1.8; word-break: break-word; }
.comments-card { padding: 20px 24px; h3 { margin-top: 0; } }
.comment-input-row { display: flex; gap: 10px; margin-bottom: 14px; }
.comments-empty { color: var(--bb-text-tertiary); font-size: 13px; }
.comment-list { list-style: none; margin: 0; padding: 0; }
.comment-item {
  display: flex; gap: 10px; padding: 10px 0;
  border-bottom: 1px solid var(--bb-border); font-size: 14px;
}
.comment-user { font-weight: 600; color: var(--bb-text-secondary); flex-shrink: 0; }
.comment-content { flex: 1; }
.comment-time { color: var(--bb-text-tertiary); font-size: 12px; flex-shrink: 0; }
@media (max-width: 768px) { .a-card { padding: 16px; } }
</style>
