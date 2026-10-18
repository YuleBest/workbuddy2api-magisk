<script setup lang="ts">
// 最近请求（紧凑表）：总览页的"刚刚发生了什么"。
// 只读日志接口，和日志页共用同一数据源，样式更省：不显示模型分布等细节。
import { ref } from 'vue'
import { api, type RequestRecord } from '../api'
import { guard } from '../store'
import { usePolling } from '../composables/usePolling'
import { clock, ms, statusClass, tokens } from '../format'

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
</script>

<template>
  <div v-if="rows.length" class="table-scroll">
    <table class="data recent">
      <thead>
        <tr>
          <th>时间</th>
          <th>模型</th>
          <th>账号</th>
          <th>方式</th>
          <th class="n">状态</th>
          <th class="n">首字</th>
          <th class="n">token</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.seq">
          <td class="num">{{ clock(r.time) }}</td>
          <td class="mono small">{{ r.model }}</td>
          <td class="small">
            <span>{{ r.nick || '—' }}</span>
            <span class="mono muted">（{{ r.uid }}）</span>
          </td>
          <td class="small muted">{{ r.mode === 'stream' ? '流式' : '非流式' }}</td>
          <td class="n">
            <span class="tag" :class="statusClass(r.status)">{{ r.status || '—' }}</span>
          </td>
          <td class="n">{{ ms(r.ttfb_ms) }}</td>
          <td class="n">{{ tokens(r.tokens) }}</td>
        </tr>
      </tbody>
    </table>
  </div>
  <div v-else class="empty">
    <strong>这个进程还没处理过请求</strong>
    累计 {{ total }} 次调用。客户端请求打进来后，这里会逐条滚动。
  </div>
</template>