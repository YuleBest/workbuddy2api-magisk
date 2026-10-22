<script setup lang="ts">
// 请求日志：持久化的最近 500 条 chat 请求，服务重启后保留。
// 与 stdout 的表格日志同源，这里多了筛选与排序的便利。
import { computed, ref } from 'vue'
import { Inbox, Search, Trash2 } from '@lucide/vue'
import { api, type RequestRecord } from '../api'
import { guard } from '../store'
import { usePolling } from '../composables/usePolling'
import PageHead from '../components/PageHead.vue'
import Panel from '../components/Panel.vue'
import StatusBadge from '../components/StatusBadge.vue'
import EmptyState from '../components/EmptyState.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ago, clock, cost, creditRate, ms, n, tokens } from '../format'

const rows = ref<RequestRecord[]>([])
const total = ref(0)
const capacity = ref(0)
const limit = ref(200)
const filter = ref('')
const modelFilter = ref('all')
const accountFilter = ref('all')
const statusFilter = ref('all')

usePolling(async () => {
  const res = await guard(() => api.logs(limit.value))
  if (res) {
    rows.value = res.records
    total.value = res.total
    capacity.value = res.capacity
  }
})

const limitValue = computed({
  get: () => String(limit.value),
  set: (value: string) => {
    limit.value = Number(value)
  },
})

const modelOptions = computed(() => [...new Set(rows.value.map((r) => r.model))].sort())

const accountOptions = computed(() => {
  const seen = new Map<string, string>()
  for (const r of rows.value) seen.set(accountKey(r), accountLabel(r))
  return [...seen.entries()].sort((a, b) => a[1].localeCompare(b[1], 'zh-CN'))
})

const statusOptions = computed(() =>
  [...new Set(rows.value.map((r) => r.status).filter((s) => s > 0))].sort((a, b) => a - b),
)

const shown = computed(() => {
  const q = filter.value.trim().toLowerCase()
  return rows.value.filter((r) => {
    if (modelFilter.value !== 'all' && r.model !== modelFilter.value) return false
    if (accountFilter.value !== 'all' && accountKey(r) !== accountFilter.value) return false
    if (statusFilter.value === 'ok' && !(r.status >= 200 && r.status < 300)) return false
    if (statusFilter.value === 'failed' && r.status > 0 && r.status < 400) return false
    if (statusFilter.value !== 'all' && statusFilter.value !== 'ok' && statusFilter.value !== 'failed') {
      if (String(r.status) !== statusFilter.value) return false
    }
    if (!q) return true
    return (
      r.model.toLowerCase().includes(q) ||
      (r.nick ?? '').toLowerCase().includes(q) ||
      (r.uid ?? '').toLowerCase().includes(q) ||
      String(r.status).includes(q)
    )
  })
})

function accountKey(r: RequestRecord): string {
  return r.uid || r.nick || '-'
}

function accountLabel(r: RequestRecord): string {
  return r.nick || r.uid || '未命名账号'
}

function accountTitle(r: RequestRecord): string {
  return r.nick && r.uid ? `${r.nick}（${r.uid}）` : accountLabel(r)
}

const failedCount = computed(() => rows.value.filter((r) => !(r.status > 0 && r.status < 400)).length)

function toneOf(status: number): 'ok' | 'warn' | 'bad' | 'idle' {
  if (status >= 200 && status < 300) return 'ok'
  if (status >= 400 && status < 500) return 'warn'
  if (status >= 500) return 'bad'
  return 'idle'
}
</script>

