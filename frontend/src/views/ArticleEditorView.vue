<script setup lang="ts">
// 文章编辑页：创建或编辑（Markdown 源文本 + 服务端安全渲染预览）
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { createArticle, updateArticle, fetchArticle } from '@/api/article'
import { previewMarkdown } from '@/api/productivity'
import { ApiError } from '@/api/http'

const route = useRoute()
const router = useRouter()

const isEdit = computed(() => !!route.params.id)
const articleId = route.params.id as string

const title = ref('')
const content = ref('')
const columnId = ref('')
const submitting = ref(false)
const previewHtml = ref('')
const previewLoading = ref(false)
const showPreview = ref(false)

onMounted(async () => {
  if (isEdit.value) {
    try {
      const a = await fetchArticle(articleId)
      title.value = a.title
      content.value = a.content
      columnId.value = a.column_id
    } catch (e) {
      if (e instanceof ApiError) ElMessage.error(e.message)
    }
  }
})

async function onPreview() {
  if (!content.value.trim()) return
  previewLoading.value = true
  showPreview.value = true
  try {
    const res = await previewMarkdown(content.value)
    previewHtml.value = res.html
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    previewLoading.value = false
  }
}

async function onSubmit() {
  if (!title.value.trim()) return ElMessage.warning('请输入标题')
  if (!content.value.trim()) return ElMessage.warning('请输入正文')
  submitting.value = true
  try {
    if (isEdit.value) {
      await updateArticle(articleId, { title: title.value.trim(), content: content.value })
      ElMessage.success('文章已更新')
      router.push(`/articles/${articleId}`)
    } else {
      const res = await createArticle({
        title: title.value.trim(),
        content: content.value,
        column_id: columnId.value || undefined,
      })
      ElMessage.success('文章已发布')
      router.push(`/articles/${res.article_id}`)
    }
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="editor-view bb-card">
    <h2>{{ isEdit ? '编辑文章' : '写文章' }}</h2>
    <el-input
      v-model="title"
      size="large"
      maxlength="200"
      placeholder="文章标题"
      class="title-input"
    />
    <el-input
      v-model="content"
      type="textarea"
      :rows="16"
      maxlength="50000"
      show-word-limit
      placeholder="正文支持安全 Markdown 子集：# 标题、**粗体**、*斜体*、`代码`、列表、[链接](url)、图片"
      class="content-input"
    />
    <div class="editor-actions">
      <el-button :loading="previewLoading" plain @click="onPreview">预览</el-button>
      <el-button type="primary" :loading="submitting" @click="onSubmit">
        {{ isEdit ? '保存' : '发布' }}
      </el-button>
    </div>
    <div v-if="showPreview" class="preview-box">
      <h4>预览（服务端安全渲染）</h4>
      <div class="preview-content" v-html="previewHtml"></div>
    </div>
  </div>
</template>

<style scoped lang="less">
.editor-view { padding: 24px 28px; h2 { margin-top: 0; } }
.title-input { margin-bottom: 14px; }
.content-input { margin-bottom: 14px; }
.editor-actions { display: flex; gap: 10px; margin-bottom: 16px; }
.preview-box {
  border: 1px solid var(--bb-border); border-radius: var(--bb-radius); padding: 16px;
  h4 { margin: 0 0 8px; color: var(--bb-text-secondary); }
}
.preview-content { line-height: 1.7; word-break: break-word; }
</style>
