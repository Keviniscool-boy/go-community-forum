<script setup lang="ts">
// 找回密码：邮箱验证码重置；成功后旧会话全部失效，需用新密码重新登录
import { ref, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Message as MessageIcon, Lock } from '@element-plus/icons-vue'
import { sendAuthCode, resetPassword } from '@/api/auth'
import { ApiError } from '@/api/http'

const router = useRouter()

const email = ref('')
const code = ref('')
const password = ref('')
const rePassword = ref('')
const submitting = ref(false)
const sending = ref(false)
const countdown = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

const emailValid = () => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value)

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

async function onSubmit() {
  if (!emailValid()) return ElMessage.warning('请输入正确的邮箱地址')
  if (!code.value) return ElMessage.warning('请输入验证码')
  if (password.value.length < 8) return ElMessage.warning('密码至少 8 位')
  if (password.value !== rePassword.value) return ElMessage.warning('两次密码不一致')
  submitting.value = true
  try {
    await resetPassword({
      email: email.value.trim(),
      code: code.value.trim(),
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
    <el-form @submit.prevent="onSubmit">
      <el-form-item>
        <div class="code-row">
          <el-input v-model="email" type="email" placeholder="注册邮箱" size="large" :prefix-icon="MessageIcon" />
          <el-button size="large" :loading="sending" :disabled="countdown > 0" @click="onSendCode">
            {{ countdown > 0 ? `${countdown}s` : '发送验证码' }}
          </el-button>
        </div>
      </el-form-item>
      <el-form-item>
        <el-input v-model="code" placeholder="6 位验证码" size="large" maxlength="6" />
      </el-form-item>
      <el-form-item>
        <el-input v-model="password" type="password" placeholder="新密码（至少 8 位）" size="large" :prefix-icon="Lock" show-password />
      </el-form-item>
      <el-form-item>
        <el-input v-model="rePassword" type="password" placeholder="确认新密码" size="large" :prefix-icon="Lock" show-password @keyup.enter="onSubmit" />
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
