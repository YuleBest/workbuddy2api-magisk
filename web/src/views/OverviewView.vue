<script setup lang="ts">
// 总览：一眼回答"网关现在能不能接活、为什么不能"。
// 页面顺序按运维实际排查顺序排：能不能用 → 流量状况 → 定时任务 → 最近发生了什么。
import { computed, ref } from 'vue'
import {
  Activity,
  ArrowRight,
  Boxes,
  CalendarCheck,
  CircleAlert,
  KeyRound,
  Plane,
  RefreshCw,
} from '@lucide/vue'
import { api, type CheckinOutcome, type Overview, type TaskResult } from '../api'
import { guard, toast } from '../store'
import { usePolling } from '../composables/usePolling'
import PageHead from '../components/PageHead.vue'
import Panel from '../components/Panel.vue'
import StatusLamp from '../components/StatusLamp.vue'
import StatusBadge from '../components/StatusBadge.vue'
import EmptyState from '../components/EmptyState.vue'
import RecentRequests from '../components/RecentRequests.vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { countdown, duration, ms, n, tokens } from '../format'

const data = ref<Overview | null>(null)
const busyTask = ref('')
// 签到回执：同步等待的任务把逐号结果留在页面上，直到用户关掉或再点一次。
const checkinResult = ref<TaskResult | null>(null)

const load = async () => {
  const res = await guard(() => api.overview())
  if (res) data.value = res
}
usePolling(load)

const pool = computed(() => data.value?.pool)
const req = computed(() => data.value?.requests)

const verdict = computed(() => {
  const p = pool.value
  if (!p) return { tone: 'idle' as const, text: '读取中…', detail: '' }
  if (p.servable) return { tone: 'ok' as const, text: '正在接活', detail: `${p.healthy} 个账号可调度` }
  if (p.healthy > 0) return { tone: 'warn' as const, text: '暂时排不上', detail: `${p.healthy} 个账号在途已满` }
  return { tone: 'bad' as const, text: '无法接活', detail: p.cooling > 0 ? `${p.cooling} 个账号在冷却` : '没有可用账号' }
})

const modelBars = computed(() => {
  const models = req.value?.models ?? []
  const top = models.slice(0, 6)
  const max = top.length > 0 ? top[0].count : 1
  return top.map((m) => ({ ...m, pct: Math.round((m.count / max) * 100) }))
})

const tasks = computed(() => {
  const s = data.value?.schedule
  if (!s) return []
  return [
    {
      key: 'checkin',
      name: '签到',
      hint: '签到并顺带查余额，冷却中的账号余额恢复即解冻',
      icon: CalendarCheck,
      tv: s.checkin,
    },
    { key: 'travel', name: '猫猫旅行', hint: '领养 / 派出 / 领奖一趟闭环', icon: Plane, tv: s.travel },
    {
      key: 'activity',
      name: '活跃上报',
      hint: '点亮连登、补满领猫所需对话次数',
      icon: Activity,
      tv: s.activity,
    },
    {
      key: 'keepalive',
      name: 'token 保活',
      hint: '提前刷新凭证，避免到期后首个请求失败',
      icon: KeyRound,
      tv: s.keepalive,
    },
  ]
})

async function runTask(key: string) {
  busyTask.value = key
  // 签到同步等结果（秒级）；其余任务在后台跑，只能给一句"已触发"。
  const res = await guard(() => api.runTask(key, key === 'checkin'))
  busyTask.value = ''
  if (!res) return
  toast(res.message, res.summary && res.summary.fail > 0 ? 'warn' : 'ok')
  if (key === 'checkin') {
    checkinResult.value = res
    void load() // 签到会刷新余额并可能解冻账号，顺手把总览拉新
  }
}

const OUTCOME_LABEL: Record<CheckinOutcome['status'], string> = {
  ok: '签到成功',
  already: '今天已签到',
  fail: '失败',
  skipped: '跳过',
}

function outcomeTone(status: CheckinOutcome['status']) {
  return status === 'ok' ? 'ok' : status === 'already' ? 'outline' : status === 'fail' ? 'bad' : 'warn'
}

function stateTone(state: string) {
  return state === 'healthy' ? 'ok' : state === 'cooling' ? 'warn' : 'bad'
}

