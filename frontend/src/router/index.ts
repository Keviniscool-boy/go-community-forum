import { createRouter, createWebHistory } from 'vue-router'
import MainLayout from '@/layouts/MainLayout.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      component: MainLayout,
      children: [
        { path: '', name: 'home', component: () => import('@/views/HomeView.vue'), meta: { title: '首页' } },
        { path: 'post/:id', name: 'post-detail', component: () => import('@/views/PostDetailView.vue'), meta: { title: '帖子详情' } },
        { path: 'questions/ask', name: 'ask-question', component: () => import('@/views/AskQuestionView.vue'), meta: { title: '提问', requiresAuth: true } },
        { path: 'questions/:id', name: 'question-detail', component: () => import('@/views/QuestionDetailView.vue'), meta: { title: '问题详情' } },
        { path: 'ask', redirect: { name: 'ask-question' } },
        { path: 'hot', name: 'hot', component: () => import('@/views/HomeView.vue'), meta: { title: '热榜' } },
        { path: 'discover', name: 'discover', component: () => import('@/views/DiscoverView.vue'), meta: { title: '发现' } },
        { path: 'topics/:id', name: 'topic', component: () => import('@/views/TopicView.vue'), meta: { title: '话题详情' } },
        { path: 'users/:id', name: 'user-profile', component: () => import('@/views/UserProfileView.vue'), meta: { title: '用户主页' } },
        { path: 'notifications', name: 'notifications', component: () => import('@/views/NotificationsView.vue'), meta: { title: '通知', requiresAuth: true } },
        { path: 'settings/profile', name: 'settings-profile', component: () => import('@/views/SettingsProfileView.vue'), meta: { title: '个人资料', requiresAuth: true } },
      ],
    },
    {
      path: '/',
      component: () => import('@/layouts/AuthLayout.vue'),
      children: [
        { path: 'login', name: 'login', component: () => import('@/views/LoginView.vue'), meta: { title: '登录', authPage: true } },
        { path: 'register', name: 'register', component: () => import('@/views/RegisterView.vue'), meta: { title: '注册', authPage: true } },
        { path: 'forgot-password', name: 'forgot-password', component: () => import('@/views/ForgotPasswordView.vue'), meta: { title: '找回密码', authPage: true } },
      ],
    },
    { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('@/views/NotFoundView.vue'), meta: { title: '页面不存在' } },
  ],
})

// 登录态恢复：路由守卫，需要登录的页面未登录时跳登录页
router.beforeEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} - Bluebell 知识社区` : 'Bluebell 知识社区'
  if (to.meta.requiresAuth && !localStorage.getItem('bb_token')) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  // 已登录用户访问登录/注册页时跳回首页
  if (to.meta.authPage && localStorage.getItem('bb_token')) {
    return { name: 'home' }
  }
})

export default router
