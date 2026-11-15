import { defineStore } from 'pinia'
import { fetchUnreadCount, fetchNotifications, markAllRead, type NotificationDetail } from '@/api/notification'
import { useAuthStore } from '@/stores/auth'

// 通知 store：未读数轮询 + 通知列表
export const useNotificationStore = defineStore('notification', {
  state: () => ({
    unreadCount: 0,
    list: [] as NotificationDetail[],
    loading: false,
    finished: false,
    cursor: '',
    timer: null as ReturnType<typeof setInterval> | null,
  }),
  actions: {
    // 拉取未读数（登录状态下每 60 秒轮询）
    async refreshUnread() {
      const auth = useAuthStore()
      if (!auth.isLoggedIn) {
        this.unreadCount = 0
        return
      }
      try {
        const res = await fetchUnreadCount()
        this.unreadCount = res.count
      } catch {
        // 轮询失败静默忽略
      }
    },
    startPolling() {
      this.refreshUnread()
      if (this.timer) clearInterval(this.timer)
      this.timer = setInterval(() => this.refreshUnread(), 60000)
    },
    stopPolling() {
      if (this.timer) {
        clearInterval(this.timer)
        this.timer = null
      }
      this.unreadCount = 0
    },
    async load(reset = false) {
      if (this.loading) return
      if (!reset && this.finished) return
      this.loading = true
      try {
        if (reset) {
          this.cursor = ''
          this.finished = false
        }
        const res = await fetchNotifications(this.cursor, 20)
        if (reset) this.list = []
        this.list = [...this.list, ...res.items]
        this.cursor = res.next_cursor
        this.finished = !res.has_more
      } finally {
        this.loading = false
      }
    },
    async readAll() {
      await markAllRead()
      this.unreadCount = 0
      await this.load(true)
    },
  },
})
