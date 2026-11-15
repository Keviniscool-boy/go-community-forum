<script setup lang="ts">
// 个人资料设置：简介与头像地址（头像上传待接入文件存储后开放）
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { updateProfile } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'

const auth = useAuthStore()

const username = ref('')
const bio = ref('')
const avatarUrl = ref('')
const submitting = ref(false)

onMounted(() => {
  if (!auth.isLoggedIn) return
  username.value = auth.user?.username || ''
  bio.value = auth.user?.bio || ''
  avatarUrl.value = auth.user?.avatar_url || ''
})

async function onSubmit() {
  if (!auth.isLoggedIn) return
  submitting.value = true
  try {
    await updateProfile({
      username: username.value.trim(),
      bio: bio.value.trim(),
      avatar_url: avatarUrl.value.trim(),
    })
    // 服务端重新拉取用户信息，保持本地与后端一致
    await auth.restoreMe()
    ElMessage.success('资料已更新')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="settings-view bb-card">
    <h2>个人资料</h2>
    <el-form label-position="top" @submit.prevent="onSubmit">
      <el-form-item label="用户名（2-32 字符）">
        <el-input v-model="username" maxlength="32" show-word-limit />
      </el-form-item>
      <el-form-item label="头像地址（图片 URL）">
        <el-input v-model="avatarUrl" maxlength="500" placeholder="https://…" />
      </el-form-item>
      <el-form-item label="个人简介">
        <el-input
          v-model="bio"
          type="textarea"
          :rows="4"
          maxlength="500"
          show-word-limit
          placeholder="介绍一下自己…"
        />
      </el-form-item>
      <el-button type="primary" :loading="submitting" native-type="submit">保存</el-button>
    </el-form>
  </div>
</template>

<style scoped lang="less">
.settings-view {
  padding: 24px 28px;

  h2 {
    margin-top: 0;
  }
}
</style>
