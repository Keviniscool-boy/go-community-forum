<script setup lang="ts">
// Feed 内容卡片：标题、作者/话题/专栏、摘要、互动数据、反馈菜单
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { ChatDotRound, Star, CaretTop, View, MoreFilled } from '@element-plus/icons-vue'
import type { FeedItem } from '@/api/feed'
import { addFeedback, blockUser } from '@/api/personalization'

const props = defineProps<{ item: FeedItem }>()
const emit = defineEmits<{ (e: 'removed', item: FeedItem): void }>()
const removed = ref(false)

// 条目链接：回答/文章跳详情
function itemLink(item: FeedItem): string {
  if (item.item_type === 'article') return `/articles/${item.item_id}`
  return item.item_type === 'answer' ? `/questions/${item.question_id}` : `/questions/${item.item_id}`
}

// 负反馈与屏蔽（失败提示重试；成功后短时撤销窗口）
async function onFeedback(cmd: string) {
  try {
    if (cmd === 'not_interested') {
      await addFeedback('not_interested', 'content', props.item.item_id)
    } else {
      await blockUser(props.item.author.user_id)
    }
    removed.value = true
    emit('removed', props.item)
    setTimeout(() => {
      removed.value = false
    }, 8000)
  } catch {
    ElMessage.error('操作失败，请重试')
  }
}

function undo() {
  removed.value = false
}
</script>

<template>
  <article v-if="!removed" class="feed-card bb-card">
    <router-link class="card-title" :to="itemLink(item)">{{ item.title }}</router-link>
    <div class="card-meta">
      <router-link class="meta-author" :to="`/users/${item.author.user_id}`">{{ item.author.username }}</router-link>
      <span v-if="item.item_type === 'answer'" class="meta-type">回答了问题</span>
      <el-tag v-if="item.topic" size="small" effect="plain" class="meta-topic">{{ item.topic.name }}</el-tag>
      <el-tag v-if="item.column" size="small" effect="plain" class="meta-topic">{{ item.column.name }}</el-tag>
      <span class="meta-time">{{ item.create_time?.slice(0, 10) }}</span>
    </div>
    <p class="card-excerpt">{{ item.excerpt }}</p>
    <div class="card-actions">
      <span v-if="item.recommendation_reason" class="reason-text">{{ item.recommendation_reason }}</span>
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
      <el-dropdown v-if="item.sources" trigger="click" class="feedback-dropdown" @command="onFeedback">
        <span class="action-item"><el-icon><MoreFilled /></el-icon></span>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="not_interested">不感兴趣</el-dropdown-item>
            <el-dropdown-item command="block">屏蔽作者</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
    <div v-if="removed" class="removed-tip">
      已减少此类内容推荐
      <el-button text size="small" type="primary" @click="undo">撤销</el-button>
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

.reason-text {
  margin-right: auto;
  color: var(--bb-text-tertiary);
  font-size: 12px;
}

.feedback-dropdown {
  margin-left: auto;
}

.removed-tip {
  background: #f0f7ff;
  border-radius: 6px;
  padding: 8px 12px;
  margin: 10px 0 0;
  font-size: 13px;
  color: var(--bb-text-secondary);
  display: flex;
  align-items: center;
  gap: 8px;
}

.action-item {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  color: var(--bb-text-tertiary);
}
</style>
