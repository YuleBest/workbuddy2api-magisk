<script setup lang="ts">
// App.vue 外壳：登录页独立成屏，其余页面共用"仪表外壳 + 读数面板"两栏骨架。
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { state, logout, dismissToast, refreshOptions, setTheme } from './store'
import { tokenStore } from './api'

const route = useRoute()
const router = useRouter()

const isPlain = computed(() => route.meta.public === true && !state.authed)

function quit() {
  logout()
  void router.push({ name: 'login' })
}

function onRefreshChange(e: Event) {
  const v = Number((e.target as HTMLSelectElement).value)
  state.refreshSec = v
}
</script>

<template>
  <RouterView v-if="isPlain" />

  <div v-else class="shell">
    <aside class="rail">
      <div class="rail-brand">
        <span class="mark" aria-hidden="true">
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none">
            <path d="M1 6h3M8 6h3M4 3v6M8 3v6" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" />
          </svg>
        </span>
        <span>
          <span class="name">网关后台</span>
          <span class="sub">workbuddy2api {{ state.version }}</span>
        </span>
      </div>

      <nav class="rail-nav">
        <RouterLink to="/overview">总览</RouterLink>
        <RouterLink to="/accounts">账号池</RouterLink>
        <RouterLink to="/keys">调用密钥</RouterLink>
        <RouterLink to="/logs">请求日志</RouterLink>
        <RouterLink to="/models">模型</RouterLink>
        <RouterLink to="/chat">模型对话</RouterLink>
        <RouterLink to="/settings">设置</RouterLink>
      </nav>

      <div class="rail-foot">
        <div class="row">
          <span class="lamp" :class="state.authed ? 'ok live' : 'bad'"></span>
          <span>{{ state.authed ? '管理会话已连接' : '未连接' }}</span>
        </div>
        <div class="row">
          <span>网关</span>
          <code style="background: transparent; border: none; color: var(--rail-ink)">{{
            state.service === 'workbuddy2api' ? '在线' : state.service
          }}</code>
        </div>
      </div>
    </aside>

    <div class="board">
      <header class="topbar">
        <span class="meta">{{ state.service }}</span>
        <span class="meta">{{ state.version }}</span>
        <span class="spacer"></span>

        <label class="checkline">
          刷新
          <select :value="state.refreshSec" @change="onRefreshChange">
            <option v-for="o in refreshOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </label>

        <button
          class="ghost"
          type="button"
          :title="state.theme === 'light' ? '切换到深色' : '切换到浅色'"
          @click="setTheme(state.theme === 'light' ? 'dark' : 'light')"
        >
          {{ state.theme === 'light' ? '深色' : '浅色' }}
        </button>

        <button v-if="tokenStore.get()" class="ghost" type="button" @click="quit">退出</button>
      </header>

      <RouterView />
    </div>
  </div>

  <div class="toasts" aria-live="polite">
    <div
      v-for="t in state.toasts"
      :key="t.id"
      class="toast"
      :class="{ bad: t.kind === 'bad', warn: t.kind === 'warn' }"
      @click="dismissToast(t.id)"
    >
      {{ t.text }}
    </div>
  </div>
</template>