<script setup lang="ts">
// 登录页：只做一件事——把这台网关的调用 key 填进来。
// 不用用户名/密码：网关本身就没有账号体系，凭据就是 keys.json 里的 key。
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CircleAlert, LogIn } from '@lucide/vue'
import { api, ApiError, tokenStore } from '../api'
import { state, toast } from '../store'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

const token = ref('')
const busy = ref(false)
const error = ref('')
const route = useRoute()
const router = useRouter()

async function submit() {
  const value = token.value.trim()
  if (!value) {
    error.value = '请输入调用 key'
    return
  }
  busy.value = true
  error.value = ''
  tokenStore.set(value)
  try {
    const info = await api.session()
    state.service = info.service
    state.version = info.version
    state.authRequired = info.auth_required
    state.authed = true
    toast('已登录管理后台')
    const next = typeof route.query.next === 'string' ? route.query.next : '/overview'
    await router.push(next)
  } catch (e) {
    tokenStore.clear()
    state.authed = false
    if (e instanceof ApiError && e.status === 401) {
      error.value = '这个 key 不对：网关拒绝了它。请核对 keys.json 里的值。'
    } else {
      error.value = e instanceof Error ? e.message : '登录失败'
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="grid min-h-[100dvh] lg:grid-cols-[1.08fr_0.92fr]">
    <section
      class="surface-ink relative hidden flex-col justify-between overflow-hidden px-10 py-12 lg:flex lg:px-14 lg:py-14"
    >
      <div class="flex items-center gap-3">
        <span
          class="grid size-9 place-items-center rounded-md border border-[color:var(--inverse-line)] text-[color:var(--inverse-accent)]"
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
        <span class="mono text-[0.62rem] tracking-[0.16em] ink-muted">WORKBUDDY2API</span>
      </div>

      <div class="max-w-[35rem]">
        <h1 class="font-serif text-[clamp(2rem,3.2vw,3rem)] leading-[1.12] tracking-[-0.025em]">
          <span class="block whitespace-nowrap">把账号池、密钥和请求，</span>
          <span class="block whitespace-nowrap ink-accent">放在一页纸上看清楚。</span>
        </h1>
        <p class="mt-6 max-w-[46ch] text-[0.92rem] leading-relaxed ink-muted">
          这是 WorkBuddy2API 网关的管理台。账号状态、调用日志、模型清单都直接读自网关进程，
          不经过任何第三方服务。
        </p>
      </div>

      <div class="flex items-center gap-4 text-[0.66rem] tracking-[0.08em] ink-muted">
        <span class="mono">{{ state.service }}</span>
        <span class="h-px w-8 bg-[color:var(--inverse-line)]"></span>
        <span class="mono">{{ state.version }}</span>
      </div>
    </section>

    <section class="flex items-center justify-center px-5 py-12 sm:px-10">
      <div class="w-full max-w-[380px]">
        <div class="mb-8 flex items-center gap-3 lg:hidden">
          <span
            class="grid size-9 place-items-center rounded-md border border-border text-primary"
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
          <span class="font-serif text-[1.05rem]">网关后台</span>
        </div>

        <div class="mb-7">
          <h2 class="font-serif text-[1.7rem] leading-tight">进入管理台</h2>
          <p class="mt-2 text-[0.82rem] leading-relaxed text-muted-foreground">
            填入一个调用 key 验证身份。这个界面能改账号池状态和<span class="whitespace-nowrap">密钥</span>，敏感程度等同 API key。
          </p>
        </div>

        <form class="space-y-4" @submit.prevent="submit">
          <div class="space-y-2">
            <Label for="token">调用 key</Label>
            <Input
              id="token"
              v-model="token"
              type="password"
              autocomplete="current-password"
              spellcheck="false"
              placeholder="sk-..."
              :disabled="busy"
              autofocus
            />
          </div>
          <Button class="w-full" type="submit" :disabled="busy">
            <LogIn class="size-4" />
            {{ busy ? '正在验证…' : '进入后台' }}
          </Button>
          <p
            v-if="error"
            class="flex items-start gap-2 text-[0.8rem] leading-relaxed text-destructive"
          >
            <CircleAlert class="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
            {{ error }}
          </p>
        </form>

        <div class="mt-10 border-t border-border pt-5">
          <p class="text-[0.68rem] font-medium tracking-[0.08em] text-muted-foreground">
            key 从哪来
          </p>
          <div class="mt-2.5 space-y-1.5 text-[0.78rem] leading-relaxed text-muted-foreground">
            <p>
              网关本机：<code class="mono rounded border border-border bg-muted px-1 py-0.5 text-[0.72rem]">wbapi key list</code>
              或直接看 <code class="mono rounded border border-border bg-muted px-1 py-0.5 text-[0.72rem]">keys.json</code>
            </p>
            <p>改了 admin.token 的话，用那个；没配则回落到 keys.json 第一个 key。</p>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
