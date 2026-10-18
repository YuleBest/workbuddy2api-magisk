<script setup lang="ts">
// 账号池：每个账号一条"仪表条"——状态灯、积分余量、冷却倒计时、在途请求、
// 以及可执行的动作（禁用/启用/清冷却）。展开可见成本账本与凭证信息。
import { computed, onUnmounted, ref } from 'vue'
import { api, type Account } from '../api'
import { guard, toast } from '../store'
import { usePolling } from '../composables/usePolling'
import PageHead from '../components/PageHead.vue'
import StatusLamp from '../components/StatusLamp.vue'
import CreditBar from '../components/CreditBar.vue'
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

async function act(a: Account, action: 'disable' | 'enable' | 'revive' | 'clear-cooldown') {
  if (action === 'disable') {
    const reason = window.prompt('停用原因（会显示在账号条上，方便以后回想）', '手动排查')
    if (reason === null) return
    pending.value = a.uid + action
    const res = await guard(() => api.accountAction(a.uid, action, reason))
    pending.value = ''
    if (res) {
      toast(
        res.disabled
          ? `已停用 ${a.nickname || a.uid.slice(0, 8)}；注意它还被系统禁用着，要另外点"复活"`
          : `已停用 ${a.nickname || a.uid.slice(0, 8)}，不再承接对话流量`,
      )
    }
    return
  }
  pending.value = a.uid + action
  const res = await guard(() => api.accountAction(a.uid, action))
  pending.value = ''
  if (!res) return
  if (action === 'enable') {
    toast(res.disabled ? '手动停用已解除，但系统禁用位还在——需要点"复活"' : '已放回池子，下一个请求就可能选中它', res.disabled ? 'warn' : 'ok')
  } else if (action === 'revive') {
    toast('系统禁用位已清除，账号回到选号池')
  } else {
    toast('冷却已清除')
  }
}

function tokenState(a: Account) {
  if (!a.expires_in_sec) return { text: '过期时间未知', tone: 'warn' }
  if (a.expires_in_sec <= 0) return { text: '凭证已过期，下次使用会先刷新', tone: 'bad' }
  if (a.expires_in_sec < 600) return { text: `${duration(a.expires_in_sec)}后过期`, tone: 'warn' }
  return { text: `${duration(a.expires_in_sec)}后过期`, tone: 'ok' }
}
</script>

