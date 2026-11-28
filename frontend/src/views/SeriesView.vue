<script setup lang="ts">
// 系列详情页：目录、上下篇、成员管理（所有者）
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { fetchSeries, type Series, type SeriesItem } from '@/api/collab'
import { listMembers, type ColumnMember } from '@/api/collab'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'
import ErrorState from '@/components/ErrorState.vue'
import EmptyState from '@/components/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const seriesId = route.params.id as string
const series = ref<Series | null>(null)
const items = ref<SeriesItem[]>([])
const members = ref<ColumnMember[]>([])
const loading = ref(true)
const error = ref('')
const showMembers = ref(false)

const isOwner = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await fetchSeries(seriesId)
    series.value = res.series
    items.value = res.items
    // 判断是否所有者（拉成员列表成功即成员）
    try {
      members.value = (await listMembers(series.value.column_id)) || []
      const me = members.value.find((m) => m.user_id === auth.user?.user_id)
      isOwner.value = me?.role === 'owner'
      showMembers.value = !!me
    } catch {
      showMembers.value = false
    }
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

// 上下篇导航（仅可用文章）
const availableItems = computed(() => items.value.filter((i) => i.available))
const currentIndex = computed(() =>
  availableItems.value.findIndex((i) => route.query.article === i.article_id),
)

onMounted(load)
</script>

<template>
  <div class="series-view">
    <div v-if="loading" class="bb-card"><LoadingState /></div>
    <div v-else-if="error" class="bb-card"><ErrorState :text="error" @retry="load" /></div>
    <template v-else-if="series">
      <div class="bb-card series-header">
        <h1 class="series-title">{{ series.title }}</h1>
        <p class="series-desc">{{ series.description || '暂无简介' }}</p>
        <div class="series-meta">
          <router-link :to="`/columns/${series.column_id}`">所属专栏</router-link>
          <span>{{ availableItems.length }} 篇文章</span>
          <el-button v-if="showMembers" size="small" plain @click="showMembers = !showMembers">
            成员管理
          </el-button>
        </div>
      </div>

      <!-- 成员管理（仅成员可见） -->
      <div v-if="showMembers && members.length > 0" class="bb-card members-card">
        <h4>成员</h4>
        <div v-for="m in members" :key="m.user_id" class="member-row">
          <span>{{ m.role === 'owner' ? '所有者' : m.role === 'editor' ? '编辑' : '审阅者' }}</span>
          <router-link :to="`/users/${m.user_id}`">用户 {{ m.user_id.slice(-6) }}</router-link>
        </div>
      </div>

      <div class="bb-card catalog-card">
        <h4>目录</h4>
        <div v-if="availableItems.length === 0"><EmptyState text="目录为空" /></div>
        <div v-for="(item, i) in availableItems" :key="item.article_id" class="catalog-item">
          <span class="catalog-idx">{{ i + 1 }}</span>
          <router-link
            class="catalog-link"
            :to="{ path: `/articles/${item.article_id}`, query: { series: seriesId } }"
          >
            {{ item.title }}
          </router-link>
          <!-- 上下篇导航 -->
          <span class="nav-arrows">
            <el-button
              v-if="i > 0" text size="small"
              @click="router.push({ path: `/articles/${availableItems[i - 1].article_id}`, query: { series: seriesId } })"
            >上一篇</el-button>
            <el-button
              v-if="i < availableItems.length - 1" text size="small"
              @click="router.push({ path: `/articles/${availableItems[i + 1].article_id}`, query: { series: seriesId } })"
            >下一篇</el-button>
          </span>
        </div>
      </div>

      <!-- 当前阅读位置提示 -->
      <div v-if="currentIndex >= 0" class="bb-card position-tip">
        当前阅读第 {{ currentIndex + 1 }} / {{ availableItems.length }} 篇
      </div>
    </template>
  </div>
</template>

<style scoped lang="less">
.series-header { padding: 24px 28px; margin-bottom: 12px; }
.series-title { margin: 0; font-size: 22px; }
.series-desc { margin: 8px 0; color: var(--bb-text-secondary); }
.series-meta {
  display: flex; gap: 12px; align-items: center;
  color: var(--bb-text-tertiary); font-size: 13px;
  a { color: var(--bb-primary); }
}
.members-card { padding: 16px 20px; margin-bottom: 12px; h4 { margin: 0 0 8px; } }
.member-row {
  display: flex; gap: 12px; padding: 6px 0; font-size: 14px;
  border-bottom: 1px solid var(--bb-border);
  &:last-child { border-bottom: none; }
}
.catalog-card { padding: 16px 20px; h4 { margin: 0 0 10px; } }
.catalog-item {
  display: flex; align-items: center; gap: 12px; padding: 10px 4px;
  border-bottom: 1px solid var(--bb-border);
}
.catalog-idx {
  width: 24px; height: 24px; border-radius: 50%;
  background: #e8f3ff; color: var(--bb-primary);
  display: flex; align-items: center; justify-content: center;
  font-size: 13px; font-weight: 600; flex-shrink: 0;
}
.catalog-link { flex: 1; color: var(--bb-text-main); font-weight: 500;
  &:hover { color: var(--bb-primary); } }
.nav-arrows { display: flex; gap: 4px; flex-shrink: 0; }
.position-tip {
  padding: 10px 16px; color: var(--bb-text-secondary); font-size: 13px;
  position: sticky; bottom: 0;
}
</style>
