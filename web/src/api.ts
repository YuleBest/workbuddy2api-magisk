// api.ts 与网关管理 API 的通信层：token 存取 + 统一错误处理。
//
// token 存 localStorage：后台是运营工具而非公共站点，刷新/新标签页不该反复登录。
// 所有请求带 Authorization: Bearer，401 统一抛 ApiError(401) 由路由层踢回登录页。

const TOKEN_KEY = 'wb2api.admin.token'

export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

export const tokenStore = {
  get(): string {
    return localStorage.getItem(TOKEN_KEY) ?? ''
  },
  set(token: string) {
    localStorage.setItem(TOKEN_KEY, token)
  },
  clear() {
    localStorage.removeItem(TOKEN_KEY)
  },
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = tokenStore.get()
  const headers: Record<string, string> = { ...(init.headers as Record<string, string>) }
  if (token) headers.Authorization = `Bearer ${token}`
  if (init.body) headers['Content-Type'] = 'application/json'

  let res: Response
  try {
    res = await fetch(`/api/admin${path}`, { ...init, headers })
  } catch {
    throw new ApiError(0, 'network', '连不上网关服务，确认进程在运行、地址没打错')
  }
  const text = await res.text()
  let body: any = null
  if (text) {
    try {
      body = JSON.parse(text)
    } catch {
      body = null
    }
  }
  if (!res.ok) {
    const err = body?.error ?? {}
    throw new ApiError(res.status, err.code ?? String(res.status), err.message ?? `请求失败（HTTP ${res.status}）`)
  }
  return body as T
}

export const api = {
  session: () => request<SessionInfo>('/session'),
  overview: () => request<Overview>('/overview'),
  accounts: () => request<{ accounts: Account[]; now: string }>('/accounts'),
  keys: () => request<{ keys: KeyInfo[]; file: string; auth_required: boolean }>('/keys'),
  createKey: (note: string) =>
    request<{ key: string; note: string; message: string }>('/keys', { method: 'POST', body: JSON.stringify({ note }) }),
  revokeKey: (prefix: string) =>
    request<{ ok: boolean; remaining: number }>(`/keys/${encodeURIComponent(prefix)}`, { method: 'DELETE' }),
  logs: (limit = 200) => request<LogsPayload>(`/logs?limit=${limit}`),
  models: () => request<{ models: ModelInfo[] }>('/models'),
  config: () => request<ConfigPayload>('/config'),
  // disable/enable 管手动停用位，revive 管系统自动禁用位（上游双位语义）。
  accountAction: (uid: string, action: 'disable' | 'enable' | 'revive' | 'clear-cooldown', reason?: string) =>
    request<Record<string, unknown>>(`/accounts/${encodeURIComponent(uid)}/${action}`, {
      method: 'POST',
      body: action === 'disable' ? JSON.stringify({ reason: reason ?? '' }) : undefined,
    }),
  // wait=true 时同步等待任务跑完并回传结果（目前只有签到支持，见后端 runCheckinSync）。
  runTask: (task: string, wait = false) =>
    request<TaskResult>(`/tasks/${task}${wait ? '?wait=1' : ''}`, { method: 'POST' }),
}

// ------------------------------------------------------------------ 类型

export interface CheckinOutcome {
  uid: string
  nickname?: string
  status: 'ok' | 'already' | 'fail' | 'skipped'
  credits?: number
  detail?: string
}

export interface TaskResult {
  ok: boolean
  task: string
  message: string
  /** 仅同步等待的任务（签到）返回：是否等到了结果。 */
  waited?: boolean
  outcomes?: CheckinOutcome[]
  summary?: { total: number; ok: number; already: number; fail: number; skipped: number }
}

export interface AccountActionState {
  uid: string
  manual_disabled: boolean
  manual_reason?: string
  disabled: boolean
  changed: boolean
}

export interface SessionInfo {
  ok: boolean
  service: string
  version: string
  auth_required: boolean
  now: string
}

export type AccountState = 'healthy' | 'cooling' | 'manual' | 'disabled'

export interface AccountBrief {
  uid: string
  realm?: string
  nickname?: string
  state: AccountState
  credits: number
  cool_remaining: number
  reason?: string
}

