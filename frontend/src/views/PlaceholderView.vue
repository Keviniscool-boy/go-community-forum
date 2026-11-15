<script setup lang="ts">
// 通用占位页：M5 上线的页面先显示占位
import { useRoute } from 'vue-router'

const route = useRoute()
</script>

<template>
  <div class="bb-card placeholder-card">
    <el-empty :description="`「${route.meta.title}」页面建设中，敬请期待`" />
  </div>
</template>

<style scoped lang="less">
.placeholder-card {
  padding: 40px;
}
</style>
