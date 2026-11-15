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

// 找回密码第一步：验证码换一次性重置凭据（服务端只存摘要）
export function verifyResetCode(email: string, code: string) {
  return http.post<{ reset_token: string; expires_in: number }>('auth/password/reset/verify', {
    email,
    code,
  })
}

// 找回密码第二步：凭据设置新密码（单次消费，撤销所有旧会话）
export function resetPasswordWithToken(data: {
  reset_token: string
  password: string
  re_password: string
}) {
  return http.post<null>('auth/password/reset', data)
}

// 绑定已验证邮箱（旧账号迁移流程；需登录）
export function bindEmail(email: string, code: string) {
  return http.post<null>('auth/email/bind', { email, code })
}

// 发送绑定验证码（需登录）
export function sendBindCode(email: string) {
  return http.post<null>('auth/send-code/bind', { email, purpose: 'bind_email' })
}

// 退出所有设备（服务端撤销全部 token）
export function logoutAll() {
  return http.post<null>('auth/logout-all')
}

// 更新个人资料（用户名可选更新 + 简介/头像地址）
export function updateProfile(data: { username?: string; bio: string; avatar_url: string }) {
  return http.put<null>('auth/profile', data)
}
