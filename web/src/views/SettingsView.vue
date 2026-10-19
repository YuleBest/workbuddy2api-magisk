<script setup lang="ts">
// 设置：网关的生效配置（脱敏）与界面偏好。
// 配置本身是只读的——改配置要动 config.json 并重启服务，页面上不放"假开关"。
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { LogOut, Moon, Sun } from '@lucide/vue'
import { api, type ConfigPayload } from '../api'
import { guard, logout, refreshOptions, setTheme, state } from '../store'
import PageHead from '../components/PageHead.vue'
import Panel from '../components/Panel.vue'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { duration, n } from '../format'

const router = useRouter()
const payload = ref<ConfigPayload | null>(null)
void (async () => {
  const res = await guard(() => api.config())
  if (res) payload.value = res
})()

const GROUP_LABELS: Record<string, string> = {
  listen: '服务',
  auth_dir: '服务',
  state_file: '服务',
  keys_file: '服务',
  max_body_mb: '服务',
  api_key_set: '服务',
  cooldown: '冷却与退避',
  schedule: '定时任务',
  upstream: '上游请求',
  features: '出站改写',
  prompt: '系统提示词',
  upstash: 'Redis 镜像',
  pool: '账号池调优',
  session_sticky: '会话粘性',
  global: '双域路由',
  admin: '管理后台',
}

const FIELD_LABELS: Record<string, string> = {
  listen: '监听地址',
  auth_dir: '凭证目录',
  state_file: '状态文件',
  keys_file: '密钥文件',
  max_body_mb: '请求体上限（MB）',
  api_key_set: 'config 里配了 api_key',
  soft_rate: '限流冷却基数',
  soft_rate_max: '冷却退避封顶',
  checkin_hours: '签到时点',
  travel_hours: '旅行时点',
  activity_hours: '活跃上报时点',
  keepalive_hours: '保活时点',
  checkin_enabled: '启用签到',
  travel_enabled: '启用旅行',
  activity_enabled: '启用活跃上报',
  keepalive_enabled: '启用保活',
  activity_report_count: '每次上报条数',
  timeout_seconds: '短请求超时（秒）',
  header_timeout_seconds: '流式首字节超时（秒）',
  idle_timeout_seconds: '流式空闲超时（秒）',
  user_agent: 'User-Agent 覆盖',
  client_version: '客户端版本段',
  cli_version: 'CLI 版本段',
  client_name: '用量归属名',
  passthrough_ip: '透传客户端 IP',
  device_token_set: '配了设备 token',
  device_token_file: '从文件读设备 token',
  sanitize_blacklist_fingerprints: '出站指纹脱敏',
  mode: '模式',
  file: '提示词文件',
  prompt_chars: '提示词字数',
  prompt_loaded: '提示词已加载',
  configured: '已配置',
  token_set: '配了 token',
  max_in_flight: '单账号最大在途',
  max_in_flight_global: '国际版单账号在途',
  degrade_threshold: '连败降权阈值',
  degrade_cooldown: '降权时长',
  degrade_cooldown_max: '降权时长上限',
  expiring_soon: '快过期积分窗口',
  cost_explore_interval: '成本探索间隔',
  chat_base: '国际版 chat base',
  billing_base: '国际版 billing base',
  realm: '域',
  consecutive_fails: '连续失败',
  degrade_until: '降权截止',
  manual_disabled: '手动停用',
  manual_reason: '停用原因',
  rate_limited_models: '限额模型',
  breaker_threshold: '熔断阈值（连续失败）',
  breaker_cooldown: '熔断基础时长',
  breaker_cooldown_max: '熔断退避封顶',
  idle_weight_per_hour: '闲置补偿（每小时）',
  idle_weight_max: '闲置补偿封顶',
  enabled: '启用',
  ttl: '绑定有效期',
  gc_interval: '清理周期',
  token_from_config: 'token 来自 config',
}

function label(key: string) {
  return FIELD_LABELS[key] ?? key
}

function formatValue(v: unknown): string {
  if (v === null || v === undefined) return '—'
  if (typeof v === 'boolean') return v ? '是' : '否'
  if (Array.isArray(v)) return v.length ? v.join(', ') : '—'
  if (typeof v === 'string') return v === '' ? '未设置' : v
  if (typeof v === 'number') return String(v)
  return JSON.stringify(v)
}

interface Group {
  key: string
  label: string
  rows: { k: string; label: string; value: string }[]
}

const groups = computed<Group[]>(() => {
  const cfg = payload.value?.config ?? {}
  const out: Group[] = []
  for (const [key, value] of Object.entries(cfg)) {
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      const rows = Object.entries(value as Record<string, unknown>).map(([k, v]) => ({
        k,
        label: label(k),
        value: formatValue(v),
      }))
      const groupLabel = GROUP_LABELS[key] ?? key
      const existing = out.find((g) => g.label === groupLabel)
      if (existing) existing.rows.push(...rows)
      else out.push({ key, label: groupLabel, rows })
    } else {
      const groupLabel = GROUP_LABELS[key] ?? '其他'
      const row = { k: key, label: label(key), value: formatValue(value) }
      const existing = out.find((g) => g.label === groupLabel)
      if (existing) existing.rows.unshift(row)
      else out.push({ key: groupLabel, label: groupLabel, rows: [row] })
    }
  }
  return out
})

