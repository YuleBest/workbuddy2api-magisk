<script setup lang="ts">
// 账号池：每个账号一条"仪表条"——状态灯、积分余量、冷却倒计时、在途请求、
// 以及可执行的动作（停用/恢复/复活/清冷却）。展开可见成本账本与凭证信息。
import { computed, onUnmounted, ref } from 'vue'
import {
  ArrowLeft,
  Ban,
  ChevronDown,
  ChevronUp,
  Clock,
  Power,
  RotateCcw,
  Snowflake,
  Users,
} from '@lucide/vue'
import { api, type Account } from '../api'
import { guard, toast } from '../store'
import { usePolling } from '../composables/usePolling'
import PageHead from '../components/PageHead.vue'
import Panel from '../components/Panel.vue'
import StatusLamp from '../components/StatusLamp.vue'
import StatusBadge from '../components/StatusBadge.vue'
import CreditBar from '../components/CreditBar.vue'
import EmptyState from '../components/EmptyState.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { ago, coolReason, countdown, cost, duration, n } from '../format'

const accounts = ref<Account[]>([])
const expanded = ref<Record<string, boolean>>({})
const pending = ref('')

// 冷却倒计时本地每秒走一格：轮询间隔 5 秒时，秒数也要看起来在动。
const nowTick = ref(Date.now())
const lastSync = ref(Date.now())
const timer = window.setInterval(() => (nowTick.value = Date.now()), 1000)
onUnmounted(() => window.clearInterval(timer))

usePolling(async () => {
  const res = await guard(() => api.accounts())
  if (res) {
    accounts.value = res.accounts
    lastSync.value = Date.now() // 倒计时基准与轮询对齐，避免本地秒表跑偏
  }
})

const maxCredits = computed(() => accounts.value.reduce((m, a) => Math.max(m, a.credits), 0))

/** 冷却剩余秒数：以后端返回的剩余秒数为基准，叠加本地已过的秒数。 */
const remainOf = (a: Account) => {
  const base = a.cool_remaining_sec ?? 0
  if (base <= 0) return 0
  const drift = Math.floor((nowTick.value - lastSync.value) / 1000)
  return Math.max(0, base - drift)
}

function tone(a: Account) {
  if (a.disabled || a.manual_disabled) return 'bad'
  return a.cooling ? 'warn' : 'ok'
}

function word(a: Account) {
  if (a.manual_disabled && a.disabled) return '已停用 + 系统禁用'
  if (a.manual_disabled) return '已停用'
  if (a.disabled) return '系统禁用'
  return a.cooling ? '冷却中' : '可用'
}

const disableTarget = ref<Account | null>(null)
const disableReason = ref('手动排查')
const disableOpen = computed({
  get: () => disableTarget.value !== null,
  set: (open: boolean) => {
    if (!open) disableTarget.value = null
  },
})

function askDisable(a: Account) {
  disableTarget.value = a
  disableReason.value = '手动排查'
}

async function confirmDisable() {
  const a = disableTarget.value
  if (!a) return
  pending.value = a.uid + 'disable'
  const res = await guard(() => api.accountAction(a.uid, 'disable', disableReason.value.trim()))
  pending.value = ''
  if (res) {
    disableTarget.value = null
    toast(
      res.disabled
        ? `已停用 ${a.nickname || a.uid.slice(0, 8)}；注意它还被系统禁用着，要另外点"复活"`
        : `已停用 ${a.nickname || a.uid.slice(0, 8)}，不再承接对话流量`,
    )
  }
}

async function act(a: Account, action: 'enable' | 'revive' | 'clear-cooldown') {
  pending.value = a.uid + action
  const res = await guard(() => api.accountAction(a.uid, action))
  pending.value = ''
  if (!res) return
  if (action === 'enable') {
    toast(
      res.disabled ? '手动停用已解除，但系统禁用位还在——需要点"复活"' : '已放回池子，下一个请求就可能选中它',
      res.disabled ? 'warn' : 'ok',
    )
  } else if (action === 'revive') {
    toast('系统禁用位已清除，账号回到选号池')
  } else {
    toast('冷却已清除')
  }
}

function tokenState(a: Account) {
  if (!a.expires_in_sec) return { text: '过期时间未知', tone: 'warn' as const }
  if (a.expires_in_sec <= 0) return { text: '凭证已过期，下次使用会先刷新', tone: 'bad' as const }
  if (a.expires_in_sec < 600) return { text: `${duration(a.expires_in_sec)}后过期`, tone: 'warn' as const }
  return { text: `${duration(a.expires_in_sec)}后过期`, tone: 'ok' as const }
}
</script>

