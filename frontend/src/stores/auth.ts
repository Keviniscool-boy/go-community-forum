import { defineStore } from 'pinia'
import { loginByEmail, registerByEmail, fetchMe, type AuthResult } from '@/api/auth'
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
    // 邮箱密码登录
    async login(email: string, password: string) {
      const res: AuthResult = await loginByEmail(email, password)
      this.setLogin(res.token, res.user)
    },
    // 邮箱验证码注册（注册成功直接登录）
    async register(data: {
      email: string
      code: string
      username: string
      password: string
      re_password: string
    }) {
      const res = await registerByEmail(data)
      this.setLogin(res.token, res.user)
    },
    // 拉取当前用户信息（登录状态恢复时调用）
    async restoreMe() {
      if (!this.token) return
      try {
        const user = await fetchMe()
        this.setUser(user)
      } catch {
        // token 失效时响应拦截器已清除登录态
      }
    },
    logout() {
      this.clear()
    },
  },
})
