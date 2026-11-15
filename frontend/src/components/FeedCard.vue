<script setup lang="ts">
// Feed 内容卡片：标题、作者/社区、摘要、互动操作
import { ChatDotRound, Star, CaretTop } from '@element-plus/icons-vue'
import type { PostDetail } from '@/types/api'

defineProps<{ post: PostDetail }>()
</script>

<template>
  <article class="feed-card bb-card">
    <router-link class="card-title" :to="`/post/${post.id}`">{{ post.title }}</router-link>
    <div class="card-meta">
      <span class="meta-author">{{ post.author_name }}</span>
      <span v-if="post.name" class="meta-community">发表于「{{ post.name }}」</span>
      <span class="meta-time">{{ post.create_time?.slice(0, 10) }}</span>
    </div>
    <p class="card-excerpt">{{ post.content }}</p>
    <div class="card-actions">
      <span class="action-item"><el-icon><ChatDotRound /></el-icon>评论</span>
      <span class="action-item"><el-icon><Star /></el-icon>收藏</span>
      <span class="action-item"><el-icon><CaretTop /></el-icon>赞同</span>
    </div>
  </article>
</template>

<style scoped lang="less">
.feed-card {
  padding: 16px 20px;
  margin-bottom: 12px;
}

.card-title {
  display: block;
  font-size: 17px;
  font-weight: 600;
  color: var(--bb-text-main);

  &:hover {
    color: var(--bb-primary);
  }
}

.card-meta {
  display: flex;
  gap: 12px;
  margin-top: 6px;
  font-size: 13px;
  color: var(--bb-text-tertiary);
}

.card-excerpt {
  margin: 10px 0;
  color: var(--bb-text-secondary);
  font-size: 14px;
  // 摘要最多三行
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.card-actions {
  display: flex;
  gap: 20px;
  padding-top: 8px;
  border-top: 1px solid var(--bb-border);
}

.action-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  color: var(--bb-text-tertiary);
  cursor: pointer;

  &:hover {
    color: var(--bb-primary);
  }
}
</style>
