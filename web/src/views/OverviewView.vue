<script setup lang="ts">
// 总览：一眼回答"网关现在能不能接活、为什么不能"。
// 页面顺序按运维实际排查顺序排：能不能用 → 流量状况 → 定时任务 → 最近发生了什么。
import { computed, ref } from 'vue'
import { api, type CheckinOutcome, type Overview, type TaskResult } from '../api'
import { guard, toast } from '../store'
import { usePolling } from '../composables/usePolling'
import PageHead from '../components/PageHead.vue'
import StatusLamp from '../components/StatusLamp.vue'
import RecentRequests from '../components/RecentRequests.vue'
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
    { key: 'checkin', name: '签到', hint: '签到并顺带查余额，冷却中的账号余额恢复即解冻', tv: s.checkin },
    { key: 'travel', name: '猫猫旅行', hint: '领养 / 派出 / 领奖一趟闭环', tv: s.travel },
    { key: 'activity', name: '活跃上报', hint: '点亮连登、补满领猫所需对话次数', tv: s.activity },
    { key: 'keepalive', name: 'token 保活', hint: '提前刷新凭证，避免到期后首个请求失败', tv: s.keepalive },
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
  return status === 'ok' ? 'ok' : status === 'already' ? '' : status === 'fail' ? 'bad' : 'warn'
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
</script>

