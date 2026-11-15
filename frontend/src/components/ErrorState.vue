<script setup lang="ts">
// 错误状态，支持重试
import { WarningFilled } from '@element-plus/icons-vue'

defineProps<{ text?: string }>()
const emit = defineEmits<{ (e: 'retry'): void }>()
</script>

<template>
  <div class="error-state">
    <el-icon><WarningFilled /></el-icon>
    <p>{{ text || '加载失败，请稍后重试' }}</p>
    <el-button size="small" @click="emit('retry')">重试</el-button>
  </div>
</template>

<style scoped lang="less">
.error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 40px 0;
  color: var(--bb-text-tertiary);

  .el-icon {
    font-size: 28px;
    color: #f56c6c;
  }
}
</style>