export interface ModelCost {
  model: string
  cost_per_1k: number
  samples?: number
  last_seen: string
}

/** 模型级限额台账（带解析时间的 6004 才出现，到期即消失）。 */
export interface RateLimitedModel {
  model: string
  until: string
  remaining_sec?: number
  reason?: string
}

export interface Account {
  uid: string
  realm?: string
  nickname?: string
  credits: number
  cooling: boolean
  cool_kind?: string
  cool_remaining_sec?: number
  until?: string
  reason?: string
  soft_streak?: number
  /** 模型级限额台账（该号当前还在限额的模型）。 */
  rate_limited_models?: RateLimitedModel[]
  model_costs?: ModelCost[]
  /** 系统自动禁用位（12153 / 连败等）：靠"复活"解除。 */
  disabled: boolean
  disabled_reason?: string
  /** 运维手动停用位：靠"恢复"解除，与 disabled 相互独立。 */
  manual_disabled: boolean
  manual_reason?: string
  success_count?: number
  err_total?: number
  consecutive_fails?: number
  degrade_until?: string
  last_success?: string
  last_err?: string
  in_flight: number
  breaker_fails: number
  breaker_until?: string
  expires_at?: string
  expires_in_sec?: number
  has_device_token: boolean
}

export interface RequestRecord {
  seq: number
  time: string
  model: string
  mode: string
  uid: string
  nick?: string
  status: number
  ttfb_ms: number
  tokens: number
  total_ms: number
  tok_per_sec: number
}

export interface LogsPayload {
  records: RequestRecord[]
  total: number
  limit: number
  capacity: number
}

export interface ModelInfo {
  id: string
  object: string
  created: number
  owned_by: string
  context_length: number
  max_output_tokens?: number
  /** 上游显示名（如 "Auto"、"GLM-5v-Turbo"）。 */
  name?: string
  /** 上游积分倍率原文（形如 "x0.21"；global 域会带 " credits" 后缀），x0.00 = 限时免费。 */
  credits?: string
  /** 中文描述，上游已把倍率拼成前缀（"[x0.21 credit] …"）。 */
  description?: string
  /** 上游标签（如 "craft"、"badge:限时免费:#FF0000"）。 */
  tags?: string[]
  /** 厂商代号（上游内部编码，单字母）。 */
  vendor?: string
  /** 上游另一口径的尺寸上限，与 context_length 不一定相同。 */
  max_allowed_size?: number
  /** 上游标记的默认模型（auto / fast-model 之类）。 */
  is_default?: boolean
  supports_images?: boolean
  supports_tool_call?: boolean
  supports_reasoning?: boolean
  /** 只能以思考模式运行（关不掉）。 */
  only_reasoning?: boolean
  /** 可传的推理档位（缺失 = 该模型无档位）。 */
  reasoning_supported_efforts?: string[]
  reasoning_default_effort?: string
  /** 上游默认 effort。 */
  reasoning_effort?: string
  /** 思考摘要模式（如 "auto"）。 */
  reasoning_summary?: string
}

export interface TaskView {
  enabled: boolean
  hours: number[]
}

export interface Overview {
  service: string
  version: string
  started_at: string
  uptime_sec: number
  listen: string
  now: string
  pool: {
    total: number
    healthy: number
    cooling: number
    disabled: number
    in_flight: number
    in_flight_full: number
    servable: boolean
  }
  accounts: AccountBrief[]
  requests: {
    total: number
    window: number
    capacity: number
    ok: number
    error: number
    avg_ttfb_ms: number
    avg_tokens: number
    last_hour: number
    models: { model: string; count: number }[]
  }
  sticky_sessions: number
  redis_mode: string
  keys_count: number
  prompt_mode: string
  degraded: boolean
  schedule: {
    checkin: TaskView
    travel: TaskView
    activity: TaskView
    keepalive: TaskView
  }
}

export interface KeyInfo {
  prefix: string
  masked: string
  note?: string
  added?: string
}

export interface ConfigPayload {
  config: Record<string, any>
  service: string
  version: string
  keys_file: string
  auth_required: boolean
  listen: string
  redis_mode: string
  prompt_mode: string
  started_at: string
  sticky_sessions: number
}