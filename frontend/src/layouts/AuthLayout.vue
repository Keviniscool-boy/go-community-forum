<script setup lang="ts">
// 认证布局：居中卡片，登录/注册共用
</script>

<template>
  <div class="auth-layout">
    <div class="auth-brand">问见</div>
    <div class="auth-card bb-card">
      <router-view />
    </div>
  </div>
</template>

<style scoped lang="less">
.auth-layout {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 16px;
}

.auth-brand {
  font-size: 24px;
  font-weight: 700;
  color: var(--bb-primary);
  margin-bottom: 24px;
}

.auth-card {
  width: 100%;
  max-width: 400px;
  padding: 28px 32px;
}
</style>
