<script setup lang="ts">
// 空状态
defineProps<{ text?: string }>()
</script>

<template>
  <el-empty :description="text || '暂无内容'" :image-size="80" />
</template>
