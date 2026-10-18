// router.ts 前端路由（history 模式，base=/admin/）。
// 未登录只有登录页可达；token 校验由 /api/admin/session 完成。

import { createRouter, createWebHistory } from 'vue-router'
import { tokenStore } from './api'
import { state } from './store'

const routes = [
  { path: '/', redirect: '/overview' },
  { path: '/login', name: 'login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
  { path: '/overview', name: 'overview', component: () => import('./views/OverviewView.vue') },
  { path: '/accounts', name: 'accounts', component: () => import('./views/AccountsView.vue') },
  { path: '/keys', name: 'keys', component: () => import('./views/KeysView.vue') },
  { path: '/logs', name: 'logs', component: () => import('./views/LogsView.vue') },
  { path: '/models', name: 'models', component: () => import('./views/ModelsView.vue') },
  { path: '/chat', name: 'chat', component: () => import('./views/ChatView.vue') },
  { path: '/settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
  { path: '/:pathMatch(.*)*', redirect: '/overview' },
]

export const router = createRouter({
  history: createWebHistory('/admin/'),
  routes,
})

router.beforeEach((to) => {
  const hasToken = tokenStore.get() !== ''
  // 服务端未启用鉴权时（token 为空）无需登录页。
  const needLogin = hasToken === false && state.authRequired
  if (to.meta.public) {
    return true
  }
  if (needLogin) {
    return { name: 'login', query: to.fullPath === '/overview' ? undefined : { next: to.fullPath } }
  }
  return true
})