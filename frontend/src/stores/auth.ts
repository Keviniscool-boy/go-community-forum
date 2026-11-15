import { defineStore } from 'pinia'
import { loginByUsername, signupByUsername } from '@/api/auth'
import type { UserInfo } from '@/types/api'

const TOKEN_KEY = 'bb_token'
const USER_KEY = 'bb_user'

// 登录态 store：token 与当前用户持久化到 localStorage，刷新页面自动恢复
export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem(TOKEN_KEY) || '',
    user: JSON.parse(localStorage.getItem(USER_KEY) || 'null') as UserInfo | null,
  }),
  getters: {
    isLoggedIn: (state) => !!state.token,
  },
  actions: {
    setLogin(token: string, user?: UserInfo | null) {
      this.token = token
      if (user) this.user = user
      localStorage.setItem(TOKEN_KEY, token)
      if (this.user) localStorage.setItem(USER_KEY, JSON.stringify(this.user))
    },
    setUser(user: UserInfo) {
      this.user = user
      localStorage.setItem(USER_KEY, JSON.stringify(user))
    },
    clear() {
      this.token = ''
      this.user = null
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(USER_KEY)
    },
    async login(username: string, password: string) {
      const { token } = await loginByUsername(username, password)
      this.setLogin(token)
    },
    async signup(username: string, password: string, rePassword: string) {
      await signupByUsername(username, password, rePassword)
    },
    logout() {
      this.clear()
    },
  },
})
