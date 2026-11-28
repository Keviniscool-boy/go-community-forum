import http from './http'

// 媒体上传（第五阶段 F5 头像走同一管线）
export function uploadImage(file: Blob, purpose: 'avatar' | 'content' = 'content') {
  const form = new FormData()
  form.append('file', file, 'image.jpg')
  return http.post<{ asset_id: string; url: string; width: number; height: number }>(
    `media/upload?purpose=${purpose}`,
    form,
    { headers: { 'Content-Type': 'multipart/form-data' } },
  )
}
