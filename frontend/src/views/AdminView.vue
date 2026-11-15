<script setup lang="ts">
// 管理后台：指标总览、举报队列、内容审核、用户禁用、审计日志
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  metricsOverview, listReports, resolveReport, moderateContent, setUserEnabled, listAuditLogs,
  type ReportDetail, type AuditLogItem,
} from '@/api/productivity'
import LoadingState from '@/components/LoadingState.vue'
import EmptyState from '@/components/EmptyState.vue'
import ErrorState from '@/components/ErrorState.vue'

const tab = ref<'metrics' | 'reports' | 'content' | 'users' | 'audit'>('metrics')

const metrics = ref<Record<string, number>>({})
const loading = ref(false)
const error = ref('')

const reports = ref<ReportDetail[]>([])
const reportStatus = ref('pending')
const reportsLoading = ref(false)

const auditLogs = ref<AuditLogItem[]>([])
const auditLoading = ref(false)

// 内容审核输入
const contentType = ref<'question' | 'answer'>('question')
const contentId = ref('')
const contentAction = ref<'approve' | 'reject' | 'quarantine'>('approve')
// 用户管理输入
const userIdInput = ref('')
const userEnable = ref(false)

async function loadMetrics() {
  try {
    metrics.value = (await metricsOverview()) || {}
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  }
}

async function loadReports() {
  reportsLoading.value = true
  try {
    const res = await listReports(reportStatus.value)
    reports.value = res.items
  } finally {
    reportsLoading.value = false
  }
}

async function loadAudit() {
  auditLoading.value = true
  try {
    const res = await listAuditLogs()
    auditLogs.value = res.items
  } finally {
    auditLoading.value = false
  }
}

async function onResolve(r: ReportDetail, decision: 'resolve' | 'dismiss') {
  try {
    await ElMessageBox.confirm(
      decision === 'resolve' ? '举报成立：内容将被隔离下架。' : '驳回该举报。',
      '处理举报', { type: 'warning' },
    )
  } catch { return }
  try {
    await resolveReport(r.id, decision, '')
    ElMessage.success('已处理')
    loadReports()
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message)
  }
}

async function onModerate() {
  try {
    await moderateContent(contentType.value, contentId.value, contentAction.value)
    ElMessage.success('已执行')
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message)
  }
}

async function onSetUser() {
  try {
    await ElMessageBox.confirm(
      userEnable.value ? '启用该用户？' : '禁用该用户（其全部会话立即失效）？',
      '用户管理', { type: 'warning' },
    )
  } catch { return }
  try {
    await setUserEnabled(userIdInput.value, userEnable.value)
    ElMessage.success('已执行')
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message)
  }
}

const metricLabels: Record<string, string> = {
  users: '用户总数', questions: '公开问题', questions_pending: '待审核问题',
  answers: '公开回答', reports_pending: '待处理举报', users_disabled: '禁用用户',
  audit_events: '审计事件',
}

function switchTab(t: typeof tab.value) {
  tab.value = t
  error.value = ''
  if (t === 'metrics') loadMetrics()
  if (t === 'reports') loadReports()
  if (t === 'audit') loadAudit()
}

onMounted(loadMetrics)
</script>