<template>
  <div class="view">
    <PageHead
      title="账号池"
      lede="网关按积分余量、闲置时长、成功率加权挑号。这里的动作直接改运行中的池状态，不需要重启服务。"
    >
      <template #actions>
        <span class="muted small">{{ accounts.length }} 个账号</span>
        <button type="button" @click="$router.push('/overview')">回总览</button>
      </template>
    </PageHead>

    <section class="panel">
      <div class="panel-head">
        <h2>池内账号</h2>
        <span class="note">积分来自上次签到查询，冷却与熔断为实时状态</span>
      </div>

      <div v-if="!accounts.length" class="empty">
        <strong>还没有账号</strong>
        在网关主机上跑 <code>./login.sh</code> 完成一次设备授权登录，凭据会自动落进 <code>auths/</code> 并被池子接管。
      </div>

      <div v-for="a in accounts" :key="a.uid" class="strip" :class="{ 'is-dead': a.disabled, 'is-cooling': a.cooling && !a.disabled }">
        <div class="ident">
          <StatusLamp :state="tone(a)" :live="!a.disabled && !a.cooling" />
          <div class="who">
            <div class="nick">
              {{ a.nickname || '未命名账号' }}
              <span v-if="a.realm" class="tag" style="margin-left: 6px">{{ a.realm === 'global' ? '国际版' : '国内版' }}</span>
            </div>
            <div class="uid">{{ a.uid }}</div>
          </div>
        </div>

        <CreditBar :value="a.credits" :max="maxCredits" />

        <div class="strip-actions">
          <button
            v-if="!a.manual_disabled"
            type="button"
            :disabled="pending === a.uid + 'disable'"
            title="把账号摘出选号池，但保留在池里：签到、保活照常，凭证和积分都是活的"
            @click="act(a, 'disable')"
          >
            停用
          </button>
          <button
            v-else
            class="primary"
            type="button"
            :disabled="pending === a.uid + 'enable'"
            title="解除手动停用"
            @click="act(a, 'enable')"
          >
            恢复
          </button>
          <button
            v-if="a.disabled"
            class="primary"
            type="button"
            :disabled="pending === a.uid + 'revive'"
            title="清除系统自动禁用位（12153 / 连续失败）"
            @click="act(a, 'revive')"
          >
            复活
          </button>
          <button
            v-if="a.cooling && !a.disabled"
            type="button"
            :disabled="pending === a.uid + 'clear-cooldown'"
            title="清掉冷却后，下一个请求可能立刻选中它"
            @click="act(a, 'clear-cooldown')"
          >
            清冷却
          </button>
          <button class="ghost" type="button" @click="expanded[a.uid] = !expanded[a.uid]">
            {{ expanded[a.uid] ? '收起' : '详情' }}
          </button>
        </div>

        <div class="strip-meta">
          <span>
            状态
            <span class="num">{{ word(a) }}</span>
          </span>
          <span v-if="a.cooling && !a.disabled" class="alert">
            冷却 <span class="num">{{ countdown(remainOf(a)) }}</span> 后到期
            <template v-if="a.reason">（{{ coolReason(a.reason) }}）</template>
            <template v-if="a.soft_streak && a.soft_streak > 1">，连续第 {{ a.soft_streak }} 次，退避在加倍</template>
          </span>
          <span v-if="a.manual_disabled" class="alert bad">
            手动停用<template v-if="a.manual_reason">：{{ a.manual_reason }}</template>
          </span>
          <span v-if="a.disabled" class="alert bad">
            系统禁用<template v-if="a.disabled_reason">：{{ a.disabled_reason }}</template>
          </span>
          <span v-if="a.rate_limited_models?.length" class="alert">
            限额模型
            <span class="num">{{ a.rate_limited_models.map((m) => m.model).join(', ') }}</span>
          </span>
          <span v-if="a.degrade_until && new Date(a.degrade_until) > new Date()" class="alert">
            连败降权中（连续失败 {{ a.consecutive_fails }} 次）
          </span>
          <span>
            在途 <span class="num">{{ a.in_flight }}</span>
          </span>
          <span v-if="a.breaker_until && new Date(a.breaker_until) > new Date()" class="alert">
            熔断中，连续失败 {{ a.breaker_fails }} 次
          </span>
          <span>
            成功 <span class="num">{{ n(a.success_count) }}</span>
            <template v-if="a.err_total">，失败 <span class="num">{{ n(a.err_total) }}</span></template>
          </span>
          <span>最近成功 {{ ago(a.last_success) }}</span>
        </div>

        <div v-if="expanded[a.uid]" class="strip-meta" style="flex-direction: column; gap: 8px">
          <div class="kv" style="width: 100%">
            <dt>凭证状态</dt>
            <dd :style="tokenState(a).tone === 'bad' ? 'color: var(--bad)' : ''">{{ tokenState(a).text }}</dd>
            <dt>设备 token</dt>
            <dd>{{ a.has_device_token ? '已配置' : '未配置（部分上游校验会因此更严）' }}</dd>
            <dt>最近错误</dt>
            <dd>{{ ago(a.last_err) }}</dd>
            <dt>成本账本</dt>
            <dd>
              <template v-if="a.model_costs?.length">
                <div v-for="c in a.model_costs" :key="c.model">
                  {{ c.model }} — 每千 token {{ cost(c.cost_per_1k) }} 积分（{{ c.samples }} 次实测）
                </div>
              </template>
              <span v-else class="muted">暂无观测：该号还没成功跑过带 usage 的请求</span>
            </dd>
          </div>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>状态含义</h2>
      </div>
      <div class="panel-body">
        <dl class="kv">
          <dt>可用</dt>
          <dd style="font-family: var(--sans)">可以被选中承载请求。</dd>
          <dt>冷却中</dt>
          <dd style="font-family: var(--sans)">上游限流或当日模型用量用尽，到点自动恢复；清冷却只在确认上游已放行时用。</dd>
          <dt>熔断</dt>
          <dd style="font-family: var(--sans)">连续失败达到阈值后指数退避，成功一次即清零。</dd>
          <dt>已停用</dt>
          <dd style="font-family: var(--sans)">
            运维主动摘除（手动位）。签到、保活、定时任务照常执行，凭证和积分都是活的；点「恢复」放回池子。
          </dd>
          <dt>系统禁用</dt>
          <dd style="font-family: var(--sans)">
            网关判定账号坏了（连续 12153 / 连败）。与手动停用是两个独立状态位，点「复活」清除；凭证真失效要重新登录才有救。
          </dd>
        </dl>
      </div>
    </section>
  </div>
</template>