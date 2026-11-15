<script setup lang="ts">
// 首页：Feed 列表（推荐/关注/最新，cursor 分页加载）
import { onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useFeedStore, type FeedType } from '@/stores/feed'
import { useAuthStore } from '@/stores/auth'
import FeedCard from '@/components/FeedCard.vue'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'

const feed = useFeedStore()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const tabs: { key: FeedType; label: string }[] = [
  { key: 'recommend', label: '推荐' },
  { key: 'following', label: '关注' },
  { key: 'latest', label: '最新' },
]

// 深链支持：/hot 进入时直接切到热榜
const isHotPage = computed(() => route.path === '/hot')

onMounted(() => {
  if (isHotPage.value) {
    feed.switchTab('hot')
  }
  if (feed.list.length === 0) feed.load(true)
})

function onLoadMore() {
  feed.load()
}

function onSwitchTab(key: FeedType) {
  if (key === 'following' && !auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  feed.switchTab(key)
}
</script>

<template>
  <div class="home-view">
    <div class="feed-tabs bb-card">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="tab-item"
        :class="{ active: !isHotPage && feed.tab === tab.key }"
        @click="onSwitchTab(tab.key)"
      >
        {{ tab.label }}
      </button>
      <button
        class="tab-item"
        :class="{ active: feed.tab === 'hot' }"
        @click="feed.switchTab('hot')"
      >
        热榜
      </button>
    </div>

    <template v-if="feed.loading && feed.list.length === 0">
      <div class="bb-card"><LoadingState /></div>
    </template>
    <div v-else-if="feed.error && feed.list.length === 0" class="bb-card">
      <ErrorState :text="feed.error" @retry="feed.load(true)" />
    </div>
    <div v-else-if="feed.list.length === 0" class="bb-card">
      <EmptyState :text="feed.tab === 'following' ? '关注一些用户后，这里会展示他们的动态' : '还没有内容，快去发布第一个问题吧'" />
    </div>
    <template v-else>
      <FeedCard v-for="item in feed.list" :key="`${item.item_type}-${item.item_id}`" :item="item" />
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
