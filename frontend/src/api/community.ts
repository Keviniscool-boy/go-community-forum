import http from './http'
import type { CommunityDetail } from '@/types/api'

// 社区列表（M3 起逐步替换为话题）
export function fetchCommunities() {
  return http.get<CommunityDetail[]>('community')
}