function stateWord(state: string) {
  switch (state) {
    case 'healthy':
      return '可用'
    case 'cooling':
      return '冷却'
    case 'manual':
      return '已停用'
    default:
      return '系统禁用'
  }
}

function hoursText(hours: number[]) {
  return hours.map((h) => `${h}:00`).join(' / ')
}
</script>

<template>
  <div class="view">
    <PageHead
      title="总览"
      lede="网关当前的接活能力、账号池分布与最近发生的请求。数据来自网关进程本身，页面每几秒自动刷新。"
    >
      <template #actions>
        <span v-if="data" class="num text-xs text-muted-foreground">已运行 {{ duration(data.uptime_sec) }}</span>
        <Button variant="outline" size="sm" @click="load">
          <RefreshCw class="size-3.5" />
          立即刷新
        </Button>
      </template>
    </PageHead>

    <Alert v-if="data?.degraded" class="mb-5 border-l-status-warn">
      <CircleAlert class="size-4" />
      <AlertTitle>提示词降级期</AlertTitle>
      <AlertDescription>
        passthrough 模式下有请求被上游内容策略拦下，网关已切换到中性提示词直到次日 00:00。
        若此时仍有大量失败，检查是不是请求内容本身触发了审核。
      </AlertDescription>
    </Alert>
    <Alert v-else-if="data && !data.pool.servable" class="mb-5 border-l-status-bad">
      <CircleAlert class="size-4" />
      <AlertTitle>当前没有账号能接活</AlertTitle>
      <AlertDescription>
        冷却会自行到期，禁用需要人工处理（凭证失效要重新走 <code>./login.sh</code>）。
        去「账号池」看看是哪个账号、什么原因。
      </AlertDescription>
    </Alert>

    <Panel
      class="rise mb-6"
      tone="ink"
      title="接活状态"
      :note="data ? `${data.listen}${data.sticky_sessions ? ` · 粘性会话 ${n(data.sticky_sessions)} 个` : ''}` : ''"
    >
      <template #actions>
        <Button
          variant="ghost"
          size="sm"
          class="text-[color:var(--inverse-fg)] hover:bg-white/10 hover:text-[color:var(--inverse-fg)]"
          @click="load"
        >
          <RefreshCw class="size-3.5" />
          刷新读数
        </Button>
      </template>

      <div class="grid gap-8 lg:grid-cols-[1.15fr_0.85fr] lg:gap-10">
        <div>
          <div class="flex items-start gap-3.5">
            <StatusLamp :state="verdict.tone" :live="pool?.servable" class="mt-2.5 scale-125" />
            <div>
              <h2 class="font-serif text-[2.1rem] leading-[1.1] tracking-[-0.02em] sm:text-[2.5rem]">
                {{ verdict.text }}
              </h2>
              <p class="mt-1.5 text-[0.8rem] ink-muted">{{ verdict.detail }}</p>
            </div>
          </div>

          <dl class="mt-7 grid grid-cols-2 gap-x-6 gap-y-5 sm:grid-cols-4">
            <div>
              <dt class="text-[0.68rem] tracking-[0.12em] ink-muted">账号总数</dt>
              <dd class="num mt-1 font-serif text-[1.7rem] leading-none">{{ n(pool?.total) }}</dd>
            </div>
            <div>
              <dt class="text-[0.68rem] tracking-[0.12em] ink-muted">可调度</dt>
              <dd class="num mt-1 font-serif text-[1.7rem] leading-none text-status-ok">
                {{ n(pool?.healthy) }}
              </dd>
            </div>
            <div>
              <dt class="text-[0.68rem] tracking-[0.12em] ink-muted">冷却中</dt>
              <dd class="num mt-1 font-serif text-[1.7rem] leading-none text-status-warn">
                {{ n(pool?.cooling) }}
              </dd>
            </div>
            <div>
              <dt class="text-[0.68rem] tracking-[0.12em] ink-muted">已禁用</dt>
              <dd
                class="num mt-1 font-serif text-[1.7rem] leading-none"
                :class="pool?.disabled ? 'text-status-bad' : ''"
              >
                {{ n(pool?.disabled) }}
              </dd>
            </div>
          </dl>
        </div>

        <div class="border-t border-[color:var(--inverse-line)] pt-6 lg:border-t-0 lg:border-l lg:pt-0 lg:pl-9">
          <div class="flex items-baseline justify-between gap-4">
            <h3 class="font-serif text-[0.95rem]">账号分布</h3>
            <span class="mono text-[0.62rem] ink-muted">在途 {{ n(pool?.in_flight) }}</span>
          </div>

          <div v-if="data?.accounts.length" class="mt-4 flex flex-col gap-1.5">
            <button
              v-for="a in data.accounts"
              :key="a.uid"
              type="button"
              class="group flex items-center gap-2.5 rounded-md border border-[color:var(--inverse-line)] px-3 py-2 text-left transition-colors hover:border-[color:var(--inverse-accent)]"
              :title="`${a.nickname || a.uid} — ${stateWord(a.state)}${a.reason ? '：' + a.reason : ''}`"
              @click="$router.push('/accounts')"
            >
              <StatusLamp :state="stateTone(a.state)" :live="a.state === 'healthy'" />
              <span class="min-w-0 flex-1 truncate text-[0.78rem]">
                {{ a.nickname || a.uid.slice(0, 8) }}
                <span v-if="a.realm === 'global'" class="text-[0.74rem] ink-muted">（国际版）</span>
              </span>
              <span class="text-[0.72rem] ink-muted">{{ stateWord(a.state) }}</span>
              <span class="num mono w-16 text-right text-[0.72rem] ink-muted">
                {{ a.state === 'cooling' ? countdown(a.cool_remaining) : `${n(a.credits)} 分` }}
              </span>
            </button>
          </div>
          <div
            v-else
            class="mt-4 rounded-md border border-dashed border-[color:var(--inverse-line)] px-4 py-5 text-[0.75rem] leading-relaxed ink-muted"
          >
            <strong class="mb-1 block font-serif text-[0.85rem] text-[color:var(--inverse-fg)]">
              账号池是空的
            </strong>
            在网关主机上执行 <code class="mono">./login.sh</code> 走一次设备授权，凭据落进
            <code class="mono">auths/</code> 后这里就会出现账号。
          </div>
        </div>
      </div>
    </Panel>

    <div class="mb-6 grid gap-6 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
      <Panel title="请求流量" :note="`最近 ${n(req?.window)} 条记录`" class="rise rise-1">
        <dl class="divide-y divide-border/70 text-[0.8rem]">
          <div class="flex items-baseline justify-between gap-4 py-2.5 first:pt-0 last:pb-0">
            <dt class="text-muted-foreground">进程累计请求</dt>
            <dd class="num font-medium">{{ n(req?.total) }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 py-2.5 first:pt-0 last:pb-0">
            <dt class="text-muted-foreground">最近一小时</dt>
            <dd class="num font-medium">{{ n(req?.last_hour) }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 py-2.5 first:pt-0 last:pb-0">
            <dt class="text-muted-foreground">窗口内失败</dt>
            <dd class="num font-medium" :class="req?.error ? 'text-status-bad' : ''">
              {{ n(req?.error) }} / {{ n((req?.ok ?? 0) + (req?.error ?? 0)) }}
            </dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 py-2.5 first:pt-0 last:pb-0">
            <dt class="text-muted-foreground">平均首字延迟</dt>
            <dd class="num font-medium">{{ ms(req?.avg_ttfb_ms) }}</dd>
          </div>
          <div class="flex items-baseline justify-between gap-4 py-2.5 first:pt-0 last:pb-0">
            <dt class="text-muted-foreground">平均输出 token</dt>
            <dd class="num font-medium">{{ tokens(req?.avg_tokens ?? 0) }}</dd>
          </div>
        </dl>
      </Panel>

      <Panel title="模型分布" note="窗口内请求数" class="rise rise-2">
        <div v-if="modelBars.length" class="space-y-4">
          <div v-for="(m, i) in modelBars" :key="m.model">
            <div class="flex items-baseline gap-3">
              <span class="mono w-5 text-[0.68rem] text-muted-foreground">{{ String(i + 1).padStart(2, '0') }}</span>
              <span class="mono min-w-0 flex-1 truncate text-[0.72rem]">{{ m.model }}</span>
              <span class="num text-[0.72rem] text-muted-foreground">{{ n(m.count) }}</span>
            </div>
            <div class="ml-8 mt-1.5 h-1.5 overflow-hidden rounded-full bg-muted">
              <div class="h-full rounded-full bg-primary/75" :style="{ width: `${m.pct}%` }"></div>
            </div>
          </div>
        </div>
        <EmptyState v-else :icon="Boxes" title="窗口里还没有请求">
          客户端接入网关后，这里会按模型统计调用量。接口地址是
          <code class="mono">/v1/chat/completions</code>。
        </EmptyState>
      </Panel>
    </div>

    <Panel
      title="定时任务"
      note="在配置的整点自动执行；下面的按钮可以立刻跑一次"
      class="rise rise-2 mb-6"
      flush
    >
      <template #actions>
        <code class="mono rounded border border-border bg-muted px-2 py-1 text-[0.7rem] text-muted-foreground">
          journalctl -u wbapi-gateway -f
        </code>
      </template>

      <div v-for="t in tasks" :key="t.key" class="border-b border-border/70 last:border-0">
        <div class="grid gap-3 px-5 py-4 sm:grid-cols-[minmax(10rem,1fr)_minmax(0,1.5fr)_auto] sm:items-center">
          <div class="flex items-center gap-3">
            <StatusLamp :state="t.tv.enabled ? 'ok' : 'idle'" :live="t.tv.enabled" />
            <div class="min-w-0">
              <p class="flex items-center gap-1.5 font-serif text-[0.95rem] leading-tight">
                <component :is="t.icon" class="size-3.5 text-muted-foreground" aria-hidden="true" />
                {{ t.name }}
              </p>
              <p class="mono mt-1 text-[0.7rem] text-muted-foreground">
                <template v-if="t.tv.enabled">每天 {{ hoursText(t.tv.hours) }}</template>
                <template v-else>已在配置里关闭</template>
              </p>
            </div>
          </div>
          <p class="text-[0.78rem] leading-relaxed text-muted-foreground">{{ t.hint }}</p>
          <div class="sm:text-right">
            <Button
              variant="outline"
              size="sm"
              :disabled="busyTask === t.key"
              @click="runTask(t.key)"
            >
              {{ busyTask === t.key ? (t.key === 'checkin' ? '签到中…' : '已触发') : '立即执行' }}
            </Button>
          </div>
        </div>
      </div>

      <div v-if="checkinResult" class="border-t border-border bg-muted/25 px-5 py-4">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
          <strong class="font-serif text-[0.9rem]">{{ checkinResult.message }}</strong>
          <Button variant="ghost" size="sm" @click="checkinResult = null">收起</Button>
        </div>
        <p v-if="checkinResult.waited === false" class="mb-3 text-[0.78rem] text-muted-foreground">
          这次没等到结果就返回了：签到还在后台跑，稍后看账号池的积分变化。
        </p>
        <div v-if="checkinResult.outcomes?.length" class="overflow-x-auto rounded-md border border-border">
          <Table class="min-w-[34rem]">
            <TableHeader>
              <TableRow>
                <TableHead>账号</TableHead>
                <TableHead>结果</TableHead>
                <TableHead class="text-right">签到后余额</TableHead>
                <TableHead>说明</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="oc in checkinResult.outcomes" :key="oc.uid">
                <TableCell>
                  <div class="text-[0.8rem]">{{ oc.nickname || '未命名账号' }}</div>
                  <div class="mono text-[0.7rem] text-muted-foreground">{{ oc.uid.slice(0, 8) }}</div>
                </TableCell>
                <TableCell>
                  <StatusBadge :tone="outcomeTone(oc.status)">{{ OUTCOME_LABEL[oc.status] }}</StatusBadge>
                </TableCell>
                <TableCell class="num text-right">{{ oc.credits === undefined ? '—' : n(oc.credits) }}</TableCell>
                <TableCell class="text-[0.78rem] text-muted-foreground">{{ oc.detail || '—' }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </div>
    </Panel>

    <Panel title="最近请求" class="rise rise-3" flush>
      <template #actions>
        <Button as-child variant="outline" size="sm">
          <RouterLink to="/logs">
            查看完整日志
            <ArrowRight class="size-3.5" />
          </RouterLink>
        </Button>
      </template>
      <RecentRequests :limit="8" />
    </Panel>
  </div>
</template>
