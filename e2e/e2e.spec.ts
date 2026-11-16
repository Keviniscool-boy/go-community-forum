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

async function apiGetCode(email: string): Promise<string> {
  // CI/本地环境无 SMTP：直接从 Redis 读验证码（与开发联调一致的约定）
  const redis = process.env.E2E_REDIS_CMD?.split(' ') || []
  if (redis.length === 0) throw new Error('E2E_REDIS_CMD not set')
  const { execFileSync } = require('child_process')
  return execFileSync(redis[0], [...redis.slice(1), 'GET', `bluebell:auth:code:register:${email}`])
    .toString()
    .trim()
}

test('register → login → ask → answer → logout', async ({ page, request }) => {
  // 1. 通过 API 发码并取码（Redis 注入，与开发联调一致）
  await request.post(`${API}/auth/send-code`, {
    data: { email, purpose: 'register' },
  })
  const code = await apiGetCode(email)
  expect(code).toMatch(/^\d{6}$/)

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
