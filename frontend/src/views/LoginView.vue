<script setup lang="ts">
// 登录页（M1 用户名密码登录；M2 上线邮箱登录后替换）
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const username = ref('')
const password = ref('')
const loading = ref(false)

async function onSubmit() {
  if (!username.value || !password.value) return ElMessage.warning('请输入用户名和密码')
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    ElMessage.success('登录成功')
    router.push((route.query.redirect as string) || '/')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div>
    <h2 class="auth-title">登录</h2>
    <el-form @submit.prevent="onSubmit">
      <el-form-item>
        <el-input v-model="username" placeholder="用户名" size="large" />
      </el-form-item>
      <el-form-item>
        <el-input v-model="password" type="password" placeholder="密码" size="large" show-password @keyup.enter="onSubmit" />
      </el-form-item>
      <el-button type="primary" size="large" style="width: 100%" :loading="loading" native-type="submit">
        登录
      </el-button>
    </el-form>
    <div class="auth-footer">
      还没有账号？
      <router-link class="link" to="/register">注册</router-link>
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
