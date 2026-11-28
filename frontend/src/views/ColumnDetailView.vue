<script setup lang="ts">
// 专栏页：专栏信息 + 文章目录 + 所有者管理
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  fetchColumn, fetchColumnArticles, updateColumn, deleteColumn,
  type ColumnDetail, type ArticleDetail,
} from '@/api/article'
import { createSeries, listSeriesForColumn, type Series } from '@/api/collab'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const columnId = route.params.id as string
const column = ref<ColumnDetail | null>(null)
const articles = ref<ArticleDetail[]>([])
const seriesList = ref<Series[]>([])
const newSeriesTitle = ref('')
const creatingSeries = ref(false)
const loading = ref(false)
const error = ref('')

const isOwner = computed(() => !!auth.user && column.value?.owner_id === auth.user.user_id)
const editVisible = ref(false)
const editName = ref('')
const editDesc = ref('')
const editSubmitting = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    column.value = await fetchColumn(columnId)
    articles.value = (await fetchColumnArticles(columnId)) || []
    try {
      seriesList.value = (await listSeriesForColumn(columnId)) || []
    } catch {
      seriesList.value = []
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

function openEdit() {
  if (!column.value) return
  editName.value = column.value.name
  editDesc.value = column.value.description
  editVisible.value = true
}

async function submitEdit() {
  if (!editName.value.trim()) return ElMessage.warning('名称不能为空')
  editSubmitting.value = true
  try {
    await updateColumn(columnId, { name: editName.value.trim(), description: editDesc.value.trim() })
    editVisible.value = false
    ElMessage.success('专栏已更新')
    load()
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    editSubmitting.value = false
  }
}

async function onCreateSeries() {
  if (!newSeriesTitle.value.trim()) return ElMessage.warning('请输入系列标题')
  creatingSeries.value = true
  try {
    const res = await createSeries(columnId, { title: newSeriesTitle.value.trim(), description: '' })
    ElMessage.success('系列已创建')
    router.push(`/series/${res.series_id}`)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    creatingSeries.value = false
  }
}

async function onDelete() {
  try {
    await ElMessageBox.confirm(
      '删除专栏后，其中文章会转为无专栏文章。确定删除？', '删除专栏', { type: 'warning' },
    )
  } catch { return }
  try {
    await deleteColumn(columnId)
    ElMessage.success('专栏已删除')
    router.push('/columns')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  }
}

onMounted(load)
</script>

<template>
  <div class="column-view">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card"><ErrorState :text="error" @retry="load" /></div>
    <template v-else-if="column">
      <div class="bb-card column-header">
        <div class="column-info">
          <h1 class="column-name">{{ column.name }}</h1>
          <p class="column-desc">{{ column.description || '暂无简介' }}</p>
          <span class="column-meta">
            {{ column.article_count }} 篇文章 · 创建者
            <router-link :to="`/users/${column.owner_id}`">{{ column.owner_name }}</router-link>
          </span>
        </div>
        <div v-if="isOwner" class="column-actions">
          <el-button size="small" plain @click="openEdit">编辑</el-button>
          <el-button size="small" plain type="danger" @click="onDelete">删除</el-button>
        </div>
      </div>

      <div class="bb-card column-articles-header">
        <h3>系列</h3>
        <div class="series-create-row">
          <el-input v-model="newSeriesTitle" size="small" placeholder="新系列标题" maxlength="100" style="width: 220px" />
          <el-button size="small" type="primary" :loading="creatingSeries" @click="onCreateSeries">创建系列</el-button>
        </div>
      </div>
      <div v-if="seriesList.length > 0" class="bb-card series-list">
        <router-link v-for="s in seriesList" :key="s.id" class="series-row" :to="`/series/${s.id}`">
          <span class="series-name">{{ s.title }}</span>
          <span class="series-arrow">→</span>
        </router-link>
      </div>
      <div class="bb-card column-articles-header">
        <h3>文章目录</h3>
        <el-button v-if="isOwner" type="primary" size="small" @click="router.push('/articles/new')">
          写新文章
        </el-button>
      </div>
      <div v-if="articles.length === 0" class="bb-card"><EmptyState text="该专栏下还没有文章" /></div>
      <article v-for="(a, idx) in articles" :key="a.id" class="bb-card catalog-card">
        <span class="catalog-idx">{{ idx + 1 }}</span>
        <router-link class="catalog-title" :to="`/articles/${a.id}`">{{ a.title }}</router-link>
        <span class="catalog-meta">{{ a.create_time?.slice(0, 10) }}</span>
      </article>

      <el-dialog v-model="editVisible" title="编辑专栏" width="90%" style="max-width: 520px">
        <el-form label-position="top">
          <el-form-item label="名称">
            <el-input v-model="editName" maxlength="64" show-word-limit />
          </el-form-item>
          <el-form-item label="简介">
            <el-input v-model="editDesc" type="textarea" :rows="4" maxlength="1000" />
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button @click="editVisible = false">取消</el-button>
          <el-button type="primary" :loading="editSubmitting" @click="submitEdit">保存</el-button>
        </template>
      </el-dialog>
    </template>
  </div>
</template>

<style scoped lang="less">
.column-header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 24px 28px; margin-bottom: 12px;
}
.column-name { margin: 0; font-size: 22px; }
.column-desc { margin: 8px 0; color: var(--bb-text-secondary); }
.column-meta { color: var(--bb-text-tertiary); font-size: 13px;
  a { color: var(--bb-primary); } }
.column-articles-header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 12px 20px; margin-bottom: 10px;
  h3 { margin: 0; }
}
.catalog-card {
  display: flex; align-items: center; gap: 12px;
  padding: 14px 20px; margin-bottom: 8px;
}
.catalog-idx {
  width: 24px; height: 24px; border-radius: 50%;
  background: #e8f3ff; color: var(--bb-primary);
  display: flex; align-items: center; justify-content: center;
  font-size: 13px; font-weight: 600; flex-shrink: 0;
}
.catalog-title { flex: 1; font-weight: 600; color: var(--bb-text-main);
  &:hover { color: var(--bb-primary); } }
.catalog-meta { color: var(--bb-text-tertiary); font-size: 12px; }
.series-create-row { display: flex; gap: 8px; align-items: center; }
.series-list { padding: 8px 16px; margin-bottom: 12px; }
.series-row {
  display: flex; justify-content: space-between; padding: 8px 4px;
  color: var(--bb-text-main); border-bottom: 1px solid var(--bb-border);
  &:last-child { border-bottom: none; }
  &:hover .series-name { color: var(--bb-primary); }
}
@media (max-width: 768px) { .column-header { padding: 16px; } }
</style>
