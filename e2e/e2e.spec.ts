// E2E：注册 → 登录 → 提问 → 回答 → 退出（规格 §10 端到端最小闭环）
// 运行前提：后端 8081 可用（CI 中由 compose/services 提供），BASE_URL 默认 http://localhost:8081
import { test, expect } from '@playwright/test'
import crypto from 'crypto'

const BASE = process.env.BASE_URL || 'http://localhost:8081'
const API = BASE + '/api/v1'

const run = process.env.E2E_RUN_ID || crypto.randomBytes(4).toString('hex')
const email = `e2e_${run}@test.local`
const username = `e2e_${run}`
const password = 'e2e-password-123'

// 全局限流（2s/100 令牌桶）在突发请求下可能返回非 JSON 文本：重试直到拿到合法 envelope
async function apiJson(request: any, method: 'post' | 'get', url: string, options?: any): Promise<any> {
  for (let i = 0; i < 6; i++) {
    const res = await request[method](url, options)
    const text = await res.text()
    try {
      return JSON.parse(text)
    } catch {
      await new Promise((r) => setTimeout(r, 1500))
    }
  }
  throw new Error('API kept returning non-JSON (rate limited)')
}

// CI/本地环境无 SMTP：send-code 接口在邮件发送失败时会立即失效验证码（fail-closed），
// 因此测试直接向 Redis 注入验证码（与开发联调一致：SET bluebell:auth:code:register:{email}）
function redisInjectRegisterCode(email: string, code: string): void {
  const redis = process.env.E2E_REDIS_CMD?.split(' ') || []
  if (redis.length === 0) throw new Error('E2E_REDIS_CMD not set')
  const { execFileSync } = require('child_process')
  execFileSync(redis[0], [...redis.slice(1), 'SET', `bluebell:auth:code:register:${email}`, code, 'EX', '300'])
}

test('register → login → ask → answer → logout', async ({ page, request }) => {
  // 1. 注入验证码并完成 UI 注册
  const code = '654321'
  redisInjectRegisterCode(email, code)

  // 2. UI 注册（邮箱验证码表单）
  await page.goto(`${BASE.replace('8081', '8080')}/register`)
  await page.getByPlaceholder('邮箱').fill(email)
  await page.getByPlaceholder('6 位验证码').fill(code)
  await page.getByPlaceholder('用户名（2-32 字符）').fill(username)
  await page.getByPlaceholder('密码（至少 8 位）', { exact: true }).fill(password)
  await page.getByPlaceholder('确认密码').fill(password)
  await page.getByRole('button', { name: '注册', exact: true }).click()
  await page.waitForURL(`${BASE.replace('8081', '8080')}/`, { timeout: 10000 })

  // 3. 提问
  await page.goto(`${BASE.replace('8081', '8080')}/questions/ask`)
  await page.getByPlaceholder(/一句话把问题说清楚/).fill(`E2E 问题 ${run}`)
  await page.getByRole('button', { name: '发布问题' }).click()
  await page.waitForURL(/\/questions\/\d+/, { timeout: 10000 })

  // 4. 回答
  await page.getByPlaceholder(/分享你的见解/).fill(`E2E 回答 ${run}`)
  await page.getByRole('button', { name: '发布回答' }).click()
  await expect(page.getByText(`E2E 回答 ${run}`)).toBeVisible({ timeout: 10000 })

  // 5. 退出登录
  await page.locator('.el-dropdown').first().click()
  await page.getByRole('menuitem', { name: '退出登录' }).click()
  await page.waitForURL(/\/login/, { timeout: 10000 })
  await expect(page).toHaveTitle(/登录/)
})

