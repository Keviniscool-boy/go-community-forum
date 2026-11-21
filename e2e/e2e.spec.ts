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

test('resolution loop → solved badge → search → my content', async ({ page, request }) => {
  // 双用户 API 造数据：A 提问 → B 回答 → A 采纳 → UI 验证已解决标识/搜索标注/我的内容
  const codeA = '111111'
  const codeB = '222222'
  const emailA = `e2e_a_${run}@test.local`
  const emailB = `e2e_b_${run}@test.local`
  redisInjectRegisterCode(emailA, codeA)
  redisInjectRegisterCode(emailB, codeB)

  const regA = await request.post(`${API}/auth/register`, {
    data: { email: emailA, username: `e2e_a_${run}`, password, re_password: password, code: codeA },
  })
  const regB = await request.post(`${API}/auth/register`, {
    data: { email: emailB, username: `e2e_b_${run}`, password, re_password: password, code: codeB },
  })
  expect(regA.ok()).toBeTruthy()
  expect(regB.ok()).toBeTruthy()
  const tokenA = (await regA.json()).data.token
  const tokenB = (await regB.json()).data.token
  expect(tokenA).toBeTruthy()
  expect(tokenB).toBeTruthy()

  // A 提问、B 回答
  const q = await request.post(`${API}/questions`, {
    headers: { Authorization: `Bearer ${tokenA}` },
    data: { title: `解决闭环 E2E ${run}`, description: '验收采纳后状态与搜索标注' },
  })
  expect(q.ok()).toBeTruthy()
  const qid = (await q.json()).data.question_id

  const ans = await request.post(`${API}/questions/${qid}/answers`, {
    headers: { Authorization: `Bearer ${tokenB}` },
    data: { content: `解决闭环回答 ${run}` },
  })
  expect(ans.ok()).toBeTruthy()
  const aid = (await ans.json()).data.answer_id

  // A 采纳
  const acc = await request.post(`${API}/questions/${qid}/answers/${aid}/accept`, {
    headers: { Authorization: `Bearer ${tokenA}` },
  })
  expect(acc.ok()).toBeTruthy()
  const accBody = (await acc.json()).data
  expect(accBody.status).toBe('solved')

  // 游客 UI：问题页显示已解决 + 已采纳标识
  const ui = BASE.replace('8081', '8080')
  await page.goto(`${ui}/questions/${qid}`)
  await expect(page.getByText('已解决', { exact: true })).toBeVisible({ timeout: 10000 })
  await expect(page.getByText('✓ 已采纳')).toBeVisible()

  // 游客搜索：结果带已解决标注
  await page.goto(`${ui}/search?q=${encodeURIComponent(`解决闭环 E2E ${run}`)}`)
  await expect(page.getByText('已解决').first()).toBeVisible({ timeout: 10000 })

  // 作者 UI 登录后查看我的内容（问题 Tab 命中且状态公开）
  await page.goto(`${ui}/login`)
  await page.getByPlaceholder('邮箱').fill(emailA)
  await page.getByPlaceholder('密码', { exact: true }).fill(password)
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForURL(`${ui}/`, { timeout: 10000 })
  await page.goto(`${ui}/me/content?type=question`)
  await expect(page.getByText(`解决闭环 E2E ${run}`)).toBeVisible({ timeout: 10000 })
  await expect(page.getByText('已解决', { exact: true })).toBeVisible()
})
