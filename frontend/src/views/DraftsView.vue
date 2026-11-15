<script setup lang="ts">
// 草稿箱：列表、发布、删除
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listDrafts, publishDraft, deleteDraft, type Draft } from '@/api/productivity'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'

const router = useRouter()
const drafts = ref<Draft[]>([])
const loading = ref(false)
const tab = ref<'question' | 'answer'>('question')

async function load() {
  loading.value = true
  try {
    drafts.value = (await listDrafts(tab.value)) || []
  } finally {
    loading.value = false
  }
}

async function onPublish(d: Draft) {
  try {
    await ElMessageBox.confirm('发布将重新执行安全审核，确定发布？', '发布草稿', { type: 'info' })
  } catch { return }
  try {
    const res = await publishDraft(d.id)
    ElMessage.success('发布成功')
    if (res.question_id) router.push(`/questions/${res.question_id}`)
    else load()
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message)
  }
}

async function onDelete(d: Draft) {
  try {
    await ElMessageBox.confirm('确定删除该草稿？', '删除草稿', { type: 'warning' })
  } catch { return }
  await deleteDraft(d.id)
  ElMessage.success('已删除')
  load()
}

onMounted(load)
</script>

<template>
  <div class="drafts-view">
    <div class="bb-card drafts-tabs">
      <el-radio-group v-model="tab" @change="load">
        <el-radio-button value="question">问题草稿</el-radio-button>
        <el-radio-button value="answer">回答草稿</el-radio-button>
      </el-radio-group>
    </div>
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="drafts.length === 0" class="bb-card"><EmptyState text="暂无草稿" /></div>
    <template v-else>
      <article v-for="d in drafts" :key="d.id" class="bb-card draft-card">
        <h3 class="draft-title">{{ d.title || '（无标题）' }}</h3>
        <p class="draft-excerpt">{{ d.content.slice(0, 120) }}</p>
        <div class="draft-actions">
          <span class="draft-time">{{ d.update_time?.slice(0, 16).replace('T', ' ') }}</span>
          <el-button size="small" type="primary" plain @click="onPublish(d)">发布</el-button>
          <el-button size="small" plain type="danger" @click="onDelete(d)">删除</el-button>
        </div>
      </article>
    </template>
  </div>
</template>

<style scoped lang="less">
.drafts-tabs { padding: 10px 16px; margin-bottom: 12px; }
.draft-card { padding: 16px 20px; margin-bottom: 10px; }
.draft-title { margin: 0; font-size: 16px; }
.draft-excerpt { color: var(--bb-text-secondary); font-size: 13px; margin: 8px 0; }
.draft-actions { display: flex; align-items: center; gap: 8px; }
.draft-time { color: var(--bb-text-tertiary); font-size: 12px; margin-right: auto; }
</style>
