<script setup lang="ts">
// 推荐设置页：个性化开关、兴趣编辑、屏蔽列表、负反馈列表
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getInterests, putInterests, getPreferences, putPreferences,
  listFeedback, removeFeedback, listBlocks, unblockUser,
  type FeedbackItem, type BlockedUser,
} from '@/api/personalization'
import { fetchTopics } from '@/api/notification'
import { fetchAnswerInvitationPreference, setAnswerInvitationPreference } from '@/api/invitation'
import { ApiError } from '@/api/http'
import LoadingState from '@/components/LoadingState.vue'

const loading = ref(true)
const personalization = ref(true)
const invitePref = ref(true)

onMounted(() => {
  fetchAnswerInvitationPreference()
    .then((res) => {
      invitePref.value = res?.enabled ?? true
    })
    .catch(() => {
      // 读取失败保持默认
    })
})

async function toggleInvitePref() {
  try {
    await setAnswerInvitationPreference(invitePref.value)
    ElMessage.success(invitePref.value ? '已开启回答邀请' : '已关闭回答邀请（你不会出现在候选中）')
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
    invitePref.value = !invitePref.value
  }
}
const prefVersion = ref(0)
const myTopics = ref<string[]>([])
const allTopics = ref<Array<{ topic_id: string; name: string }>>([])
const feedback = ref<FeedbackItem[]>([])
const blocks = ref<BlockedUser[]>([])

async function load() {
  loading.value = true
  try {
    const [prefs, topics, fb, bl] = await Promise.all([
      getPreferences(), fetchTopics(50), listFeedback(), listBlocks(),
    ])
    personalization.value = prefs.personalization_enabled
    prefVersion.value = prefs.version
    allTopics.value = topics || []
    feedback.value = fb || []
    blocks.value = bl || []
    const mine = await getInterests()
    myTopics.value = mine.topic_ids || []
  } finally {
    loading.value = false
  }
}

async function togglePersonalization() {
  try {
    const res = await putPreferences(personalization.value, prefVersion.value)
    prefVersion.value = res.version
    ElMessage.success(personalization.value ? '个性化已开启' : '个性化已关闭（推荐将使用公开热度与新鲜度）')
  } catch {
    // 版本冲突：重读设置
    const prefs = await getPreferences()
    personalization.value = prefs.personalization_enabled
    prefVersion.value = prefs.version
    ElMessage.warning('设置已被其他设备更新，已重新读取')
  }
}

async function toggleTopic(topicId: string) {
  const idx = myTopics.value.indexOf(topicId)
  const next = [...myTopics.value]
  if (idx >= 0) next.splice(idx, 1)
  else if (next.length >= 20) return ElMessage.warning('兴趣最多 20 项')
  else next.push(topicId)
  await putInterests(next)
  myTopics.value = next
}

async function removeFb(item: FeedbackItem) {
  await removeFeedback(item.kind, item.target_type, item.target_id)
  feedback.value = feedback.value.filter((f) => f !== item)
  ElMessage.success('已撤销')
}

async function unblock(id: string) {
  await unblockUser(id)
  blocks.value = blocks.value.filter((b) => b.user_id !== id)
  ElMessage.success('已取消屏蔽')
}

onMounted(load)
</script>

<template>
  <div v-if="loading" class="bb-card"><LoadingState /></div>
  <div v-else class="pref-view">
    <div class="bb-card pref-card">
      <h3>推荐个性化</h3>
      <div class="pref-row">
        <div>
          <p class="pref-title">使用个性化推荐</p>
          <p class="pref-desc">关闭后推荐仅基于公开热度与新鲜度，不使用你的浏览与点击历史；关注流不受影响。</p>
        </div>
        <el-switch v-model="personalization" @change="togglePersonalization" />
      </div>
    </div>

    <div class="bb-card pref-card">
      <h3>回答邀请</h3>
      <div class="pref-row">
        <div>
          <p class="pref-title">接收回答邀请</p>
          <p class="pref-desc">关闭后其他用户不能邀请你回答问题，你也不会出现在邀请候选列表中。</p>
        </div>
        <el-switch v-model="invitePref" @change="toggleInvitePref" />
      </div>
    </div>

    <div class="bb-card pref-card">
      <h3>我的兴趣（{{ myTopics.length }}/20）</h3>
      <p class="pref-desc">兴趣仅用于推荐，关注话题请到话题页操作。</p>
      <div class="topic-chips">
        <el-tag
          v-for="t in allTopics" :key="t.topic_id"
          :effect="myTopics.includes(t.topic_id) ? 'dark' : 'plain'"
          class="topic-chip"
          @click="toggleTopic(t.topic_id)"
        >
          {{ t.name }}
        </el-tag>
      </div>
    </div>

    <div class="bb-card pref-card">
      <h3>屏蔽的作者（{{ blocks.length }}）</h3>
      <p class="pref-desc">被屏蔽作者的内容不会出现在推荐、关注与搜索中。</p>
      <div v-if="blocks.length === 0" class="empty-line">没有屏蔽任何作者</div>
      <div v-for="b in blocks" :key="b.user_id" class="block-row">
        <span>{{ b.username }}</span>
        <el-button size="small" text @click="unblock(b.user_id)">取消屏蔽</el-button>
      </div>
    </div>

    <div class="bb-card pref-card">
      <h3>推荐负反馈（{{ feedback.length }}）</h3>
      <p class="pref-desc">"不感兴趣"与"减少此话题"记录，可撤销。</p>
      <div v-if="feedback.length === 0" class="empty-line">没有负反馈记录</div>
      <div v-for="(f, i) in feedback" :key="i" class="block-row">
        <span>{{ f.kind === 'not_interested' ? '不感兴趣' : '减少话题' }} · {{ f.target_type }} #{{ f.target_id }}</span>
        <el-button size="small" text @click="removeFb(f)">撤销</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="less">
.pref-view { max-width: 720px; }
.pref-card { padding: 20px 24px; margin-bottom: 12px; h3 { margin-top: 0; } }
.pref-row { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
.pref-title { margin: 0; font-weight: 600; }
.pref-desc { margin: 4px 0 0; color: var(--bb-text-tertiary); font-size: 12px; }
.topic-chips { display: flex; flex-wrap: wrap; gap: 8px; }
.topic-chip { cursor: pointer; }
.block-row {
  display: flex; justify-content: space-between; align-items: center;
  padding: 8px 0; border-bottom: 1px solid var(--bb-border);
}
.empty-line { color: var(--bb-text-tertiary); font-size: 13px; padding: 6px 0; }
</style>
