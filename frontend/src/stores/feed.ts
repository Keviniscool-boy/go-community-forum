import { defineStore } from 'pinia'
import { fetchFeed, type FeedType, type FeedItem, type PageResult } from '@/api/feed'

export type { FeedType }

// Feed store：当前 Tab、条目列表、cursor 游标分页与加载状态
export const useFeedStore = defineStore('feed', {
  state: () => ({
    tab: 'recommend' as FeedType,
    list: [] as FeedItem[],
    cursor: '', // opaque 游标，后端生成，前端只透传
    loading: false,
    finished: false, // 没有更多数据
    error: '',
  }),
  actions: {
    async load(reset = false) {
      if (this.loading) return
      if (!reset && this.finished) return
      this.loading = true
      this.error = ''
      try {
        if (reset) {
          this.cursor = ''
          this.finished = false
        }
        const res: PageResult<FeedItem> = await fetchFeed(this.tab, this.cursor, 20)
        if (reset) this.list = []
        this.list = [...this.list, ...(res.items || [])]
        this.cursor = res.next_cursor
        this.finished = !res.has_more
      } catch (e) {
        this.error = e instanceof Error ? e.message : '加载失败'
      } finally {
        this.loading = false
      }
    },
    switchTab(tab: FeedType) {
      if (this.tab === tab) return
      this.tab = tab
      this.load(true)
    },
  },
})