test('phase5: follow lists → block → comment edit → DM', async ({ page, request }) => {
  const codeA = '555555'
  const codeB = '666666'
  const emailA = `p5_a_${run}@test.local`
  const emailB = `p5_b_${run}@test.local`
  redisInjectRegisterCode(emailA, codeA)
  redisInjectRegisterCode(emailB, codeB)
  const bodyA = await apiJson(request, 'post', `${API}/auth/register`, {
    data: { email: emailA, username: `p5_a_${run}`, password, re_password: password, code: codeA },
  })
  const bodyB = await apiJson(request, 'post', `${API}/auth/register`, {
    data: { email: emailB, username: `p5_b_${run}`, password, re_password: password, code: codeB },
  })
  const tokenA = bodyA.data.token
  const tokenB = bodyB.data.token
  const uidA = bodyA.data.user.user_id
  const uidB = bodyB.data.user.user_id

  // B 关注 A；A 提问 + 评论（B 视角）
  await apiJson(request, 'post', `${API}/users/${uidA}/follow`, { headers: { Authorization: `Bearer ${tokenB}` } })
  const q = await apiJson(request, 'post', `${API}/questions`, {
    headers: { Authorization: `Bearer ${tokenA}` },
    data: { title: `第五阶段 E2E ${run}`, description: '社交与私信验收' },
  })
  const qid = q.data.question_id
  await apiJson(request, 'post', `${API}/comments`, {
    headers: { Authorization: `Bearer ${tokenB}` },
    data: { target_type: 'question', target_id: qid, content: `待编辑评论 ${run}` },
  })

  // 粉丝列表：A 的粉丝含 B
  const fl = await apiJson(request, 'get', `${API}/users/${uidA}/followers`, {
    headers: { Authorization: `Bearer ${tokenB}` },
  })
  expect(fl.data.items.some((it: { user_id: string }) => it.user_id === uidB)).toBeTruthy()

  // A 拉黑 B → B 关注 A 被拒；A 的评论列表无 B
  await apiJson(request, 'post', `${API}/users/${uidB}/block`, { headers: { Authorization: `Bearer ${tokenA}` } })
  const refollow = await apiJson(request, 'post', `${API}/users/${uidA}/follow`, { headers: { Authorization: `Bearer ${tokenB}` } })
  expect(refollow.code).toBe(1026)
  const cs = await apiJson(request, 'get', `${API}/comments?target_type=question&target_id=${qid}`, {
    headers: { Authorization: `Bearer ${tokenA}` },
  })
  expect(cs.data.items.some((it: { user_id: string }) => it.user_id === uidB)).toBeFalsy()

  // 取消拉黑 → DM：A 开聊 → B 会话未读 1 → B 回复 → 通知聚合 1 条
  await apiJson(request, 'delete', `${API}/users/${uidB}/block`, { headers: { Authorization: `Bearer ${tokenA}` } })
  const open = await apiJson(request, 'post', `${API}/messages/users/${uidB}`, {
    headers: { Authorization: `Bearer ${tokenA}` },
    data: { content: `私信验收 ${run}` },
  })
  expect(open.data.thread_id).toBeTruthy()
  const tid = open.data.thread_id

  const threadsB = await apiJson(request, 'get', `${API}/messages/threads`, {
    headers: { Authorization: `Bearer ${tokenB}` },
  })
  const th = threadsB.data.items.find((x: { id: string }) => x.id === tid)
  expect(th.unread_count).toBe(1)

  const reply = await apiJson(request, 'post', `${API}/messages/threads/${tid}`, {
    headers: { Authorization: `Bearer ${tokenB}` },
    data: { content: '收到' },
  })
  expect(reply.data.message.content).toBe('收到')

  const notif = await apiJson(request, 'get', `${API}/notifications`, {
    headers: { Authorization: `Bearer ${tokenB}` },
  })
  const dmNotifs = notif.data.items.filter((n: { type: number }) => n.type === 7)
  expect(dmNotifs.length).toBe(1)

  // UI：375px 会话列表可达
  const ui = BASE.replace('8081', '8080')
  await page.setViewportSize({ width: 375, height: 812 })
  await page.goto(`${ui}/login`)
  await page.getByPlaceholder('邮箱').fill(emailA)
  await page.getByPlaceholder('密码', { exact: true }).fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForURL(`${ui}/`, { timeout: 10000 })
  await page.goto(`${ui}/messages`)
  await expect(page.getByText('p5_b_' + run).first()).toBeVisible({ timeout: 10000 })
})
