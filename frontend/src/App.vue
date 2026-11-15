<script setup lang="ts">
// 应用根组件：仅渲染路由出口，全局布局在 MainLayout 中
// 启动时恢复登录态（校验 token 并拉取当前用户信息）
import { onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()

onMounted(() => {
  auth.restoreMe()
})
</script>

<template>
  <router-view />
</template>
