<script setup lang="ts">
// 收藏夹：文件夹管理 + 条目列表（目标不可用状态）
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  listFolders, createFolder, deleteFolder, listFolderItems,
  type BookmarkFolder, type BookmarkItemVO,
} from '@/api/productivity'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'

const folders = ref<BookmarkFolder[]>([])
const activeFolder = ref('')
const items = ref<BookmarkItemVO[]>([])
const loading = ref(false)
const itemsLoading = ref(false)
const newFolderName = ref('')

async function loadFolders() {
  loading.value = true
  try {
    folders.value = (await listFolders()) || []
    if (folders.value.length > 0 && !activeFolder.value) {
      selectFolder(folders.value[0].folder_id)
    }
  } finally {
    loading.value = false
  }
}

async function selectFolder(id: string) {
  activeFolder.value = id
  itemsLoading.value = true
  try {
    items.value = (await listFolderItems(id)) || []
  } finally {
    itemsLoading.value = false
  }
}

async function onCreateFolder() {
  if (!newFolderName.value.trim()) return
  try {
    await createFolder(newFolderName.value.trim())
    newFolderName.value = ''
    await loadFolders()
    ElMessage.success('收藏夹已创建')
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message)
  }
}

async function onDeleteFolder(f: BookmarkFolder) {
  try {
    await ElMessageBox.confirm(`删除收藏夹「${f.name}」及其全部收藏？`, '删除收藏夹', { type: 'warning' })
  } catch { return }
  await deleteFolder(f.folder_id)
  activeFolder.value = ''
  await loadFolders()
}

function itemLink(it: BookmarkItemVO): string {
  return it.target_type === 1 ? `/questions/${it.target_id}` : `/questions/${it.target_id}`
}

onMounted(loadFolders)
</script>

<template>
  <div class="bookmarks-view">
    <div class="bb-card folder-panel">
      <div class="folder-create">
        <el-input v-model="newFolderName" size="small" placeholder="新建收藏夹名称" maxlength="64" @keyup.enter="onCreateFolder" />
        <el-button size="small" type="primary" @click="onCreateFolder">创建</el-button>
      </div>
      <div v-for="f in folders" :key="f.folder_id" class="folder-item" :class="{ active: activeFolder === f.folder_id }" @click="selectFolder(f.folder_id)">
        <span>{{ f.name }}</span>
        <el-button v-if="f.name !== '我的收藏'" text size="small" type="danger" @click.stop="onDeleteFolder(f)">删除</el-button>
      </div>
    </div>

    <div class="items-panel">
      <div v-if="itemsLoading" class="bb-card"><LoadingState /></div>
      <div v-else-if="items.length === 0" class="bb-card"><EmptyState text="收藏夹还没有内容" /></div>
      <template v-else>
        <article v-for="it in items" :key="`${it.target_type}-${it.target_id}`" class="bb-card bookmark-card" :class="{ unavailable: !it.available }">
          <template v-if="it.available">
            <router-link class="bookmark-link" :to="itemLink(it)">查看内容</router-link>
          </template>
          <template v-else>
            <span class="unavailable-text">内容已不可用</span>
          </template>
          <span class="bookmark-time">{{ it.created_at?.slice(0, 10) }}</span>
        </article>
      </template>
    </div>
  </div>
</template>

<style scoped lang="less">
.bookmarks-view { display: flex; gap: 16px; align-items: flex-start; }
.folder-panel {
  width: 220px; flex-shrink: 0; padding: 12px;
  position: sticky; top: 72px;
}
.folder-create { display: flex; gap: 6px; margin-bottom: 10px; }
.folder-item {
  display: flex; justify-content: space-between; align-items: center;
  padding: 8px 10px; border-radius: 6px; cursor: pointer;
  font-size: 14px; color: var(--bb-text-secondary);
  &:hover { background: var(--bb-bg-page); }
  &.active { background: #e8f3ff; color: var(--bb-primary); font-weight: 600; }
}
.items-panel { flex: 1; min-width: 0; }
.bookmark-card { padding: 14px 18px; margin-bottom: 10px; }
.bookmark-link { color: var(--bb-primary); }
.unavailable-text { color: var(--bb-text-tertiary); }
.bookmark-time { float: right; color: var(--bb-text-tertiary); font-size: 12px; }
@media (max-width: 768px) {
  .bookmarks-view { flex-direction: column; }
  .folder-panel { width: 100%; position: static; }
}
</style>
