// 后端接口通用类型定义
// 注意：所有雪花 ID 在 JSON 中按字符串返回，避免 JS 精度丢失

export interface ApiResponse<T = unknown> {
  code: number
  msg: string
  data: T
}

export interface CommunityDetail {
  id: string
  name: string
  introduction: string
}

export interface PostDetail {
  id: string
  title: string
  content: string
  author_id: string
  author_name: string
  community_id: string
  status: number
  create_time: string
  name: string // 所属社区名（CommunityDetail 嵌入）
  introduction: string
}

export interface UserInfo {
  user_id: string
  username: string
  email?: string
  avatar_url?: string
  bio?: string
  role?: string
}

export interface LoginResult {
  token: string
  user?: UserInfo
}

// 问题详情（api/question.ts 定义主体，此处放引用用到的轻量结构）
export interface QuestionBrief {
  id: string
  title: string
}
