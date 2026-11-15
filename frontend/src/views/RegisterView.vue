<script setup lang="ts">
// 注册页（M1 用户名密码注册；M2 上线邮箱注册后替换）
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'

const auth = useAuthStore()
const router = useRouter()

const username = ref('')
const password = ref('')
const rePassword = ref('')
const loading = ref(false)

async function onSubmit() {
  if (!username.value) return ElMessage.warning('请输入用户名')
  if (password.value.length < 8) return ElMessage.warning('密码至少 8 位')
  if (password.value !== rePassword.value) return ElMessage.warning('两次密码不一致')
  loading.value = true
  try {
    await auth.signup(username.value, password.value, rePassword.value)
    ElMessage.success('注册成功，请登录')
    router.push('/login')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div>
    <h2 class="auth-title">注册</h2>
    <el-form @submit.prevent="onSubmit">
      <el-form-item>
        <el-input v-model="username" placeholder="用户名" size="large" />
      </el-form-item>
      <el-form-item>
        <el-input v-model="password" type="password" placeholder="密码（至少 8 位）" size="large" show-password />
      </el-form-item>
      <el-form-item>
        <el-input v-model="rePassword" type="password" placeholder="确认密码" size="large" show-password @keyup.enter="onSubmit" />
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
