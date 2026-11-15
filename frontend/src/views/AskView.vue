<script setup lang="ts">
// 发布帖子页（M1 使用旧 /post 接口；M3 拆分为提问/回答）
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { createPost } from '@/api/post'
import { fetchCommunities } from '@/api/community'
import type { CommunityDetail } from '@/types/api'

const router = useRouter()

const title = ref('')
const content = ref('')
const communityId = ref('')
const communities = ref<CommunityDetail[]>([])
const submitting = ref(false)

onMounted(async () => {
  try {
    communities.value = (await fetchCommunities()) || []
  } catch {
    // 社区列表加载失败不阻塞发帖，只是无法选择社区
  }
})

async function onSubmit() {
  if (!title.value.trim()) return ElMessage.warning('请输入标题')
  if (!content.value.trim()) return ElMessage.warning('请输入内容')
  if (!communityId.value) return ElMessage.warning('请选择社区')
  submitting.value = true
  try {
    await createPost({
      title: title.value.trim(),
      content: content.value.trim(),
      community_id: communityId.value,
    })
    ElMessage.success('发布成功')
    router.push('/')
  } catch (e) {
    if (e instanceof Error && e.message) ElMessage.error(e.message)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="ask-view bb-card">
    <h2>发布帖子</h2>
    <el-form label-position="top">
      <el-form-item label="标题">
        <el-input v-model="title" maxlength="128" show-word-limit placeholder="一句话说清主题" />
      </el-form-item>
      <el-form-item label="社区">
        <el-select v-model="communityId" placeholder="选择所属社区" style="width: 100%">
          <el-option
            v-for="c in communities"
            :key="c.id"
            :label="c.name"
            :value="c.id"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="内容">
        <el-input
          v-model="content"
          type="textarea"
          :rows="10"
          maxlength="8192"
          show-word-limit
          placeholder="正文内容"
        />
      </el-form-item>
      <el-button type="primary" :loading="submitting" @click="onSubmit">发布</el-button>
    </el-form>
  </div>
</template>

<style scoped lang="less">
.ask-view {
  padding: 24px 28px;

  h2 {
    margin-top: 0;
  }
}
</style>
