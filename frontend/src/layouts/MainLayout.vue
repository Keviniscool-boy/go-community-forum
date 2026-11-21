<script setup lang="ts">
// 主布局：顶部导航 + 左侧导航 + 中间内容 + 右侧边栏
// 移动端（<768px）：汉堡菜单打开导航抽屉（Escape 关闭、路由变化关闭），
// 底部主导航（首页/发现/提问/通知/我的）保证主任务可达（PRD §6.1）
import { ref, computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { HomeFilled, EditPen, Compass, TrendCharts, Document, Star, Clock, Setting, Notebook, Collection, DataAnalysis, ChatDotRound, Bell, User } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { useNotificationStore } from '@/stores/notification'
import TopNav from '@/components/TopNav.vue'

const auth = useAuthStore()
const notification = useNotificationStore()
const route = useRoute()
const router = useRouter()

const isMobileSidebarVisible = ref(false)

// 路由变化后关闭抽屉（PRD §6.1）
watch(
  () => route.fullPath,
  () => {
    if (isMobileSidebarVisible.value) isMobileSidebarVisible.value = false
  },
)

// 底部导航"我的"高亮：当前正在看自己的主页
const isSelfProfile = computed(() => !!auth.user && route.fullPath === `/users/${auth.user.user_id}`)

// 未登录点击通知/我的 → 登录页并保留返回路径
function onBottomNav(path: string) {
  const needAuth = path === '/notifications' || path === '/me'
  if (needAuth && !auth.isLoggedIn) {
    router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  router.push(path === '/me' && auth.user ? `/users/${auth.user.user_id}` : path)
}
</script>

<template>
  <div class="app-shell">
    <TopNav @toggle-sidebar="isMobileSidebarVisible = !isMobileSidebarVisible" />

    <!-- 移动端导航抽屉：全部一级入口 + 登录态；Escape 关闭由 el-drawer 默认支持 -->
    <el-drawer
      v-model="isMobileSidebarVisible"
      direction="ltr"
      size="78%"
      :title="auth.isLoggedIn && auth.user ? auth.user.username : '导航菜单'"
      :append-to-body="true"
    >
      <nav class="drawer-nav" aria-label="主导航">
        <router-link class="side-link" to="/">
          <el-icon><HomeFilled /></el-icon><span>首页</span>
        </router-link>
        <router-link class="side-link" to="/discover">
          <el-icon><Compass /></el-icon><span>发现</span>
        </router-link>
        <router-link class="side-link" to="/hot">
          <el-icon><TrendCharts /></el-icon><span>热榜</span>
        </router-link>
        <router-link class="side-link" to="/questions/ask">
          <el-icon><EditPen /></el-icon><span>提问</span>
        </router-link>
        <router-link class="side-link" to="/articles">
          <el-icon><Notebook /></el-icon><span>文章</span>
        </router-link>
        <router-link class="side-link" to="/columns">
          <el-icon><Collection /></el-icon><span>专栏</span>
        </router-link>
        <router-link v-if="auth.isLoggedIn" class="side-link" to="/me/content">
          <el-icon><Document /></el-icon><span>我的内容</span>
        </router-link>
        <router-link v-if="auth.isLoggedIn" class="side-link" to="/drafts">
          <el-icon><Document /></el-icon><span>草稿箱</span>
        </router-link>
        <router-link v-if="auth.isLoggedIn" class="side-link" to="/bookmarks">
          <el-icon><Star /></el-icon><span>收藏夹</span>
        </router-link>
        <router-link v-if="auth.isLoggedIn" class="side-link" to="/messages">
          <el-icon><ChatDotRound /></el-icon><span>私信</span>
        </router-link>
        <router-link v-if="auth.isLoggedIn" class="side-link" to="/history">
          <el-icon><Clock /></el-icon><span>历史</span>
        </router-link>
        <router-link v-if="auth.isLoggedIn" class="side-link" to="/creator">
          <el-icon><DataAnalysis /></el-icon><span>创作者中心</span>
        </router-link>
        <router-link v-if="auth.isLoggedIn" class="side-link" to="/settings/personalization">
          <el-icon><Compass /></el-icon><span>推荐设置</span>
        </router-link>
        <router-link v-if="auth.user?.role === 'admin' || auth.user?.role === 'moderator'" class="side-link" to="/admin">
          <el-icon><Setting /></el-icon><span>管理后台</span>
        </router-link>
      </nav>
      <div v-if="!auth.isLoggedIn" class="side-login-tip">
        <el-button type="primary" style="width: 100%" @click="$router.push('/login')">登录</el-button>
      </div>
    </el-drawer>

    <div class="app-body">
      <!-- 左侧导航（桌面端） -->
      <aside class="left-sidebar bb-card">
        <nav class="side-nav" aria-label="主导航">
          <router-link class="side-link" to="/">
            <el-icon><HomeFilled /></el-icon><span>首页</span>
          </router-link>
          <router-link class="side-link" to="/discover">
            <el-icon><Compass /></el-icon><span>发现</span>
          </router-link>
          <router-link class="side-link" to="/hot">
            <el-icon><TrendCharts /></el-icon><span>热榜</span>
          </router-link>
          <router-link class="side-link" to="/questions/ask">
            <el-icon><EditPen /></el-icon><span>提问</span>
          </router-link>
          <router-link class="side-link" to="/articles">
            <el-icon><Notebook /></el-icon><span>文章</span>
          </router-link>
          <router-link class="side-link" to="/columns">
            <el-icon><Collection /></el-icon><span>专栏</span>
          </router-link>
          <router-link class="side-link" to="/drafts">
            <el-icon><Document /></el-icon><span>草稿箱</span>
          </router-link>
          <router-link class="side-link" to="/bookmarks">
            <el-icon><Star /></el-icon><span>收藏夹</span>
          </router-link>
          <router-link class="side-link" to="/messages">
            <el-icon><ChatDotRound /></el-icon><span>私信</span>
          </router-link>
          <router-link class="side-link" to="/history">
            <el-icon><Clock /></el-icon><span>历史</span>
          </router-link>
          <router-link v-if="auth.isLoggedIn" class="side-link" to="/creator">
            <el-icon><DataAnalysis /></el-icon><span>创作者中心</span>
          </router-link>
          <router-link v-if="auth.isLoggedIn" class="side-link" to="/settings/personalization">
            <el-icon><Compass /></el-icon><span>推荐设置</span>
          </router-link>
          <router-link v-if="auth.user?.role === 'admin' || auth.user?.role === 'moderator'" class="side-link" to="/admin">
            <el-icon><Setting /></el-icon><span>管理后台</span>
          </router-link>
        </nav>
        <div v-if="!auth.isLoggedIn" class="side-login-tip">
          <el-button type="primary" style="width: 100%" @click="$router.push('/login')">登录</el-button>
        </div>
      </aside>

      <!-- 中间内容 -->
      <main class="main-content">
        <router-view />
      </main>

      <!-- 右侧边栏（桌面端）：热门话题 -->
      <aside class="right-sidebar">
        <div class="bb-card right-block">
          <div class="right-block-title">发现</div>
          <div class="right-block-links">
            <router-link class="right-link" to="/discover">热门话题</router-link>
            <router-link class="right-link" to="/hot">热榜内容</router-link>
          </div>
        </div>
      </aside>
    </div>

    <!-- 移动端底部主导航 -->
    <nav class="bottom-nav" aria-label="底部导航">
      <button class="bottom-nav-item" :class="{ active: route.fullPath === '/' }" @click="onBottomNav('/')">
        <el-icon><HomeFilled /></el-icon><span>首页</span>
      </button>
      <button class="bottom-nav-item" :class="{ active: route.fullPath.startsWith('/discover') }" @click="onBottomNav('/discover')">
        <el-icon><Compass /></el-icon><span>发现</span>
      </button>
      <button class="bottom-nav-item bottom-nav-ask" @click="onBottomNav('/questions/ask')">
        <el-icon><EditPen /></el-icon><span>提问</span>
      </button>
      <button class="bottom-nav-item" :class="{ active: route.fullPath.startsWith('/notifications') }" @click="onBottomNav('/notifications')">
        <el-badge :value="notification.unreadCount" :hidden="notification.unreadCount === 0" :max="99">
          <el-icon><Bell /></el-icon>
        </el-badge>
        <span>通知</span>
      </button>
      <button class="bottom-nav-item" :class="{ active: isSelfProfile }" @click="onBottomNav('/me')">
        <el-icon><User /></el-icon><span>我的</span>
      </button>
    </nav>
  </div>
</template>

<style scoped lang="less">
.app-shell {
  min-height: 100vh;
}

.app-body {
  display: flex;
  gap: 16px;
  max-width: 1200px;
  margin: 16px auto;
  padding: 0 16px;
  align-items: flex-start;
}

.left-sidebar {
  width: 200px;
  flex-shrink: 0;
  padding: 12px 8px;
  position: sticky;
  top: 72px;
}

.side-nav {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.drawer-nav {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.side-link {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-radius: 6px;
  color: var(--bb-text-secondary);
  font-size: 15px;
  transition: background 0.15s;

  &:hover {
    background: var(--bb-bg-page);
  }

  &.router-link-active {
    color: var(--bb-primary);
    font-weight: 600;
  }
}

.side-login-tip {
  padding: 12px 8px 4px;
  border-top: 1px solid var(--bb-border);
  margin-top: 8px;
}

.main-content {
  flex: 1;
  min-width: 0;
}

.right-sidebar {
  width: 300px;
  flex-shrink: 0;
  position: sticky;
  top: 72px;
}

.right-block {
  padding: 16px;
}

.right-block-title {
  font-weight: 600;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--bb-border);
}

.right-block-links {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.right-link {
  padding: 8px 4px;
  font-size: 14px;
  color: var(--bb-text-secondary);
  border-radius: 4px;

  &:hover {
    color: var(--bb-primary);
    background: var(--bb-bg-page);
  }
}

.bottom-nav {
  display: none;
}

// 移动端：隐藏两侧栏，显示底部导航与抽屉
@media (max-width: 768px) {
  .app-body {
    margin: 8px;
    padding: 0 8px 56px; // 底部导航高度预留，内容不被遮挡
  }

  .left-sidebar,
  .right-sidebar {
    display: none;
  }

  .bottom-nav {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 90;
    display: flex;
    background: #fff;
    border-top: 1px solid var(--bb-border);
    padding: 4px 0 calc(4px + env(safe-area-inset-bottom));
  }

  .bottom-nav-item {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    border: none;
    background: transparent;
    color: var(--bb-text-tertiary);
    font-size: 11px;
    cursor: pointer;
    padding: 6px 0;

    .el-icon {
      font-size: 20px;
    }

    &.active {
      color: var(--bb-primary);
    }
  }

  .bottom-nav-ask {
    color: var(--bb-primary);
    font-weight: 600;
  }
}
</style>
