<script setup lang="ts">
// 私信隐私设置：incoming policy + 已读回执
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getMessageSettings, putMessageSettings } from '@/api/messages'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const loading = ref(true)
const policy = ref('following')
const receipts = ref(true)
const version = ref(0)
const saving = ref(false)

async function load() {
  loading.value = true
  try {
    const res = await getMessageSettings()
    policy.value = res.incoming_policy
    receipts.value = res.read_receipts_enabled
    version.value = res.version
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const res = await putMessageSettings({
      incoming_policy: policy.value,
      read_receipts_enabled: receipts.value,
      version: version.value,
    })
    version.value = res.version
    ElMessage.success('已保存')
  } catch {
    // 版本冲突：重新读取
    await load()
    ElMessage.warning('设置已被其他设备更新，已重新读取')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  if (!auth.isLoggedIn) return
  load()
})
</script>

<template>
  <div class="privacy-view">
    <div class="bb-card pref-card">
      <h3>谁能给我发私信</h3>
      <el-radio-group v-model="policy" class="policy-group">
        <el-radio value="nobody">任何人都不可以</el-radio>
        <el-radio value="following">仅我关注的人</el-radio>
        <el-radio value="everyone">所有已登录用户</el-radio>
      </el-radio-group>
      <p class="pref-desc">默认为"仅我关注的人"。设置只影响新请求；已建立的会话保持可用，你可以在会话中单独关闭。</p>
    </div>
    <div class="bb-card pref-card">
      <h3>已读回执</h3>
      <div class="pref-row">
        <p class="pref-desc" style="margin: 0">关闭后对方看不到你的已读状态，你自己的未读计数仍正常。</p>
        <el-switch v-model="receipts" />
      </div>
    </div>
    <el-button type="primary" :loading="saving" @click="save">保存</el-button>
  </div>
</template>

<style scoped lang="less">
.privacy-view { max-width: 640px; }
.pref-card { padding: 20px 24px; margin-bottom: 12px; h3 { margin-top: 0; } }
.pref-row { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
.pref-desc { color: var(--bb-text-tertiary); font-size: 12px; }
.policy-group { display: flex; flex-direction: column; gap: 8px; }
</style>