<template>
  <div class="view">
    <PageHead title="总览" lede="网关当前的接活能力、账号池分布与最近发生的请求。数据来自网关进程本身，页面每几秒自动刷新。">
      <template #actions>
        <span v-if="data" class="muted small num">已运行 {{ duration(data.uptime_sec) }}</span>
        <button type="button" @click="load">立即刷新</button>
      </template>
    </PageHead>

    <div v-if="data?.degraded" class="notice">
      <strong>提示词降级期</strong>：passthrough 模式下有请求被上游内容策略拦下，网关已切换到中性提示词直到次日 00:00。
      若此时仍有大量失败，检查是不是请求内容本身触发了审核。
    </div>
    <div v-else-if="data && !data.pool.servable" class="notice bad">
      <strong>当前没有账号能接活</strong>：冷却会自行到期，禁用需要人工处理（凭证失效要重新走 <code>./login.sh</code>）。
      去「账号池」看看是哪个账号、什么原因。
    </div>

    <section class="panel">
      <div class="panel-head">
        <h2>接活状态</h2>
        <span class="note">{{ data?.listen ?? '' }}</span>
        <span class="spacer"></span>
        <span v-if="data?.sticky_sessions" class="note">粘性会话 {{ n(data.sticky_sessions) }} 个</span>
      </div>

      <div class="panel-body">
        <div class="row" style="gap: 12px">
          <StatusLamp :state="verdict.tone" :live="pool?.servable" />
          <div>
            <div style="font-size: 18px; font-weight: 600">{{ verdict.text }}</div>
            <div class="muted small">{{ verdict.detail }}</div>
          </div>
        </div>

        <div class="lanes" v-if="data?.accounts.length">
          <button
            v-for="a in data.accounts"
            :key="a.uid"
            type="button"
            class="lane"
            :class="a.state"
            :title="`${a.nickname || a.uid} — ${stateWord(a.state)}${a.reason ? '：' + a.reason : ''}`"
            @click="$router.push('/accounts')"
          >
            <StatusLamp :state="stateTone(a.state)" :live="a.state === 'healthy'" />
            <span class="lane-name">
              {{ a.nickname || a.uid.slice(0, 8) }}
              <span v-if="a.realm === 'global'" class="muted small">（国际版）</span>
            </span>
            <span class="lane-state">{{ stateWord(a.state) }}</span>
            <span v-if="a.state === 'cooling'" class="num lane-cool">剩 {{ countdown(a.cool_remaining) }}</span>
            <span v-else class="num lane-cool">{{ n(a.credits) }} 分</span>
          </button>
        </div>
        <div v-else class="empty">
          <strong>账号池是空的</strong>
          在网关主机上执行 <code>./login.sh</code> 走一次设备授权，凭据落进 <code>auths/</code> 后这里就会出现账号。
        </div>
      </div>

      <div class="facts">
        <div class="fact">
          <div class="k">账号总数</div>
          <div class="v">{{ n(pool?.total) }}</div>
        </div>
        <div class="fact">
          <div class="k">可调度</div>
          <div class="v" style="color: var(--ok)">{{ n(pool?.healthy) }}</div>
        </div>
        <div class="fact">
          <div class="k">冷却中</div>
          <div class="v" style="color: var(--warn)">{{ n(pool?.cooling) }}</div>
        </div>
        <div class="fact">
          <div class="k">已禁用</div>
          <div class="v" :style="pool?.disabled ? 'color: var(--bad)' : ''">{{ n(pool?.disabled) }}</div>
        </div>
        <div class="fact">
          <div class="k">在途请求</div>
          <div class="v">{{ n(pool?.in_flight) }}</div>
        </div>
      </div>
    </section>

    <div class="grid two">
      <section class="panel">
        <div class="panel-head">
          <h2>请求流量</h2>
          <span class="note">最近 {{ n(req?.window) }} 条记录</span>
        </div>
        <div class="panel-body tight">
          <table class="data">
            <tbody>
              <tr>
                <td class="muted">进程累计请求</td>
                <td class="n">{{ n(req?.total) }}</td>
              </tr>
              <tr>
                <td class="muted">最近一小时</td>
                <td class="n">{{ n(req?.last_hour) }}</td>
              </tr>
              <tr>
                <td class="muted">窗口内失败</td>
                <td class="n" :style="req?.error ? 'color: var(--bad)' : ''">
                  {{ n(req?.error) }} / {{ n((req?.ok ?? 0) + (req?.error ?? 0)) }}
                </td>
              </tr>
              <tr>
                <td class="muted">平均首字延迟</td>
                <td class="n">{{ ms(req?.avg_ttfb_ms) }}</td>
              </tr>
              <tr>
                <td class="muted">平均输出 token</td>
                <td class="n">{{ tokens(req?.avg_tokens ?? 0) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="panel">
        <div class="panel-head">
          <h2>模型分布</h2>
          <span class="note">窗口内请求数</span>
        </div>
        <div class="panel-body">
          <div v-if="modelBars.length" class="stack" style="gap: 10px">
            <div v-for="m in modelBars" :key="m.model">
              <div class="row" style="justify-content: space-between">
                <span class="mono small">{{ m.model }}</span>
                <span class="num muted small">{{ n(m.count) }}</span>
              </div>
              <div class="credit">
                <div class="track" style="height: 5px">
                  <span class="seg" :class="{ on: true }" :style="`flex: 0 0 ${m.pct}%; max-width: 100%`"></span>
                  <span class="seg" style="flex: 1"></span>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="empty">
            <strong>窗口里还没有请求</strong>
            客户端接入网关后，这里会按模型统计调用量。接口地址是 <code>/v1/chat/completions</code>。
          </div>
        </div>
      </section>
    </div>

    <section class="panel">
      <div class="panel-head">
        <h2>定时任务</h2>
        <span class="note">在配置的整点自动执行；下面的按钮可以立刻跑一次</span>
        <span class="spacer"></span>
        <span class="note">服务日志：<code>journalctl -u wbapi-gateway -f</code></span>
      </div>
      <div class="panel-body tight">
        <div v-for="t in tasks" :key="t.key" class="strip" style="grid-template-columns: minmax(140px, 220px) 1fr auto">
          <div class="ident">
            <StatusLamp :state="t.tv.enabled ? 'ok' : 'idle'" />
            <div class="who">
              <div class="nick">{{ t.name }}</div>
              <div class="muted small">
                <template v-if="t.tv.enabled">
                  每天 <span class="num">{{ t.tv.hours.map((h) => `${h}:00`).join(' / ') }}</span>
                </template>
                <template v-else>已在配置里关闭</template>
              </div>
            </div>
          </div>
          <div class="muted small">{{ t.hint }}</div>
          <div class="strip-actions">
            <button type="button" :disabled="busyTask === t.key" @click="runTask(t.key)">
              {{ busyTask === t.key ? (t.key === 'checkin' ? '签到中…' : '已触发') : '立即执行' }}
            </button>
          </div>
        </div>
      </div>

      <div v-if="checkinResult" class="panel-body" style="border-top: 1px solid var(--line)">
        <div class="row" style="justify-content: space-between; margin-bottom: 10px">
          <strong style="font-size: 13px">{{ checkinResult.message }}</strong>
          <button class="ghost" type="button" @click="checkinResult = null">收起</button>
        </div>
        <p v-if="checkinResult.waited === false" class="muted small" style="margin: 0 0 10px">
          这次没等到结果就返回了：签到还在后台跑，稍后看账号池的积分变化。
        </p>
        <table v-if="checkinResult.outcomes?.length" class="data">
          <thead>
            <tr>
              <th>账号</th>
              <th>结果</th>
              <th class="n">签到后余额</th>
              <th>说明</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="oc in checkinResult.outcomes" :key="oc.uid">
              <td>
                <div>{{ oc.nickname || '未命名账号' }}</div>
                <div class="mono small muted">{{ oc.uid.slice(0, 8) }}</div>
              </td>
              <td><span class="tag" :class="outcomeTone(oc.status)">{{ OUTCOME_LABEL[oc.status] }}</span></td>
              <td class="n">{{ oc.credits === undefined ? '—' : n(oc.credits) }}</td>
              <td class="small muted">{{ oc.detail || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>最近请求</h2>
        <span class="spacer"></span>
        <RouterLink class="btn" to="/logs">查看完整日志</RouterLink>
      </div>
      <RecentRequests :limit="8" />
    </section>
  </div>
</template>