<template>
  <div class="admin-view">
    <div class="bb-card admin-tabs">
      <button class="tab-btn" :class="{ active: tab === 'metrics' }" @click="switchTab('metrics')">总览</button>
      <button class="tab-btn" :class="{ active: tab === 'reports' }" @click="switchTab('reports')">举报队列</button>
      <button class="tab-btn" :class="{ active: tab === 'content' }" @click="switchTab('content')">内容审核</button>
      <button class="tab-btn" :class="{ active: tab === 'users' }" @click="switchTab('users')">用户管理</button>
      <button class="tab-btn" :class="{ active: tab === 'audit' }" @click="switchTab('audit')">审计日志</button>
    </div>

    <div v-if="error" class="bb-card"><ErrorState :text="error" /></div>

    <!-- 总览 -->
    <template v-else-if="tab === 'metrics'">
      <div v-if="loading" class="bb-card"><LoadingState /></div>
      <div v-else class="metric-grid">
        <div v-for="(v, k) in metrics" :key="k" class="bb-card metric-card">
          <div class="metric-value">{{ v }}</div>
          <div class="metric-label">{{ metricLabels[k] || k }}</div>
        </div>
      </div>
    </template>

    <!-- 举报队列 -->
    <template v-else-if="tab === 'reports'">
      <div class="bb-card reports-header">
        <el-radio-group v-model="reportStatus" size="small" @change="loadReports">
          <el-radio-button value="pending">待处理</el-radio-button>
          <el-radio-button value="resolved">已成立</el-radio-button>
          <el-radio-button value="dismissed">已驳回</el-radio-button>
        </el-radio-group>
      </div>
      <div v-if="reportsLoading" class="bb-card"><LoadingState /></div>
      <div v-else-if="reports.length === 0" class="bb-card"><EmptyState text="队列为空" /></div>
      <article v-for="r in reports" :key="r.id" class="bb-card report-card">
        <div class="report-line">
          <strong>{{ r.reporter_name }}</strong>
          <span class="report-text">举报了{{ ['问题', '回答', '评论', '用户'][r.target_type - 1] || '内容' }} #{{ r.target_id }}</span>
          <el-tag size="small" effect="plain">{{ r.reason_code }}</el-tag>
        </div>
        <p v-if="r.description" class="report-desc">{{ r.description }}</p>
        <div v-if="r.status === 0" class="report-actions">
          <el-button size="small" type="danger" @click="onResolve(r, 'resolve')">举报成立（下架）</el-button>
          <el-button size="small" plain @click="onResolve(r, 'dismiss')">驳回</el-button>
        </div>
        <div v-else class="report-actions">
          <el-tag size="small" :type="r.status === 1 ? 'danger' : 'info'">{{ r.status === 1 ? '已成立' : '已驳回' }}</el-tag>
        </div>
      </article>
    </template>

    <!-- 内容审核 -->
    <template v-else-if="tab === 'content'">
      <div class="bb-card admin-form">
        <h3>内容审核操作</h3>
        <div class="form-row">
          <el-select v-model="contentType" style="width: 140px">
            <el-option label="问题" value="question" />
            <el-option label="回答" value="answer" />
          </el-select>
          <el-input v-model="contentId" placeholder="内容 ID（雪花 ID）" />
          <el-select v-model="contentAction" style="width: 140px">
            <el-option label="通过" value="approve" />
            <el-option label="拒绝" value="reject" />
            <el-option label="隔离" value="quarantine" />
          </el-select>
          <el-button type="primary" @click="onModerate">执行</el-button>
        </div>
        <p class="form-hint">隔离/拒绝的内容将从 Feed、详情与搜索中隐藏，并保留原始记录供审计。</p>
      </div>
    </template>

    <!-- 用户管理 -->
    <template v-else-if="tab === 'users'">
      <div class="bb-card admin-form">
        <h3>用户禁用/启用</h3>
        <div class="form-row">
          <el-input v-model="userIdInput" placeholder="用户 ID（雪花 ID）" />
          <el-switch v-model="userEnable" active-text="启用" inactive-text="禁用" />
          <el-button type="primary" @click="onSetUser">执行</el-button>
        </div>
        <p class="form-hint">禁用后该用户全部登录会话立即失效，其内容从公开入口隐藏。</p>
      </div>
    </template>

    <!-- 审计日志 -->
    <template v-else-if="tab === 'audit'">
      <div v-if="auditLoading" class="bb-card"><LoadingState /></div>
      <div v-else-if="auditLogs.length === 0" class="bb-card"><EmptyState text="暂无审计记录" /></div>
      <article v-for="log in auditLogs" :key="log.id" class="bb-card audit-card">
        <div class="report-line">
          <el-tag size="small" effect="plain">{{ log.action }}</el-tag>
          <span class="report-text">{{ log.target_type }} #{{ log.target_id }}</span>
          <span class="report-text">操作者 {{ log.actor_id }}</span>
        </div>
        <div class="audit-meta">req: {{ log.request_id }} · {{ log.created_at?.slice(0, 19).replace('T', ' ') }}</div>
      </article>
    </template>
  </div>
</template>

<style scoped lang="less">
.admin-tabs { display: flex; padding: 6px; margin-bottom: 12px; flex-wrap: wrap; }
.tab-btn {
  flex: 1; padding: 10px 0; border: none; background: transparent;
  font-size: 14px; color: var(--bb-text-secondary); cursor: pointer; border-radius: 6px;
  &.active { color: var(--bb-primary); font-weight: 600; background: #e8f3ff; }
}
.metric-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px; }
.metric-card { padding: 16px; text-align: center; }
.metric-value { font-size: 26px; font-weight: 700; color: var(--bb-primary); }
.metric-label { color: var(--bb-text-secondary); font-size: 13px; margin-top: 4px; }
.reports-header { padding: 10px 16px; margin-bottom: 12px; }
.report-card { padding: 14px 18px; margin-bottom: 10px; }
.report-line { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.report-text { color: var(--bb-text-secondary); font-size: 13px; }
.report-desc { margin: 8px 0; color: var(--bb-text-main); font-size: 13px; }
.report-actions { margin-top: 8px; }
.admin-form { padding: 20px 24px; h3 { margin-top: 0; } }
.form-row { display: flex; gap: 10px; flex-wrap: wrap; }
.form-hint { color: var(--bb-text-tertiary); font-size: 12px; }
.audit-card { padding: 12px 18px; margin-bottom: 8px; }
.audit-meta { color: var(--bb-text-tertiary); font-size: 12px; margin-top: 4px; word-break: break-all; }
</style>
