<script setup lang="ts">
// 主布局：顶部导航 + 左侧导航 + 中间内容 + 右侧边栏
// 移动端（<768px）隐藏两侧栏，Feed 单列显示
import { ref } from 'vue'
import { HomeFilled, EditPen } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import TopNav from '@/components/TopNav.vue'

const auth = useAuthStore()
const isMobileSidebarVisible = ref(false)
</script>

<template>
  <div class="app-shell">
    <TopNav @toggle-sidebar="isMobileSidebarVisible = !isMobileSidebarVisible" />
    <div class="app-body">
      <!-- 左侧导航（桌面端） -->
      <aside class="left-sidebar bb-card">
        <nav class="side-nav">
          <router-link class="side-link" to="/">
            <el-icon><HomeFilled /></el-icon><span>首页</span>
          </router-link>
          <router-link class="side-link" to="/ask">
            <el-icon><EditPen /></el-icon><span>发布</span>
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

      <!-- 右侧边栏（桌面端）：热榜占位，M5 接入 -->
      <aside class="right-sidebar">
        <div class="bb-card right-block">
          <div class="right-block-title">热榜</div>
          <div class="right-block-empty">热榜即将上线</div>
        </div>
      </aside>
    </div>
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

.right-block-empty {
  color: var(--bb-text-tertiary);
  font-size: 13px;
  padding-top: 12px;
}

// 移动端：隐藏两侧栏
@media (max-width: 768px) {
  .app-body {
    margin: 8px;
    padding: 0 8px;
  }

  .left-sidebar,
  .right-sidebar {
    display: none;
  }
}
</style>
