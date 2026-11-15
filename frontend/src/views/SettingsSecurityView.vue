<script setup lang="ts">
// 账号安全：退出所有设备
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { logoutAllDevices } from '@/api/productivity'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()
const loading = ref(false)

async function onLogoutAll() {
  try {
    await ElMessageBox.confirm(
      '将撤销当前账号在所有设备上登录状态（包括本机），需要重新登录。',
      '退出所有设备', { type: 'warning' },
    )
  } catch { return }
  loading.value = true
  try {
    await logoutAllDevices()
    auth.clear()
    ElMessage.success('已在所有设备退出登录')
    router.push('/login')
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="security-view bb-card">
    <h2>账号安全</h2>
    <div class="security-item">
      <div>
        <h4>退出所有设备</h4>
        <p>撤销该账号的全部登录会话（包括当前设备）。密码重置后也会自动执行。</p>
      </div>
      <el-button type="danger" plain :loading="loading" @click="onLogoutAll">退出所有设备</el-button>
    </div>
    <div class="security-item">
      <div>
        <h4>修改密码</h4>
        <p>通过「找回密码」流程重置密码，重置后所有旧会话自动失效。</p>
      </div>
      <el-button plain @click="router.push('/forgot-password')">去重置密码</el-button>
    </div>
  </div>
</template>

<style scoped lang="less">
.security-view { padding: 24px 28px; h2 { margin-top: 0; } }
.security-item {
  display: flex; justify-content: space-between; align-items: center; gap: 16px;
  padding: 16px 0; border-top: 1px solid var(--bb-border);
  h4 { margin: 0 0 4px; }
  p { margin: 0; color: var(--bb-text-secondary); font-size: 13px; }
}
</style>
