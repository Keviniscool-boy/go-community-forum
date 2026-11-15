<script setup lang="ts">
// 404 页面
</script>

<template>
  <div class="bb-card not-found">
    <el-empty description="页面不存在或已被移除">
      <el-button type="primary" @click="$router.push('/')">回到首页</el-button>
    </el-empty>
  </div>
</template>

<style scoped lang="less">
.not-found {
  padding: 40px;
}
</style>