<template>
  <div class="view">
    <PageHead
      title="账号池"
      lede="网关按积分余量、闲置时长、成功率加权挑号。这里的动作直接改运行中的池状态，不需要重启服务。"
    >
      <template #actions>
        <span class="num text-xs text-muted-foreground">{{ accounts.length }} 个账号</span>
        <Button as-child variant="outline" size="sm">
          <RouterLink to="/overview">
            <ArrowLeft class="size-3.5" />
            回总览
          </RouterLink>
        </Button>
      </template>
    </PageHead>

    <Panel
      title="池内账号"
      note="积分来自上次签到查询，冷却与熔断为实时状态"
      class="rise"
      flush
    >
      <EmptyState v-if="!accounts.length" :icon="Users" title="还没有账号">
        在网关主机上跑 <code class="mono">./login.sh</code> 完成一次设备授权登录，凭据会自动落进
        <code class="mono">auths/</code> 并被池子接管。
        <template #action>
          <Button as-child variant="outline" size="sm">
            <RouterLink to="/overview">看总览</RouterLink>
          </Button>
        </template>
      </EmptyState>

      <article
        v-for="a in accounts"
        :key="a.uid"
        class="border-b border-l-2 border-border/70 px-5 py-4 last:border-b-0 sm:px-6"
        :class="
          a.disabled
            ? 'border-l-status-bad'
            : a.cooling
              ? 'border-l-status-warn'
              : 'border-l-transparent'
        "
      >
        <div class="grid gap-4 sm:grid-cols-[minmax(0,1.2fr)_minmax(0,0.9fr)_auto] sm:items-center">
          <div class="flex min-w-0 items-start gap-3">
            <StatusLamp :state="tone(a)" :live="!a.disabled && !a.cooling" class="mt-1.5" />
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="font-serif text-[1.02rem] leading-tight">{{ a.nickname || '未命名账号' }}</h3>
                <StatusBadge v-if="a.realm" tone="outline">
                  {{ a.realm === 'global' ? '国际版' : '国内版' }}
                </StatusBadge>
                <StatusBadge :tone="tone(a)">{{ word(a) }}</StatusBadge>
              </div>
              <p class="mono mt-1 truncate text-[0.7rem] text-muted-foreground">{{ a.uid }}</p>
            </div>
          </div>

          <CreditBar :value="a.credits" :max="maxCredits" />

          <div class="flex flex-wrap items-center gap-2 sm:justify-end">
            <Button
              v-if="!a.manual_disabled"
              variant="outline"
              size="sm"
              :disabled="pending === a.uid + 'disable'"
              title="把账号摘出选号池，但保留在池里：签到、保活照常，凭证和积分都是活的"
              @click="askDisable(a)"
            >
              <Ban class="size-3.5" />
              停用
            </Button>
            <Button
              v-else
              size="sm"
              :disabled="pending === a.uid + 'enable'"
              title="解除手动停用"
              @click="act(a, 'enable')"
            >
              <Power class="size-3.5" />
              恢复
            </Button>
            <Button
              v-if="a.disabled"
              variant="outline"
              size="sm"
              :disabled="pending === a.uid + 'revive'"
              title="清除系统自动禁用位（12153 / 连续失败）"
              @click="act(a, 'revive')"
            >
              <RotateCcw class="size-3.5" />
              复活
            </Button>
            <Button
              v-if="a.cooling && !a.disabled"
              variant="outline"
              size="sm"
              :disabled="pending === a.uid + 'clear-cooldown'"
              title="清掉冷却后，下一个请求可能立刻选中它"
              @click="act(a, 'clear-cooldown')"
            >
              <Snowflake class="size-3.5" />
              清冷却
            </Button>
            <Button variant="ghost" size="sm" @click="expanded[a.uid] = !expanded[a.uid]">
              {{ expanded[a.uid] ? '收起' : '详情' }}
              <ChevronUp v-if="expanded[a.uid]" class="size-3.5" />
              <ChevronDown v-else class="size-3.5" />
            </Button>
          </div>
        </div>

        <div
          class="mt-3 flex flex-wrap items-center gap-x-5 gap-y-1.5 text-[0.75rem] leading-relaxed text-muted-foreground sm:pl-6"
        >
          <span class="inline-flex items-center gap-1.5">
            <Clock class="size-3.5" aria-hidden="true" />
            <template v-if="a.cooling && !a.disabled">
              <span class="num text-status-warn">{{ countdown(remainOf(a)) }}</span>
              后冷却到期
              <template v-if="a.reason">（{{ coolReason(a.reason) }}）</template>
              <template v-if="a.soft_streak && a.soft_streak > 1">，第 {{ a.soft_streak }} 次</template>
            </template>
            <template v-else>最近成功 {{ ago(a.last_success) }}</template>
          </span>
          <span v-if="a.manual_disabled" class="text-status-bad">
            手动停用<template v-if="a.manual_reason">：{{ a.manual_reason }}</template>
          </span>
          <span v-if="a.disabled" class="text-status-bad">
            系统禁用<template v-if="a.disabled_reason">：{{ a.disabled_reason }}</template>
          </span>
          <span v-if="a.rate_limited_models?.length">
            限额模型
            <span class="mono text-foreground">{{ a.rate_limited_models.map((m) => m.model).join(', ') }}</span>
          </span>
          <span v-if="a.degrade_until && new Date(a.degrade_until) > new Date()" class="text-status-warn">
            连败降权中（连续失败 {{ a.consecutive_fails }} 次）
          </span>
          <span>在途 <span class="num text-foreground">{{ a.in_flight }}</span></span>
          <span v-if="a.breaker_until && new Date(a.breaker_until) > new Date()" class="text-status-warn">
            熔断中，连续失败 {{ a.breaker_fails }} 次
          </span>
          <span>
            成功 <span class="num text-foreground">{{ n(a.success_count) }}</span>
            <template v-if="a.err_total">，失败 <span class="num text-foreground">{{ n(a.err_total) }}</span></template>
          </span>
        </div>

        <div
          v-if="expanded[a.uid]"
          class="mt-4 rounded-md border border-border bg-muted/35 px-4 py-3 sm:ml-6"
        >
          <dl class="grid gap-x-6 gap-y-2.5 text-[0.78rem] sm:grid-cols-[9rem_minmax(0,1fr)]">
            <dt class="text-muted-foreground">凭证状态</dt>
            <dd :class="tokenState(a).tone === 'bad' ? 'text-status-bad' : tokenState(a).tone === 'warn' ? 'text-status-warn' : ''">
              {{ tokenState(a).text }}
            </dd>
            <dt class="text-muted-foreground">设备 token</dt>
            <dd>{{ a.has_device_token ? '已配置' : '未配置（部分上游校验会因此更严）' }}</dd>
            <dt class="text-muted-foreground">最近错误</dt>
            <dd>{{ ago(a.last_err) }}</dd>
            <dt class="text-muted-foreground">成本账本</dt>
            <dd class="space-y-1">
              <template v-if="a.model_costs?.length">
                <div v-for="c in a.model_costs" :key="c.model">
                  <span class="mono">{{ c.model }}</span>
                  — 每千 token {{ cost(c.cost_per_1k) }} 积分（{{ c.samples }} 次实测）
                </div>
              </template>
              <span v-else class="text-muted-foreground">暂无观测：该号还没成功跑过带 usage 的请求</span>
            </dd>
          </dl>
        </div>
      </article>
    </Panel>

    <Panel title="状态含义" class="rise rise-1">
      <dl class="grid gap-x-10 gap-y-5 md:grid-cols-2">
        <div>
          <dt class="font-serif text-[0.92rem]">可用</dt>
          <dd class="mt-1 text-[0.78rem] leading-relaxed text-muted-foreground">可以被选中承载请求。</dd>
        </div>
        <div>
          <dt class="font-serif text-[0.92rem]">冷却中</dt>
          <dd class="mt-1 text-[0.78rem] leading-relaxed text-muted-foreground">
            上游限流或当日模型用量用尽，到点自动恢复；清冷却只在确认上游已放行时用。
          </dd>
        </div>
        <div>
          <dt class="font-serif text-[0.92rem]">熔断</dt>
          <dd class="mt-1 text-[0.78rem] leading-relaxed text-muted-foreground">
            连续失败达到阈值后指数退避，成功一次即清零。
          </dd>
        </div>
        <div>
          <dt class="font-serif text-[0.92rem]">已停用</dt>
          <dd class="mt-1 text-[0.78rem] leading-relaxed text-muted-foreground">
            运维主动摘除（手动位）。签到、保活、定时任务照常执行，凭证和积分都是活的；点「恢复」放回池子。
          </dd>
        </div>
        <div>
          <dt class="font-serif text-[0.92rem]">系统禁用</dt>
          <dd class="mt-1 text-[0.78rem] leading-relaxed text-muted-foreground">
            网关判定账号坏了（连续 12153 / 连败）。与手动停用是两个独立状态位，点「复活」清除；
            凭证真失效要重新登录才有救。
          </dd>
        </div>
      </dl>
    </Panel>

    <ConfirmDialog
      v-model:open="disableOpen"
      title="停用账号"
      :description="
        disableTarget
          ? `把 ${disableTarget.nickname || disableTarget.uid.slice(0, 8)} 摘出选号池。签到、保活和凭证都保留。`
          : ''
      "
      confirm-text="停用"
      destructive
      :busy="disableTarget ? pending === disableTarget.uid + 'disable' : false"
      @confirm="confirmDisable"
    >
      <div class="space-y-2">
        <Label for="disable-reason">停用原因</Label>
        <Textarea
          id="disable-reason"
          v-model="disableReason"
          rows="3"
          placeholder="会显示在账号条上，方便以后回想"
        />
      </div>
    </ConfirmDialog>
  </div>
</template>
