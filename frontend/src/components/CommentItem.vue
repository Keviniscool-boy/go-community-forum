<script setup lang="ts">
// 单条评论：一级评论与楼中楼回复共用；操作事件交给父组件处理
import { Pointer } from '@element-plus/icons-vue'
import type { CommentDetail } from '@/api/interaction'

defineProps<{
  comment: CommentDetail
}>()

defineEmits<{
  (e: 'like'): void
  (e: 'reply'): void
  (e: 'delete'): void
  (e: 'report'): void
}>()

// 2026-11-27T17:36:09+08:00 → 2026-11-27 17:36
const fmtTime = (s?: string) => (s || '').slice(0, 16).replace('T', ' ')
</script>

<template>
  <div class="ci" :class="{ 'is-deleted': comment.is_deleted }">
    <div class="ci-main">
      <span class="ci-user">{{ comment.is_deleted ? '该评论已删除' : comment.username }}</span>
      <span v-if="!comment.is_deleted && comment.reply_to_user" class="ci-reply-to">回复 @{{ comment.reply_to_user }}</span>
      <span v-if="!comment.is_deleted" class="ci-content">{{ comment.content }}</span>
    </div>
    <div v-if="!comment.is_deleted" class="ci-actions">
      <button class="ci-btn" :class="{ active: comment.liked }" type="button" @click="$emit('like')">
        <el-icon><Pointer /></el-icon>{{ comment.liked ? '已赞' : '赞' }}<template v-if="comment.like_count"> · {{ comment.like_count }}</template>
      </button>
      <button class="ci-btn" type="button" @click="$emit('reply')">回复</button>
      <button v-if="comment.can_delete" class="ci-btn danger" type="button" @click="$emit('delete')">删除</button>
      <button class="ci-btn" type="button" @click="$emit('report')">举报</button>
      <span class="ci-time">{{ fmtTime(comment.create_time) }}</span>
    </div>
    <div v-else class="ci-actions">
      <span class="ci-time">{{ fmtTime(comment.create_time) }}</span>
    </div>
  </div>
</template>

<style scoped lang="less">
.ci {
  padding: 10px 0;
  border-bottom: 1px solid var(--bb-border);
  font-size: 14px;
}

.ci-main {
  display: flex;
  flex-wrap: wrap;
  gap: 0 6px;
  align-items: baseline;
  line-height: 1.6;
}

.ci-user {
  font-weight: 600;
  color: var(--bb-text-secondary);
}

.is-deleted .ci-user {
  font-weight: 400;
  color: var(--bb-text-tertiary);
}

.ci-reply-to {
  color: var(--bb-primary);
}

.ci-content {
  color: var(--bb-text-main);
  white-space: pre-wrap;
  word-break: break-word;
}

.ci-actions {
  display: flex;
  flex-wrap: wrap; // 窄屏楼中楼里放不下时整组折行，而不是把按钮文字挤成两行
  align-items: center;
  gap: 2px;
  margin-top: 2px;
}

.ci-btn {
  white-space: nowrap;
  flex-shrink: 0;
  border: none;
  background: transparent;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 13px;
  color: var(--bb-text-tertiary);
  padding: 2px 8px;
  border-radius: 4px;
  transition: color 0.15s, background-color 0.15s;

  &:hover {
    color: var(--bb-primary);
    background: var(--bb-bg-page);
  }

  &.active {
    color: var(--bb-primary);
    font-weight: 600;
  }

  &.danger:hover {
    color: #f56c6c;
  }
}

.ci-time {
  margin-left: auto;
  color: var(--bb-text-tertiary);
  font-size: 12px;
}
</style>
