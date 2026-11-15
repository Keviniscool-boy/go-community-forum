import { defineStore } from 'pinia'
import { fetchPostList, type PostListParams } from '@/api/post'
import type { PostDetail } from '@/types/api'

export type FeedTab = 'recommend' | 'latest' | 'hot'

// Feed store：管理当前 Tab、帖子列表和分页状态
// M1 阶段使用旧 /posts2 的 page/size 分页；M4 上线后切换为 /feed/* 游标分页
export const useFeedStore = defineStore('feed', {
  state: () => ({
    tab: 'recommend' as FeedTab,
    list: [] as PostDetail[],
    page: 1,
    size: 20,
    loading: false,
    finished: false, // 没有更多数据
    error: '' as string, // 非空表示加载失败
  }),
  actions: {
    tabToOrder(tab: FeedTab): PostListParams['order'] {
      if (tab === 'latest') return 'time'
      if (tab === 'hot') return 'score'
      return 'time' // 旧接口无推荐排序，M1 暂用时间序替代
    },
    async load(reset = false) {
      if (this.loading) return
      if (!reset && this.finished) return
      this.loading = true
      this.error = ''
      try {
        if (reset) {
          this.page = 1
          this.finished = false
        }
        const data = await fetchPostList({
          page: this.page,
          size: this.size,
          order: this.tabToOrder(this.tab),
        })
        if (reset) this.list = []
        this.list = [...this.list, ...(data || [])]
        // 返回条数不足一页视为已到底
        if (!data || data.length < this.size) this.finished = true
        this.page += 1
      } catch (e) {
        this.error = e instanceof Error ? e.message : '加载失败'
      } finally {
        this.loading = false
      }
    },
    switchTab(tab: FeedTab) {
      if (this.tab === tab) return
      this.tab = tab
      this.load(true)
    },
  },
})
