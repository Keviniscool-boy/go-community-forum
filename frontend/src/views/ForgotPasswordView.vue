<script setup lang="ts">
// 找回密码（两段式）：第一步验证码换一次性重置凭据，第二步凭据设置新密码
// 完成后所有旧会话失效，需用新密码重新登录
import { ref, onBeforeUnmount, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Message as MessageIcon, Lock, Key } from '@element-plus/icons-vue'
import { sendAuthCode, verifyResetCode, resetPasswordWithToken } from '@/api/auth'
import { ApiError } from '@/api/http'

const router = useRouter()

const step = ref<1 | 2>(1)
const email = ref('')
const code = ref('')
const resetToken = ref('')
const password = ref('')
const rePassword = ref('')
const submitting = ref(false)
const sending = ref(false)
const countdown = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

const emailValid = () => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value)
const expiresText = computed(() => '重置凭据已就绪（10 分钟内有效）')

async function onSendCode() {
  if (!emailValid()) return ElMessage.warning('请输入正确的邮箱地址')
  if (countdown.value > 0) return
  sending.value = true
  try {
    await sendAuthCode(email.value.trim(), 'reset_password')
    ElMessage.success('验证码已发送，请查收邮件')
    countdown.value = 60
    timer = setInterval(() => {
      countdown.value -= 1
      if (countdown.value <= 0 && timer) {
        clearInterval(timer)
        timer = null
      }
    }, 1000)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    sending.value = false
  }
}

// 第一步：验证码换取一次性重置凭据（明文凭据只在本次响应返回）
async function onVerify() {
  if (!emailValid()) return ElMessage.warning('请输入正确的邮箱地址')
  if (!code.value) return ElMessage.warning('请输入验证码')
  submitting.value = true
  try {
    const res = await verifyResetCode(email.value.trim(), code.value.trim())
    resetToken.value = res.reset_token
    step.value = 2
    ElMessage.success('验证通过，请设置新密码')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    submitting.value = false
  }
}

// 第二步：凭据单次消费，改密并撤销所有旧会话
async function onReset() {
  if (password.value.length < 8) return ElMessage.warning('密码至少 8 位')
  if (password.value !== rePassword.value) return ElMessage.warning('两次密码不一致')
  submitting.value = true
  try {
    await resetPasswordWithToken({
      reset_token: resetToken.value,
      password: password.value,
      re_password: rePassword.value,
    })
    ElMessage.success('密码已重置，所有旧会话已失效，请用新密码登录')
    router.push('/login')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    submitting.value = false
  }
}

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div>
    <h2 class="auth-title">找回密码</h2>

    <el-steps v-if="step === 1" :active="0" align-center class="steps">
      <el-step title="验证身份" />
      <el-step title="设置新密码" />
    </el-steps>

    <el-form v-if="step === 1" @submit.prevent="onVerify">
      <el-form-item>
        <div class="code-row">
          <el-input v-model="email" type="email" placeholder="注册邮箱" size="large" :prefix-icon="MessageIcon" />
          <el-button size="large" :loading="sending" :disabled="countdown > 0" @click="onSendCode">
            {{ countdown > 0 ? `${countdown}s` : '发送验证码' }}
          </el-button>
        </div>
      </el-form-item>
      <el-form-item>
        <el-input v-model="code" placeholder="6 位验证码" size="large" maxlength="6" :prefix-icon="Key" @keyup.enter="onVerify" />
      </el-form-item>
      <el-button type="primary" size="large" style="width: 100%" :loading="submitting" native-type="submit">
        验证身份
      </el-button>
    </el-form>

    <el-form v-else @submit.prevent="onReset">
      <el-steps :active="1" align-center class="steps">
        <el-step title="验证身份" />
        <el-step title="设置新密码" />
      </el-steps>
      <p class="reset-hint">{{ expiresText }}</p>
      <el-form-item>
        <el-input v-model="password" type="password" placeholder="新密码（至少 8 位）" size="large" :prefix-icon="Lock" show-password />
      </el-form-item>
      <el-form-item>
        <el-input v-model="rePassword" type="password" placeholder="确认新密码" size="large" :prefix-icon="Lock" show-password @keyup.enter="onReset" />
      </el-form-item>
      <el-button type="primary" size="large" style="width: 100%" :loading="submitting" native-type="submit">
        重置密码
      </el-button>
    </el-form>

    <div class="auth-footer">
      想起密码了？
      <router-link class="link" to="/login">返回登录</router-link>
    </div>
  </div>
</template>

<style scoped lang="less">
.auth-title {
  margin: 0 0 20px;
  text-align: center;
}

.steps {
  margin-bottom: 24px;
}

.reset-hint {
  margin: 0 0 16px;
  color: var(--bb-text-secondary);
  font-size: 13px;
  text-align: center;
}

.code-row {
  display: flex;
  gap: 10px;
  width: 100%;

  .el-button {
    flex-shrink: 0;
    min-width: 110px;
  }
}

.auth-footer {
  margin-top: 16px;
  text-align: center;
  color: var(--bb-text-secondary);
  font-size: 14px;

  .link {
    color: var(--bb-primary);
  }
}
</style>
