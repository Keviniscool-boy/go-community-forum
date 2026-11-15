<script setup lang="ts">
// 加载中状态
import { Loading } from '@element-plus/icons-vue'

defineProps<{ text?: string }>()
</script>

<template>
  <div class="loading-state">
    <el-icon class="spin"><Loading /></el-icon>
    <span>{{ text || '加载中…' }}</span>
  </div>
</template>

<style scoped lang="less">
.loading-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 40px 0;
  color: var(--bb-text-tertiary);
}

.spin {
  animation: rotate 1s linear infinite;
}

@keyframes rotate {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
