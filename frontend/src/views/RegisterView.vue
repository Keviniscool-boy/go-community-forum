<script setup lang="ts">
// 注册页：邮箱验证码注册，注册成功直接进入登录态
import { ref, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Message as MessageIcon, Lock, User } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { sendAuthCode } from '@/api/auth'
import { ApiError } from '@/api/http'

const auth = useAuthStore()
const router = useRouter()

const email = ref('')
const code = ref('')
const username = ref('')
const password = ref('')
const rePassword = ref('')
const loading = ref(false)
const sending = ref(false)
const countdown = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

const emailValid = () => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value)

// 发送验证码，60 秒倒计时内不可重复点击
async function onSendCode() {
  if (!emailValid()) return ElMessage.warning('请输入正确的邮箱地址')
  if (countdown.value > 0) return
  sending.value = true
  try {
    await sendAuthCode(email.value.trim(), 'register')
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
  if (!username.value) return ElMessage.warning('请输入用户名')
  if (password.value.length < 8) return ElMessage.warning('密码至少 8 位')
  if (password.value !== rePassword.value) return ElMessage.warning('两次密码不一致')
  loading.value = true
  try {
    await auth.register({
      email: email.value.trim(),
      code: code.value.trim(),
      username: username.value.trim(),
      password: password.value,
      re_password: rePassword.value,
    })
    ElMessage.success('注册成功')
    router.push('/')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div>
    <h2 class="auth-title">注册</h2>
    <el-form @submit.prevent="onSubmit">
      <el-form-item>
        <div class="code-row">
          <el-input v-model="email" type="email" placeholder="邮箱" size="large" :prefix-icon="MessageIcon" />
          <el-button size="large" :loading="sending" :disabled="countdown > 0" @click="onSendCode">
            {{ countdown > 0 ? `${countdown}s` : '发送验证码' }}
          </el-button>
        </div>
      </el-form-item>
      <el-form-item>
        <el-input v-model="code" placeholder="6 位验证码" size="large" maxlength="6" @keyup.enter="onSubmit" />
      </el-form-item>
      <el-form-item>
        <el-input v-model="username" placeholder="用户名（2-32 字符）" size="large" :prefix-icon="User" />
      </el-form-item>
      <el-form-item>
        <el-input v-model="password" type="password" placeholder="密码（至少 8 位）" size="large" :prefix-icon="Lock" show-password />
      </el-form-item>
      <el-form-item>
        <el-input v-model="rePassword" type="password" placeholder="确认密码" size="large" :prefix-icon="Lock" show-password @keyup.enter="onSubmit" />
      </el-form-item>
      <el-button type="primary" size="large" style="width: 100%" :loading="loading" native-type="submit">
        注册
      </el-button>
    </el-form>
    <div class="auth-footer">
      已有账号？
      <router-link class="link" to="/login">登录</router-link>
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
