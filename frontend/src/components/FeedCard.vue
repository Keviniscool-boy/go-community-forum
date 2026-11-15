<script setup lang="ts">
// Feed 内容卡片：标题、作者/话题、摘要、互动数据
import { ChatDotRound, Star, CaretTop, View } from '@element-plus/icons-vue'
import type { FeedItem } from '@/api/feed'

defineProps<{ item: FeedItem }>()

// 条目链接：回答跳问题详情，问题跳问题详情
function itemLink(item: FeedItem): string {
  return item.item_type === 'answer' ? `/questions/${item.question_id}` : `/questions/${item.item_id}`
}
</script>

<template>
  <article class="feed-card bb-card">
    <router-link class="card-title" :to="itemLink(item)">{{ item.title }}</router-link>
    <div class="card-meta">
      <router-link class="meta-author" :to="`/users/${item.author.user_id}`">{{ item.author.username }}</router-link>
      <span v-if="item.item_type === 'answer'" class="meta-type">回答了问题</span>
      <el-tag v-if="item.topic" size="small" effect="plain" class="meta-topic">{{ item.topic.name }}</el-tag>
      <span class="meta-time">{{ item.create_time?.slice(0, 10) }}</span>
    </div>
    <p class="card-excerpt">{{ item.excerpt }}</p>
    <div class="card-actions">
      <span v-if="item.item_type === 'answer'" class="action-item">
        <el-icon><CaretTop /></el-icon>{{ item.stats.vote_count }} 赞同
      </span>
      <span v-if="item.item_type === 'answer'" class="action-item">
        <el-icon><Star /></el-icon>{{ item.stats.collect_count }} 收藏
      </span>
      <span v-if="item.item_type === 'question'" class="action-item">
        {{ item.stats.answer_count || 0 }} 回答
      </span>
      <span class="action-item"><el-icon><ChatDotRound /></el-icon>{{ item.stats.comment_count }} 评论</span>
      <span class="action-item"><el-icon><View /></el-icon>浏览</span>
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
  align-items: center;
  gap: 12px;
  margin-top: 6px;
  font-size: 13px;
  color: var(--bb-text-tertiary);
}

.meta-author {
  color: var(--bb-text-secondary);
  font-weight: 500;

  &:hover {
    color: var(--bb-primary);
  }
}

.meta-topic {
  border-radius: 4px;
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
}
</style>
