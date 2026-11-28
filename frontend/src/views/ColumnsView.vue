<script setup lang="ts">
// 专栏列表页：浏览 + 创建
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { listColumns, createColumn, type ColumnDetail } from '@/api/article'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'

const router = useRouter()
const auth = useAuthStore()
const columns = ref<ColumnDetail[]>([])
const loading = ref(false)
const createVisible = ref(false)
const newName = ref('')
const newDesc = ref('')
const creating = ref(false)

async function load() {
  loading.value = true
  try {
    columns.value = (await listColumns(50)) || []
  } finally {
    loading.value = false
  }
}

async function onCreate() {
  if (!newName.value.trim()) return ElMessage.warning('请输入专栏名称')
  creating.value = true
  try {
    const res = await createColumn({ name: newName.value.trim(), description: newDesc.value.trim() })
    createVisible.value = false
    newName.value = ''
    newDesc.value = ''
    ElMessage.success('专栏已创建')
    router.push(`/columns/${res.column_id}`)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    creating.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="columns-view">
    <div class="bb-card columns-header">
      <h3>专栏</h3>
      <el-button v-if="auth.isLoggedIn" type="primary" size="small" @click="createVisible = true">
        创建专栏
      </el-button>
    </div>
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="columns.length === 0" class="bb-card"><EmptyState text="还没有专栏" /></div>
    <template v-else>
      <article v-for="c in columns" :key="c.id" class="bb-card column-card" @click="router.push(`/columns/${c.id}`)">
        <div class="column-card-body">
          <h3 class="column-name">{{ c.name }}</h3>
          <p class="column-desc">{{ c.description || '暂无简介' }}</p>
          <div class="column-meta">
            {{ c.article_count }} 篇文章 · 创建者 {{ c.owner_name }} · {{ c.create_time?.slice(0, 10) }}
          </div>
        </div>
      </article>
    </template>

    <el-dialog v-model="createVisible" title="创建专栏" width="90%" style="max-width: 520px">
      <el-form label-position="top">
        <el-form-item label="名称">
          <el-input v-model="newName" maxlength="64" show-word-limit placeholder="专栏名称" />
        </el-form-item>
        <el-form-item label="简介">
          <el-input v-model="newDesc" type="textarea" :rows="4" maxlength="1000" placeholder="专栏简介" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="onCreate">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="less">
.columns-header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 14px 20px; margin-bottom: 12px;
  h3 { margin: 0; }
}
.column-card { padding: 16px 20px; margin-bottom: 12px; cursor: pointer;
  &:hover { border-color: var(--bb-primary); } }
.column-name { margin: 0; font-size: 16px; }
.column-desc { margin: 8px 0; color: var(--bb-text-secondary); font-size: 14px; }
.column-meta { color: var(--bb-text-tertiary); font-size: 12px; }
</style>
