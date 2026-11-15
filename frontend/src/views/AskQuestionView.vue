<script setup lang="ts">
// 提问页：发布新问题
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { createQuestion } from '@/api/question'
import { ApiError } from '@/api/http'

const router = useRouter()

const title = ref('')
const description = ref('')
const submitting = ref(false)

async function onSubmit() {
  if (!title.value.trim()) return ElMessage.warning('请输入问题标题')
  if (title.value.trim().length < 5) return ElMessage.warning('标题至少 5 个字符')
  submitting.value = true
  try {
    const res = await createQuestion({
      title: title.value.trim(),
      description: description.value.trim(),
    })
    ElMessage.success('发布成功')
    router.push(`/questions/${res.question_id}`)
  } catch (e) {
    if (e instanceof ApiError) ElMessage.error(e.message)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="ask-view bb-card">
    <h2>提出你的问题</h2>
    <el-form label-position="top" @submit.prevent="onSubmit">
      <el-form-item label="问题标题">
        <el-input
          v-model="title"
          maxlength="200"
          show-word-limit
          size="large"
          placeholder="用一句话把问题说清楚，例如：Go 中如何设计一个 Feed 流？"
        />
      </el-form-item>
      <el-form-item label="问题补充说明（可选）">
        <el-input
          v-model="description"
          type="textarea"
          :rows="10"
          maxlength="10000"
          show-word-limit
          placeholder="补充问题背景、你已经尝试过的方案等，帮助回答者理解你的问题"
        />
      </el-form-item>
      <el-button type="primary" size="large" :loading="submitting" native-type="submit">发布问题</el-button>
    </el-form>
  </div>
</template>

<style scoped lang="less">
.ask-view {
  padding: 24px 28px;

  h2 {
    margin-top: 0;
  }
}
</style>
