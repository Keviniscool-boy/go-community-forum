import axios, { AxiosError, type AxiosInstance, type InternalAxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'
import type { ApiResponse } from '@/types/api'
import { useAuthStore } from '@/stores/auth'

// 需要清除登录态并跳转登录页的业务错误码（对齐 controller/code.go）
const AUTH_FAIL_CODES = [1009, 1010] // CodeInvalidAuth / CodeNeedLogin

const http: AxiosInstance = axios.create({
  baseURL: '/api/v1/',
  timeout: 15000,
})

// 请求拦截器：统一携带 token
http.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const auth = useAuthStore()
  if (auth.token) {
    config.headers.Authorization = `Bearer ${auth.token}`
  }
  return config
})

// 响应拦截器：解包业务数据，统一处理错误提示与登录态失效
http.interceptors.response.use(
  (response) => {
    const res = response.data as ApiResponse
    // 登录态失效：清除本地状态并跳转登录页
    if (AUTH_FAIL_CODES.includes(res.code)) {
      const auth = useAuthStore()
      auth.clear()
      ElMessage.warning('登录状态已失效，请重新登录')
      if (window.location.pathname !== '/login') {
        window.location.href = '/login'
      }
      return Promise.reject(new Error(res.msg || '请先登录'))
    }
    // 业务错误：抛出给调用方处理，调用方可静默自行提示
    if (res.code !== 1000) {
      return Promise.reject(new ApiError(res.code, res.msg || '请求失败'))
    }
    // 已解包业务数据，此处强转以满足 axios 拦截器签名，真实类型见 UnwrappedHttp
    return res.data as unknown as typeof response
  },
  (error: AxiosError) => {
    ElMessage.error('网络异常，请稍后重试')
    return Promise.reject(error)
  },
)

// 携带后端业务错误码的错误类型，调用方可据此做分支处理
export class ApiError extends Error {
  code: number
  constructor(code: number, message: string) {
    super(message)
    this.code = code
  }
}

// 响应拦截器已解包 Response.data，对外暴露按业务数据类型声明的接口
interface UnwrappedHttp {
  get<T = unknown>(url: string, config?: Record<string, unknown>): Promise<T>
  post<T = unknown>(url: string, data?: unknown, config?: Record<string, unknown>): Promise<T>
  put<T = unknown>(url: string, data?: unknown, config?: Record<string, unknown>): Promise<T>
  patch<T = unknown>(url: string, data?: unknown, config?: Record<string, unknown>): Promise<T>
  delete<T = unknown>(url: string, config?: Record<string, unknown>): Promise<T>
}

export default http as unknown as UnwrappedHttp
