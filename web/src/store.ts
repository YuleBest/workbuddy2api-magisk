// store.ts 全局小状态：会话信息、刷新节奏、提示条。
// 刻意不用 pinia——整个后台的跨页状态只有这几项，reactive 对象足够。

import { reactive } from 'vue'
import { ApiError, tokenStore } from './api'

export interface Toast {
  id: number
  kind: 'ok' | 'warn' | 'bad'
  text: string
}

export const state = reactive({
  service: 'workbuddy2api',
  version: 'dev',
  authRequired: true,
  /** 已通过 token 校验（登录页设，退出/401 清）。 */
  authed: false,
  /** 自动刷新间隔（秒），0 = 不自动刷新。 */
  refreshSec: 5,
  theme: (localStorage.getItem('wb2api.admin.theme') as 'light' | 'dark') || 'light',
  toasts: [] as Toast[],
})

let toastSeq = 0

export function toast(text: string, kind: Toast['kind'] = 'ok') {
  const id = ++toastSeq
  state.toasts.push({ id, kind, text })
  setTimeout(() => dismissToast(id), kind === 'bad' ? 9000 : 4500)
}

export function dismissToast(id: number) {
  const i = state.toasts.findIndex((t) => t.id === id)
  if (i >= 0) state.toasts.splice(i, 1)
}

export function setTheme(theme: 'light' | 'dark') {
  state.theme = theme
  localStorage.setItem('wb2api.admin.theme', theme)
  document.documentElement.dataset.theme = theme
}

export function logout() {
  tokenStore.clear()
  state.authed = false
}

/** 统一包一层：把 API 异常变成提示条；401 直接登出（路由守卫会送回登录页）。 */
export async function guard<T>(fn: () => Promise<T>): Promise<T | undefined> {
  try {
    return await fn()
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.status === 401) {
        logout()
        toast('管理 token 已失效，请重新登录', 'warn')
        return undefined
      }
      toast(e.message, 'bad')
      return undefined
    }
    toast('发生未知错误', 'bad')
    throw e
  }
}

/** 自动刷新间隔：0 表示手动。 */
export const refreshOptions = [
  { value: 0, label: '手动' },
  { value: 3, label: '3 秒' },
  { value: 5, label: '5 秒' },
  { value: 10, label: '10 秒' },
  { value: 30, label: '30 秒' },
]