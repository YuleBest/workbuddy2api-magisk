<script setup lang="ts">
// App.vue 外壳：登录页独立成屏；其余页面共用墨黑侧栏 + 羊皮纸正文的章节骨架。
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Boxes,
  KeyRound,
  LayoutDashboard,
  LogOut,
  MessagesSquare,
  Moon,
  ScrollText,
  Settings,
  Sun,
  Users,
} from '@lucide/vue'
import { state, dismissToast, logout, refreshOptions, setTheme } from './store'
import { tokenStore } from './api'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'

const route = useRoute()
const router = useRouter()

const isPlain = computed(() => route.meta.public === true)
const pageTitle = computed(() => (route.meta.title as string | undefined) ?? '控制台')

const nav = [
  { to: '/overview', label: '总览', icon: LayoutDashboard },
  { to: '/accounts', label: '账号池', icon: Users },
  { to: '/keys', label: '调用密钥', icon: KeyRound },
  { to: '/logs', label: '请求日志', icon: ScrollText },
  { to: '/models', label: '模型', icon: Boxes },
  { to: '/chat', label: '模型对话', icon: MessagesSquare },
  { to: '/settings', label: '设置', icon: Settings },
]

const refreshValue = computed({
  get: () => String(state.refreshSec),
  set: (value: string) => {
    state.refreshSec = Number(value)
  },
})

function quit() {
  logout()
  void router.push({ name: 'login' })
}

function toastTone(kind: 'ok' | 'warn' | 'bad') {
  return kind === 'bad' ? 'bg-status-bad' : kind === 'warn' ? 'bg-status-warn' : 'bg-status-ok'
}
</script>

