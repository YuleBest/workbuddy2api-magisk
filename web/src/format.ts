// format.ts 展示层格式化：时间、时长、数字。
// 这里只做"给人看"的转换，不做任何业务判断（状态语义由视图决定）。

const pad = (n: number) => String(n).padStart(2, '0')

/** 本地时间 HH:mm:ss。 */
export function clock(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return '—'
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** 本地日期时间 MM-DD HH:mm。 */
export function stamp(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return '—'
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** 相对时间：刚刚 / 3 分钟前 / 2 小时前。 */
export function ago(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return '—'
  const sec = Math.round((Date.now() - d.getTime()) / 1000)
  if (sec < 5) return '刚刚'
  if (sec < 60) return `${sec} 秒前`
  if (sec < 3600) return `${Math.floor(sec / 60)} 分钟前`
  if (sec < 86400) return `${Math.floor(sec / 3600)} 小时前`
  return `${Math.floor(sec / 86400)} 天前`
}

/** 秒数 → 倒计时 mm:ss（超过 1 小时用 h:mm:ss）。 */
export function countdown(sec?: number | null): string {
  if (sec === undefined || sec === null || sec <= 0) return '0:00'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = Math.floor(sec % 60)
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`
}

/** 秒数 → 时长文案（12 分 30 秒 / 3 小时 5 分）。 */
export function duration(sec?: number): string {
  if (!sec || sec <= 0) return '0 秒'
  const h = Math.floor(sec / 3600)
  const m = Math.round((sec % 3600) / 60)
  if (h > 0) return m > 0 ? `${h} 小时 ${m} 分` : `${h} 小时`
  const s = Math.floor(sec % 60)
  if (m > 0) return s > 0 ? `${m} 分 ${s} 秒` : `${m} 分`
  return `${s} 秒`
}

/** 毫秒 → 人类可读（320ms / 3.2s）。 */
export function ms(v?: number): string {
  if (!v) return '—'
  return v < 1000 ? `${Math.round(v)}ms` : `${(v / 1000).toFixed(1)}s`
}

/** 整数千分位。 */
export function n(v?: number): string {
  if (v === undefined || v === null) return '—'
  return v.toLocaleString('zh-CN')
}

/** 积分单位下的成本：0 = 免费，小数保留 4 位。 */
export function cost(v: number): string {
  if (v === 0) return '免费'
  return v < 0.01 ? v.toFixed(4) : v.toFixed(2)
}

/** token 数：>=1000 折成 k。 */
export function tokens(v: number): string {
  if (v < 0) return '—'
  return v >= 1000 ? `${(v / 1000).toFixed(1)}k` : String(v)
}

/** HTTP 状态码 → 语义类名。 */
export function statusClass(status: number): string {
  if (status >= 200 && status < 300) return 'ok'
  if (status >= 400 && status < 500) return 'warn'
  if (status >= 500) return 'bad'
  return ''
}

/** 冷却原因代码 → 中文说明（后端给的是英文枚举/上游原文）。 */
export function coolReason(reason?: string): string {
  if (!reason) return '限流冷却'
  if (reason.includes('6004')) return '当日模型用量用尽'
  if (reason.includes('429')) return '上游限流'
  if (reason.includes('404')) return '上游路径偶发缺失'
  if (reason.includes('余额')) return '积分耗尽'
  if (reason.includes('12153')) return '凭证失效'
  return reason
}