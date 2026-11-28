<script setup lang="ts">
// 个人资料设置：简介与头像地址（头像上传待接入文件存储后开放）
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { updateProfile } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import { uploadImage } from '@/api/media'

const auth = useAuthStore()

const username = ref('')
const bio = ref('')
const avatarUrl = ref('')
const avatarUploading = ref(false)
const avatarInput = ref<HTMLInputElement | null>(null)

// 头像上传（PRD F5）：客户端 canvas 裁剪 1:1、压缩到 ≤512×512、≤2MB，再走统一媒体管线
async function onAvatarPick(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  ;(event.target as HTMLInputElement).value = ''
  if (!file.type.startsWith('image/')) return ElMessage.error('请选择图片文件')
  avatarUploading.value = true
  try {
    const bitmap = await createImageBitmap(file)
    const size = Math.min(bitmap.width, bitmap.height)
    const canvas = document.createElement('canvas')
    canvas.width = canvas.height = Math.min(size, 512)
    const ctx = canvas.getContext('2d')!
    ctx.drawImage(
      bitmap,
      (bitmap.width - size) / 2, (bitmap.height - size) / 2, size, size,
      0, 0, canvas.width, canvas.height,
    )
    const blob: Blob = await new Promise((resolve) =>
      canvas.toBlob((b) => resolve(b as Blob), 'image/jpeg', 0.9),
    )
    if (blob.size > 2 * 1024 * 1024) return ElMessage.error('图片压缩后仍超过 2MB')
    const res = await uploadImage(blob, 'avatar')
    avatarUrl.value = res.url
    ElMessage.success('头像已上传，记得保存资料')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
    else ElMessage.error('头像处理失败')
  } finally {
    avatarUploading.value = false
  }
}
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
        <div class="avatar-row">
          <input type="file" accept="image/*" hidden ref="avatarInput" @change="onAvatarPick" />
          <el-button :loading="avatarUploading" @click="(avatarInput as HTMLInputElement).click()">
            上传头像
          </el-button>
          <span class="avatar-hint">自动裁剪为正方形，≤512×512</span>
        </div>
        <el-input v-model="avatarUrl" maxlength="500" placeholder="https://… 或站内 /media/ 地址" />
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

.avatar-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;

  .avatar-hint {
    color: var(--bb-text-tertiary);
    font-size: 12px;
  }
}
