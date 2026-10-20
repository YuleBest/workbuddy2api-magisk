<script setup lang="ts">
// 最近请求（紧凑表）：总览页的"刚刚发生了什么"。
// 只读日志接口，和日志页共用同一数据源；样式更省，不展示模型分布等细节。
import { ref } from 'vue'
import { Inbox } from '@lucide/vue'
import { api, type RequestRecord } from '../api'
import { guard } from '../store'
import { usePolling } from '../composables/usePolling'
import { clock, ms, tokens } from '../format'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import StatusBadge from './StatusBadge.vue'
import EmptyState from './EmptyState.vue'

const props = withDefaults(defineProps<{ limit?: number }>(), { limit: 8 })

const rows = ref<RequestRecord[]>([])
const total = ref(0)

usePolling(async () => {
  const res = await guard(() => api.logs(props.limit))
  if (res) {
    rows.value = res.records
    total.value = res.total
  }
})

function toneOf(status: number): 'ok' | 'warn' | 'bad' | 'idle' {
  if (status >= 200 && status < 300) return 'ok'
  if (status >= 400 && status < 500) return 'warn'
  if (status >= 500) return 'bad'
  return 'idle'
}
</script>

<template>
  <Table v-if="rows.length" class="min-w-[38rem]">
    <TableHeader>
      <TableRow>
        <TableHead>时间</TableHead>
        <TableHead>模型</TableHead>
        <TableHead>账号</TableHead>
        <TableHead>方式</TableHead>
        <TableHead class="text-right">状态</TableHead>
        <TableHead class="text-right">首字</TableHead>
        <TableHead class="text-right">token</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      <TableRow v-for="r in rows" :key="r.seq">
        <TableCell class="num text-xs text-muted-foreground">{{ clock(r.time) }}</TableCell>
        <TableCell class="mono text-xs">{{ r.model }}</TableCell>
        <TableCell class="text-xs">
          {{ r.nick || '未命名账号' }}
        </TableCell>
        <TableCell class="text-xs text-muted-foreground">
          {{ r.mode === 'stream' ? '流式' : '非流式' }}
        </TableCell>
        <TableCell class="text-right">
          <StatusBadge :tone="toneOf(r.status)">{{ r.status || '—' }}</StatusBadge>
        </TableCell>
        <TableCell class="num text-right text-xs">{{ ms(r.ttfb_ms) }}</TableCell>
        <TableCell class="num text-right text-xs">{{ tokens(r.tokens) }}</TableCell>
      </TableRow>
    </TableBody>
  </Table>
  <EmptyState v-else :icon="Inbox" title="还没有请求记录">
    累计 {{ total }} 次调用。客户端请求打进来后，这里会逐条滚动并持久化。
  </EmptyState>
</template>
