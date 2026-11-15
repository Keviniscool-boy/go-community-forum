<script setup lang="ts">
// 首页：Feed 列表（M1 调用旧 /posts2 接口）
import { onMounted } from 'vue'
import { useFeedStore, type FeedTab } from '@/stores/feed'
import FeedCard from '@/components/FeedCard.vue'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'

const feed = useFeedStore()

const tabs: { key: FeedTab; label: string }[] = [
  { key: 'recommend', label: '推荐' },
  { key: 'latest', label: '最新' },
  { key: 'hot', label: '热榜' },
]

onMounted(() => {
  if (feed.list.length === 0) feed.load(true)
})

function onLoadMore() {
  feed.load()
}
</script>

<template>
  <div class="home-view">
    <div class="feed-tabs bb-card">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="tab-item"
        :class="{ active: feed.tab === tab.key }"
        @click="feed.switchTab(tab.key)"
      >
        {{ tab.label }}
      </button>
    </div>

    <template v-if="feed.loading && feed.list.length === 0">
      <div class="bb-card"><LoadingState /></div>
    </template>
    <div v-else-if="feed.error && feed.list.length === 0" class="bb-card">
      <ErrorState :text="feed.error" @retry="feed.load(true)" />
    </div>
    <div v-else-if="feed.list.length === 0" class="bb-card">
      <EmptyState text="还没有内容，快去发布第一条帖子吧" />
    </div>
    <template v-else>
      <FeedCard v-for="post in feed.list" :key="post.id" :post="post" />
      <div class="load-more">
        <el-button
          v-if="!feed.finished"
          :loading="feed.loading"
          plain
          style="width: 100%"
          @click="onLoadMore"
        >
          加载更多
        </el-button>
        <div v-else class="load-more-end">没有更多内容了</div>
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.feed-tabs {
  display: flex;
  padding: 4px;
  margin-bottom: 12px;
}

.tab-item {
  flex: 1;
  padding: 10px 0;
  border: none;
  background: transparent;
  font-size: 15px;
  color: var(--bb-text-secondary);
  cursor: pointer;
  border-radius: 6px;
  transition: all 0.15s;

  &:hover {
    background: var(--bb-bg-page);
  }

  &.active {
    color: var(--bb-primary);
    font-weight: 600;
    background: #e8f3ff;
  }
}

.load-more {
  margin-bottom: 24px;
}

.load-more-end {
  text-align: center;
  color: var(--bb-text-tertiary);
  font-size: 13px;
  padding: 8px 0;
}
</style>