<template>
  <TooltipProvider :delay-duration="350">
    <RouterView v-if="isPlain" v-slot="{ Component }">
      <Transition name="page" mode="out-in">
        <component :is="Component" />
      </Transition>
    </RouterView>

    <div v-else class="min-h-[100dvh] lg:grid lg:grid-cols-[15.5rem_minmax(0,1fr)]">
      <aside
        class="border-b border-sidebar-border bg-sidebar text-sidebar-foreground lg:sticky lg:top-0 lg:flex lg:h-[100dvh] lg:flex-col lg:border-r lg:border-b-0"
      >
        <div class="flex items-center gap-3 px-4 py-4 lg:px-5 lg:py-6">
          <span
            class="grid size-9 shrink-0 place-items-center rounded-md border border-sidebar-border text-sidebar-primary"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path
                d="M4.5 20V10L12 4l7.5 6v10"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linejoin="round"
              />
              <path d="M9 20v-6.5h6V20" stroke="currentColor" stroke-width="1.5" />
              <path d="M4.5 20h15" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
            </svg>
          </span>
          <span class="min-w-0">
            <span class="block font-serif text-[1.02rem] leading-tight">网关后台</span>
            <span
              class="mono mt-0.5 block truncate text-[0.6rem] tracking-[0.12em] text-sidebar-foreground/45"
            >
              WORKBUDDY2API · {{ state.version }}
            </span>
          </span>
        </div>

        <nav
          class="flex gap-1 overflow-x-auto px-3 pb-3 lg:flex-1 lg:flex-col lg:overflow-visible lg:px-3 lg:pb-0"
        >
          <RouterLink
            v-for="item in nav"
            :key="item.to"
            :to="item.to"
            class="nav-link relative flex shrink-0 items-center gap-2.5 rounded-md border-b-2 border-transparent px-3 py-2 text-[0.8rem] text-sidebar-foreground/60 transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground lg:border-b-0 lg:border-l-2 lg:gap-3"
          >
            <component :is="item.icon" class="size-4 opacity-65" aria-hidden="true" />
            <span class="whitespace-nowrap">{{ item.label }}</span>
          </RouterLink>
        </nav>

        <div class="hidden border-t border-sidebar-border px-5 py-4 lg:block">
          <div class="flex items-center gap-2.5 text-[0.72rem] text-sidebar-foreground/60">
            <span class="lamp" :class="state.authed ? 'ok live' : 'bad'"></span>
            {{ state.authed ? '管理会话已连接' : '未连接' }}
          </div>
          <div
            class="mt-3 flex items-center justify-between text-[0.68rem] tracking-[0.06em] text-sidebar-foreground/40"
          >
            <span>网关服务</span>
            <span class="mono">{{
              state.service === 'workbuddy2api' ? '在线' : state.service
            }}</span>
          </div>
        </div>
      </aside>

      <div class="flex min-w-0 flex-col">
        <header
          class="sticky top-0 z-30 border-b border-border bg-background/85 backdrop-blur-md"
        >
          <div class="flex h-14 items-center gap-2 px-4 sm:gap-3 sm:px-6 lg:px-8">
            <div class="flex min-w-0 items-baseline gap-2">
              <span
                class="mono hidden text-[0.6rem] tracking-[0.14em] text-muted-foreground sm:inline"
              >
                WORKBUDDY2API
              </span>
              <span class="hidden text-muted-foreground/50 sm:inline">/</span>
              <span class="truncate font-serif text-[0.98rem]">{{ pageTitle }}</span>
            </div>

            <span class="flex-1"></span>

            <span class="mono hidden text-[0.66rem] text-muted-foreground md:inline">
              v{{ state.version }}
            </span>

            <span class="hidden text-[0.68rem] text-muted-foreground md:inline">自动刷新</span>
            <Select v-model="refreshValue">
              <SelectTrigger size="sm" class="w-[5.1rem] text-xs" aria-label="自动刷新间隔">
                <SelectValue />
              </SelectTrigger>
              <SelectContent align="end">
                <SelectItem v-for="o in refreshOptions" :key="o.value" :value="String(o.value)">
                  {{ o.label }}
                </SelectItem>
              </SelectContent>
            </Select>

            <Tooltip>
              <TooltipTrigger as-child>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  :aria-label="state.theme === 'light' ? '切换到深色' : '切换到浅色'"
                  @click="setTheme(state.theme === 'light' ? 'dark' : 'light')"
                >
                  <Sun v-if="state.theme === 'light'" class="size-4" />
                  <Moon v-else class="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                {{ state.theme === 'light' ? '切换到深色' : '切换到浅色' }}
              </TooltipContent>
            </Tooltip>

            <Tooltip v-if="tokenStore.get()">
              <TooltipTrigger as-child>
                <Button variant="ghost" size="icon-sm" aria-label="退出登录" @click="quit">
                  <LogOut class="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>退出登录</TooltipContent>
            </Tooltip>
          </div>
        </header>

        <main class="mx-auto w-full max-w-[1280px] px-4 py-7 sm:px-6 lg:px-8 lg:py-9">
          <RouterView v-slot="{ Component }">
            <Transition name="page" mode="out-in">
              <component :is="Component" />
            </Transition>
          </RouterView>
        </main>
      </div>
    </div>

    <Teleport to="body">
      <TransitionGroup
        name="toast"
        tag="div"
        class="pointer-events-none fixed inset-x-4 bottom-4 z-[500] flex flex-col items-end gap-2 sm:inset-x-auto sm:right-6 sm:bottom-6"
        aria-live="polite"
      >
        <button
          v-for="t in state.toasts"
          :key="t.id"
          type="button"
          class="pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-md border border-border bg-card px-4 py-3 text-left shadow-lift transition-transform hover:-translate-y-px"
          title="点击关闭"
          @click="dismissToast(t.id)"
        >
          <span class="mt-1.5 size-1.5 shrink-0 rounded-full" :class="toastTone(t.kind)"></span>
          <span class="text-[0.8rem] leading-relaxed">{{ t.text }}</span>
        </button>
      </TransitionGroup>
    </Teleport>
  </TooltipProvider>
</template>

<style scoped>
.nav-link.router-link-active {
  background: var(--sidebar-accent);
  color: var(--sidebar-foreground);
}

.nav-link.router-link-active svg {
  opacity: 1;
  color: var(--sidebar-primary);
}

@media (max-width: 1023px) {
  .nav-link.router-link-active {
    border-bottom-color: var(--sidebar-primary);
  }
}

@media (min-width: 1024px) {
  .nav-link.router-link-active {
    border-left-color: var(--sidebar-primary);
  }
}

.toast-enter-active,
.toast-leave-active {
  transition:
    opacity 180ms ease,
    transform 180ms ease;
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateY(6px);
}
</style>