const runtime = computed(() => {
  const p = payload.value
  return [
    { label: '服务', value: p?.service ?? '—' },
    { label: '版本', value: p?.version ?? '—' },
    { label: '监听', value: p?.listen ?? '—' },
    { label: '启动时间', value: p ? new Date(p.started_at).toLocaleString('zh-CN') : '—' },
    {
      label: '运行时长',
      value: p ? duration(Math.floor((Date.now() - new Date(p.started_at).getTime()) / 1000)) : '—',
    },
    { label: '粘性会话', value: `${n(p?.sticky_sessions)} 个绑定` },
    { label: 'Redis 镜像', value: p?.redis_mode === 'upstash' ? '已启用' : '未启用（纯内存）' },
    {
      label: '提示词模式',
      value: p?.prompt_mode === 'passthrough' ? 'passthrough（透传客户端 system）' : 'custom（网关自有提示词）',
    },
    { label: '密钥文件', value: p?.keys_file || '未配置文件（用 config 里的 api_key）' },
    { label: '管理鉴权', value: p?.auth_required ? '需要 token' : '未设 token，任何人可访问' },
  ]
})

const refreshValue = computed({
  get: () => String(state.refreshSec),
  set: (value: string) => {
    state.refreshSec = Number(value)
  },
})

function quit() {
  logout()
  void router.push('/login')
}
</script>

<template>
  <div class="view">
    <PageHead
      title="设置"
      lede="这里是网关当前生效的配置（密钥类字段只显示有没有配，不显示值）。要改配置，编辑 config.json 后重启服务。"
    />

    <Panel title="运行信息" class="rise mb-6">
      <dl class="grid gap-x-10 gap-y-3 text-[0.82rem] sm:grid-cols-2">
        <div v-for="row in runtime" :key="row.label" class="flex items-baseline justify-between gap-4 border-b border-border/60 pb-2.5">
          <dt class="shrink-0 text-muted-foreground">{{ row.label }}</dt>
          <dd class="min-w-0 text-right break-words">{{ row.value }}</dd>
        </div>
      </dl>
    </Panel>

    <Panel title="生效配置" note="密钥只回显是否配置，不显示值" class="rise rise-1 mb-6">
      <div class="columns-1 gap-x-12 lg:columns-2">
        <section v-for="g in groups" :key="g.key" class="mb-7 break-inside-avoid">
          <h3 class="font-serif text-[0.9rem]">{{ g.label }}</h3>
          <dl class="mt-2 border-t border-border/70">
            <div
              v-for="r in g.rows"
              :key="r.k"
              class="flex items-baseline justify-between gap-4 border-b border-border/50 py-1.5 text-[0.78rem]"
            >
              <dt class="shrink-0 text-muted-foreground">{{ r.label }}</dt>
              <dd class="min-w-0 text-right break-words">{{ r.value }}</dd>
            </div>
          </dl>
        </section>
      </div>
    </Panel>

    <Panel title="界面偏好" class="rise rise-2 mb-6">
      <div class="flex flex-wrap items-end gap-x-10 gap-y-5">
        <div class="space-y-2">
          <span class="block text-xs font-medium tracking-wide text-muted-foreground">自动刷新间隔</span>
          <Select v-model="refreshValue">
            <SelectTrigger class="w-[7.5rem]" aria-label="自动刷新间隔">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="o in refreshOptions" :key="o.value" :value="String(o.value)">
                {{ o.label }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div class="space-y-2">
          <span class="block text-xs font-medium tracking-wide text-muted-foreground">主题</span>
          <div class="flex items-center gap-1.5">
            <Button
              :variant="state.theme === 'light' ? 'default' : 'outline'"
              size="sm"
              @click="setTheme('light')"
            >
              <Sun class="size-3.5" />
              浅色
            </Button>
            <Button
              :variant="state.theme === 'dark' ? 'default' : 'outline'"
              size="sm"
              @click="setTheme('dark')"
            >
              <Moon class="size-3.5" />
              深色
            </Button>
          </div>
        </div>

        <div class="space-y-2">
          <span class="block text-xs font-medium tracking-wide text-muted-foreground">登录状态</span>
          <Button variant="outline" size="sm" @click="quit">
            <LogOut class="size-3.5" />
            退出登录
          </Button>
        </div>
      </div>
    </Panel>

    <Panel title="改配置去哪改" class="rise rise-3">
      <p class="text-[0.8rem] leading-relaxed text-muted-foreground">
        配置项都在网关主机上，页面刻意不提供在线修改——写错一个值就可能让整个网关起不来。
      </p>
      <pre class="mono mt-4 overflow-x-auto rounded-md border border-border bg-muted/60 px-4 py-3 text-[0.72rem] leading-relaxed">vim ~/wb2api/config.json
sudo systemctl restart wb2api-gateway

# 调用密钥可以直接在「调用密钥」页改，热生效、不用重启</pre>
    </Panel>
  </div>
</template>
