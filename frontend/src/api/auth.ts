import http from './http'
import type { UserInfo } from '@/types/api'

export interface AuthResult {
  token: string
  user: UserInfo
}

// 发送邮箱验证码
export function sendAuthCode(email: string, purpose: 'register' | 'login' | 'reset_password') {
  return http.post<null>('auth/send-code', { email, purpose })
}

// 邮箱验证码注册
export function registerByEmail(data: {
  email: string
  code: string
  username: string
  password: string
  re_password: string
}) {
  return http.post<AuthResult>('auth/register', data)
}

// 邮箱密码登录
export function loginByEmail(email: string, password: string) {
  return http.post<AuthResult>('auth/login', { email, password })
}

// 用户名密码登录（旧接口，迁移期保留）
export function loginByUsername(username: string, password: string) {
  return http.post<{ token: string }>('login', { username, password })
}

// 用户名密码注册（旧接口，迁移期保留）
export function signupByUsername(username: string, password: string, rePassword: string) {
  return http.post<null>('signup', { username, password, re_password: rePassword })
}

// 获取当前登录用户信息
export function fetchMe() {
  return http.get<UserInfo>('auth/me')
}
