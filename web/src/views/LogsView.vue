<script setup lang="ts">
// 请求日志：网关进程内最近若干条 chat 请求（内存环形缓冲，重启清零）。
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
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ago, clock, ms, n, tokens } from '../format'

const rows = ref<RequestRecord[]>([])
const total = ref(0)
const capacity = ref(0)
const limit = ref(200)
const filter = ref('')
const onlyFailed = ref(false)

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

const shown = computed(() => {
  const q = filter.value.trim().toLowerCase()
  return rows.value.filter((r) => {
    if (onlyFailed.value && r.status > 0 && r.status < 400) return false
    if (!q) return true
    return (
      r.model.toLowerCase().includes(q) ||
      r.uid.toLowerCase().includes(q) ||
      String(r.status).includes(q)
    )
  })
})

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
      lede="网关进程内存里的最近请求记录，重启即清空。要长期留档就看服务日志（journalctl -u wbapi-gateway）。"
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
          <Checkbox id="only-failed" v-model="onlyFailed" />
          <Label for="only-failed" class="text-[0.72rem]">只看失败</Label>
        </div>
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

      <div class="relative max-w-xl">
        <Search
          class="pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          v-model="filter"
          type="search"
          class="pl-9"
          placeholder="按模型名、账号 uid 前 8 位或状态码筛选"
        />
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

    <Panel title="记录" note="最新在前" class="rise rise-1" flush>
      <EmptyState v-if="!shown.length" :icon="Inbox" :title="rows.length ? '没有符合筛选的记录' : '窗口里还没有请求'">
        {{ rows.length ? '换个关键词，或取消「只看失败」。' : '客户端开始调用 /v1/chat/completions 后，这里会逐条出现。' }}
      </EmptyState>

      <Table v-else class="min-w-[54rem]">
        <TableHeader>
          <TableRow>
            <TableHead class="text-right">#</TableHead>
            <TableHead>时间</TableHead>
            <TableHead>模型</TableHead>
            <TableHead>方式</TableHead>
            <TableHead>账号</TableHead>
            <TableHead class="text-right">状态</TableHead>
            <TableHead class="text-right">首字</TableHead>
            <TableHead class="text-right">耗时</TableHead>
            <TableHead class="text-right">token</TableHead>
            <TableHead class="text-right">tok/s</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="r in shown" :key="r.seq">
            <TableCell class="num text-right text-[0.74rem] text-muted-foreground">{{ r.seq }}</TableCell>
            <TableCell class="num whitespace-nowrap text-[0.78rem]" :title="ago(r.time)">
              {{ clock(r.time) }}
            </TableCell>
            <TableCell class="mono text-[0.74rem]">{{ r.model }}</TableCell>
            <TableCell class="text-[0.76rem] text-muted-foreground">
              {{ r.mode === 'stream' ? '流式' : '非流式' }}
            </TableCell>
            <TableCell class="text-[0.76rem]">
              <span>{{ r.nick || '—' }}</span>
              <span class="mono ml-1 text-[0.68rem] text-muted-foreground">（{{ r.uid }}）</span>
            </TableCell>
            <TableCell class="text-right">
              <StatusBadge :tone="toneOf(r.status)">{{ r.status || '—' }}</StatusBadge>
            </TableCell>
            <TableCell class="num text-right text-[0.76rem]">{{ ms(r.ttfb_ms) }}</TableCell>
            <TableCell class="num text-right text-[0.76rem]">{{ ms(r.total_ms) }}</TableCell>
            <TableCell class="num text-right text-[0.76rem]">{{ tokens(r.tokens) }}</TableCell>
            <TableCell class="num text-right text-[0.76rem]">
              {{ r.tok_per_sec ? r.tok_per_sec.toFixed(1) : '—' }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </Panel>
  </div>
</template>
