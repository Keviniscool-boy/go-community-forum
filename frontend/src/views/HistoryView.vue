<script setup lang="ts">
// 浏览历史
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listHistory, clearHistory, type BookmarkItemVO } from '@/api/productivity'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'

const items = ref<BookmarkItemVO[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    items.value = (await listHistory()) || []
  } finally {
    loading.value = false
  }
}

async function onClear() {
  try {
    await ElMessageBox.confirm('清空全部浏览历史？', '清空历史', { type: 'warning' })
  } catch { return }
  await clearHistory()
  ElMessage.success('已清空')
  load()
}

onMounted(load)
</script>

<template>
  <div class="history-view">
    <div class="bb-card history-header">
      <h3>浏览历史</h3>
      <el-button size="small" plain type="danger" @click="onClear">清空历史</el-button>
    </div>
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="items.length === 0" class="bb-card"><EmptyState text="暂无浏览记录" /></div>
    <template v-else>
      <article v-for="it in items" :key="`${it.target_type}-${it.target_id}`" class="bb-card history-card" :class="{ unavailable: !it.available }">
        <template v-if="it.available">
          <router-link class="history-link" :to="`/questions/${it.target_id}`">查看内容</router-link>
        </template>
        <span v-else class="unavailable-text">内容已不可用</span>
        <span class="history-time">{{ it.created_at?.slice(0, 16).replace('T', ' ') }}</span>
      </article>
    </template>
  </div>
</template>

<style scoped lang="less">
.history-header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 14px 20px; margin-bottom: 12px;
  h3 { margin: 0; }
}
.history-card { padding: 12px 18px; margin-bottom: 8px; }
.history-link { color: var(--bb-primary); }
.unavailable-text { color: var(--bb-text-tertiary); }
.history-time { float: right; color: var(--bb-text-tertiary); font-size: 12px; }
</style>
