import http from './http'
import type { UserInfo } from '@/types/api'

// 用户名密码登录（旧接口，迁移到邮箱登录前暂时使用）
export function loginByUsername(username: string, password: string) {
  return http.post<{ token: string }>('login', { username, password })
}

// 用户名密码注册（旧接口）
export function signupByUsername(username: string, password: string, rePassword: string) {
  return http.post<null>('signup', { username, password, re_password: rePassword })
}

// 获取当前登录用户信息
export function fetchMe() {
  return http.get<UserInfo>('me')
}
