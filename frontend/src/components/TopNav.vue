<script setup lang="ts">
// 顶部导航：Logo、搜索框、发布按钮、用户头像（移动端增加菜单按钮）
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Menu, Search } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const emit = defineEmits<{ (e: 'toggle-sidebar'): void }>()

const auth = useAuthStore()
const router = useRouter()

const keyword = ref('')

const defaultAvatar = computed(
  () =>
    'data:image/svg+xml;utf8,' +
    encodeURIComponent(
      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40"><circle cx="20" cy="20" r="20" fill="#d5e7fb"/><circle cx="20" cy="15" r="7" fill="#7db5e8"/><path d="M6 36a14 14 0 0 1 28 0z" fill="#7db5e8"/></svg>`,
    ),
)

function onSearch() {
  // 搜索接口在 M5 接入，先提示
  if (keyword.value.trim()) ElMessage.info('搜索功能即将上线')
}

function onAvatarCommand(command: string) {
  if (command === 'logout') {
    auth.logout()
    ElMessage.success('已退出登录')
    router.push('/login')
  } else if (command === 'profile') {
    if (auth.user) router.push(`/users/${auth.user.user_id}`)
  }
}
</script>

<template>
  <header class="top-nav">
    <div class="top-nav-inner">
      <el-icon class="mobile-menu-btn" @click="emit('toggle-sidebar')"><Menu /></el-icon>
      <router-link class="logo" to="/">Bluebell</router-link>
      <el-input
        v-model="keyword"
        class="search-input"
        placeholder="搜索你感兴趣的内容…"
        :prefix-icon="Search"
        @keyup.enter="onSearch"
      />
      <div class="nav-actions">
        <el-button type="primary" size="small" round @click="router.push('/ask')">发布</el-button>
        <template v-if="auth.isLoggedIn && auth.user">
          <el-dropdown trigger="click" @command="onAvatarCommand">
            <el-avatar :size="32" :src="auth.user.avatar_url || defaultAvatar" />
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>{{ auth.user.username }}</el-dropdown-item>
                <el-dropdown-item divided command="profile">个人主页</el-dropdown-item>
                <el-dropdown-item command="logout">退出登录</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
        <template v-else>
          <el-button text size="small" @click="router.push('/login')">登录</el-button>
        </template>
      </div>
    </div>
  </header>
</template>

<style scoped lang="less">
.top-nav {
  position: sticky;
  top: 0;
  z-index: 100;
  background: #fff;
  border-bottom: 1px solid var(--bb-border);
}

.top-nav-inner {
  display: flex;
  align-items: center;
  gap: 16px;
  max-width: 1200px;
  margin: 0 auto;
  padding: 0 16px;
  height: 56px;
}

.mobile-menu-btn {
  display: none;
  font-size: 20px;
  cursor: pointer;
}

.logo {
  font-size: 20px;
  font-weight: 700;
  color: var(--bb-primary);
  flex-shrink: 0;
}

.search-input {
  max-width: 400px;
  flex: 1;
}

.nav-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-left: auto;
  flex-shrink: 0;
}

@media (max-width: 768px) {
  .mobile-menu-btn {
    display: block;
  }

  .search-input {
    max-width: none;
  }
}
</style>
