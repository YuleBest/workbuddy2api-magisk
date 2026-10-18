<script setup lang="ts">
// 请求日志：网关进程内最近若干条 chat 请求（内存环形缓冲，重启清零）。
// 与 stdout 的表格日志同源，这里多了筛选与排序的便利。
import { computed, ref } from 'vue'
import { api, type RequestRecord } from '../api'
import { guard } from '../store'
import { usePolling } from '../composables/usePolling'
import PageHead from '../components/PageHead.vue'
import { ago, clock, ms, n, statusClass, tokens } from '../format'

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
</script>

<template>
  <div class="view">
    <PageHead
      title="请求日志"
      lede="网关进程内存里的最近请求记录，重启即清空。要长期留档就看服务日志（journalctl -u wbapi-gateway）。"
    >
      <template #actions>
        <span class="muted small num">累计 {{ n(total) }} 次调用</span>
        <button type="button" @click="rows = []">清空视图</button>
      </template>
    </PageHead>

    <section class="panel">
      <div class="panel-head">
        <h2>筛选</h2>
        <span class="spacer"></span>
        <label class="checkline">
          只看失败
          <input v-model="onlyFailed" type="checkbox" />
        </label>
        <label class="checkline">
          条数
          <select v-model.number="limit">
            <option :value="50">50</option>
            <option :value="200">200</option>
            <option :value="500">500</option>
          </select>
        </label>
      </div>
      <div class="panel-body">
        <input
          v-model="filter"
          type="search"
          placeholder="按模型名、账号 uid 前 8 位或状态码筛选"
          style="width: 100%"
        />
      </div>
      <div class="facts">
        <div class="fact">
          <div class="k">窗口内记录</div>
          <div class="v">{{ n(rows.length) }}<small>/ {{ n(capacity) }}</small></div>
        </div>
        <div class="fact">
          <div class="k">失败</div>
          <div class="v" :style="failedCount ? 'color: var(--bad)' : ''">{{ n(failedCount) }}</div>
        </div>
        <div class="fact">
          <div class="k">当前显示</div>
          <div class="v">{{ n(shown.length) }}</div>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>记录</h2>
        <span class="note">最新在前</span>
      </div>

      <div v-if="!shown.length" class="empty">
        <strong>{{ rows.length ? '没有符合筛选的记录' : '窗口里还没有请求' }}</strong>
        {{ rows.length ? '换个关键词，或取消「只看失败」。' : '客户端开始调用 /v1/chat/completions 后，这里会逐条出现。' }}
      </div>

      <div v-else class="table-scroll">
        <table class="data logs">
        <thead>
          <tr>
            <th class="n">#</th>
            <th>时间</th>
            <th>模型</th>
            <th>方式</th>
            <th>账号</th>
            <th class="n">状态</th>
            <th class="n">首字</th>
            <th class="n">耗时</th>
            <th class="n">token</th>
            <th class="n">tok/s</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in shown" :key="r.seq">
            <td class="n muted">{{ r.seq }}</td>
            <td class="num nowrap" :title="ago(r.time)">{{ clock(r.time) }}</td>
            <td class="mono small">{{ r.model }}</td>
            <td class="small muted">{{ r.mode === 'stream' ? '流式' : '非流式' }}</td>
            <td class="small">
              <span>{{ r.nick || '—' }}</span>
              <span class="mono muted">（{{ r.uid }}）</span>
            </td>
            <td class="n">
              <span class="tag" :class="statusClass(r.status)">{{ r.status || '—' }}</span>
            </td>
            <td class="n">{{ ms(r.ttfb_ms) }}</td>
            <td class="n">{{ ms(r.total_ms) }}</td>
            <td class="n">{{ tokens(r.tokens) }}</td>
            <td class="n">{{ r.tok_per_sec ? r.tok_per_sec.toFixed(1) : '—' }}</td>
          </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>