<template>
  <div class="view">
    <PageHead
      title="请求日志"
      lede="持久化保留最近 500 条请求，服务重启后仍可查看。更长期留档看服务日志（journalctl -u wbapi-gateway）。"
    >
      <template #actions>
        <span class="num text-xs text-muted-foreground">累计 {{ n(total) }} 次调用</span>
        <Button variant="outline" size="sm" @click="rows = []">
          <Trash2 class="size-3.5" />
          清空视图
        </Button>
      </template>
    </PageHead>

    <Panel title="筛选" class="rise mb-6">
      <template #actions>
        <div class="flex items-center gap-2">
          <span class="text-[0.68rem] text-muted-foreground">条数</span>
          <Select v-model="limitValue">
            <SelectTrigger size="sm" class="w-[4.6rem] text-xs" aria-label="日志条数">
              <SelectValue />
            </SelectTrigger>
            <SelectContent align="end">
              <SelectItem value="50">50</SelectItem>
              <SelectItem value="200">200</SelectItem>
              <SelectItem value="500">500</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </template>

      <div class="flex flex-wrap items-end gap-3">
        <div class="w-[12rem]">
          <span class="mb-1.5 block text-[0.68rem] text-muted-foreground">模型</span>
          <Select v-model="modelFilter">
            <SelectTrigger size="sm" class="w-full text-xs" aria-label="按模型筛选">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部模型</SelectItem>
              <SelectItem v-for="m in modelOptions" :key="m" :value="m">{{ m }}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div class="w-[11rem]">
          <span class="mb-1.5 block text-[0.68rem] text-muted-foreground">账号</span>
          <Select v-model="accountFilter">
            <SelectTrigger size="sm" class="w-full text-xs" aria-label="按账号筛选">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部账号</SelectItem>
              <SelectItem v-for="[key, label] in accountOptions" :key="key" :value="key">{{ label }}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div class="w-[9rem]">
          <span class="mb-1.5 block text-[0.68rem] text-muted-foreground">状态</span>
          <Select v-model="statusFilter">
            <SelectTrigger size="sm" class="w-full text-xs" aria-label="按状态筛选">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部状态</SelectItem>
              <SelectItem value="ok">成功 (2xx)</SelectItem>
              <SelectItem value="failed">失败 (≥400)</SelectItem>
              <SelectItem v-for="s in statusOptions" :key="s" :value="String(s)">{{ s }}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div class="min-w-[14rem] flex-1">
          <span class="mb-1.5 block text-[0.68rem] text-muted-foreground">关键词</span>
          <div class="relative">
            <Search
              class="pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              v-model="filter"
              type="search"
              class="h-8 pl-9"
              placeholder="模型名、账号或状态码"
            />
          </div>
        </div>
      </div>

      <dl class="mt-5 grid grid-cols-3 divide-x divide-border/70 border-t border-border/70 pt-4">
        <div class="pr-4">
          <dt class="text-[0.68rem] tracking-[0.1em] text-muted-foreground">窗口内记录</dt>
          <dd class="num mt-1 font-serif text-[1.35rem] leading-none">
            {{ n(rows.length) }}<span class="ml-1 text-[0.7rem] text-muted-foreground">/ {{ n(capacity) }}</span>
          </dd>
        </div>
        <div class="px-4">
          <dt class="text-[0.68rem] tracking-[0.1em] text-muted-foreground">失败</dt>
          <dd class="num mt-1 font-serif text-[1.35rem] leading-none" :class="failedCount ? 'text-status-bad' : ''">
            {{ n(failedCount) }}
          </dd>
        </div>
        <div class="pl-4">
          <dt class="text-[0.68rem] tracking-[0.1em] text-muted-foreground">当前显示</dt>
          <dd class="num mt-1 font-serif text-[1.35rem] leading-none">{{ n(shown.length) }}</dd>
        </div>
      </dl>
    </Panel>

    <Panel title="记录" note="最新在前 · 已持久化" class="rise rise-1" flush>
      <EmptyState v-if="!shown.length" :icon="Inbox" :title="rows.length ? '没有符合筛选的记录' : '窗口里还没有请求'">
        {{ rows.length ? '换个模型、账号或状态条件。' : '客户端开始调用 /v1/chat/completions 后，这里会逐条出现。' }}
      </EmptyState>

      <Table v-else class="min-w-[58rem] table-fixed [&_td]:px-2 [&_th]:px-2">
        <TableHeader>
          <TableRow>
            <TableHead class="w-12 text-right">#</TableHead>
            <TableHead class="w-20">时间</TableHead>
            <TableHead class="w-48">模型</TableHead>
            <TableHead class="w-14">方式</TableHead>
            <TableHead class="w-20">账号</TableHead>
            <TableHead class="w-16 text-right">状态</TableHead>
            <TableHead class="w-14 text-right">首字</TableHead>
            <TableHead class="w-14 text-right">耗时</TableHead>
            <TableHead class="w-14 text-right">输入</TableHead>
            <TableHead class="w-14 text-right">输出</TableHead>
            <TableHead class="w-14 text-right">积分</TableHead>
            <TableHead class="w-16 text-right">倍率</TableHead>
            <TableHead class="w-16 text-right">tok/s</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="r in shown" :key="r.seq">
            <TableCell class="num text-right text-[0.74rem] text-muted-foreground">{{ r.seq }}</TableCell>
            <TableCell class="num whitespace-nowrap text-[0.78rem]" :title="ago(r.time)">
              {{ clock(r.time) }}
            </TableCell>
            <TableCell class="mono text-[0.74rem]">
              <span class="block truncate" :title="r.model">{{ r.model }}</span>
            </TableCell>
            <TableCell class="text-[0.76rem] text-muted-foreground">
              {{ r.mode === 'stream' ? '流式' : '非流式' }}
            </TableCell>
            <TableCell class="text-[0.76rem]">
              <span class="block truncate" :title="accountTitle(r)">{{ accountLabel(r) }}</span>
            </TableCell>
            <TableCell class="text-right">
              <StatusBadge :tone="toneOf(r.status)">{{ r.status || '—' }}</StatusBadge>
            </TableCell>
            <TableCell class="num text-right text-[0.76rem]">{{ ms(r.ttfb_ms) }}</TableCell>
            <TableCell class="num text-right text-[0.76rem]">{{ ms(r.total_ms) }}</TableCell>
            <TableCell class="num text-right text-[0.76rem]">{{ tokens(r.prompt_tokens) }}</TableCell>
            <TableCell class="num text-right text-[0.76rem]">{{ tokens(r.completion_tokens) }}</TableCell>
            <TableCell class="num text-right text-[0.76rem]">
              {{ r.credit == null ? '—' : cost(r.credit) }}
            </TableCell>
            <TableCell class="num text-right text-[0.76rem]" :title="r.credits || undefined">
              {{ creditRate(r.credits) }}
            </TableCell>
            <TableCell class="num text-right text-[0.76rem]">
              {{ r.tok_per_sec ? r.tok_per_sec.toFixed(1) : '—' }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </Panel>
  </div>
</template>
