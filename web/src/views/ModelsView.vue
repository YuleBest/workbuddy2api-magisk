<script setup lang="ts">
// 模型：上游当前开放的模型清单。数据由网关动态拉取（缓存 1 小时），纯动态无静态兜底，
// 一个可用账号都没有时会是空列表。
//
// 列的选择原则：只放"选模型时会用来判断"的字段（域 / 倍率 / 能不能识图 / 上下文与输出上限），
// 其余（推理档位、能力细节、上游描述等）收进「更多」弹窗——表格宽了反而不好扫。
import { computed, ref } from 'vue'
import {
  ArrowUpDown,
  Boxes,
  Check,
  ChevronDown,
  ChevronUp,
  Copy,
  Ellipsis,
  RefreshCw,
  Search,
  X,
} from '@lucide/vue'
import { api, type ModelInfo } from '../api'
import { guard, toast } from '../store'
import PageHead from '../components/PageHead.vue'
import Panel from '../components/Panel.vue'
import StatusBadge from '../components/StatusBadge.vue'
import EmptyState from '../components/EmptyState.vue'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { n } from '../format'

const models = ref<ModelInfo[]>([])
const loadedAt = ref<number>(0)
const busy = ref(false)
const detail = ref<ModelRow | null>(null)

interface ModelRow extends ModelInfo {
  realm: string
  name: string
}

// 模型表变化很慢，不必轮询：进页面拉一次，需要时手动刷新。
async function load() {
  busy.value = true
  const res = await guard(() => api.models())
  busy.value = false
  if (res) {
    models.value = res.models
    loadedAt.value = Date.now()
  }
}
void load()

// 上游 credits 字段两个域形态不一致：CN 是 "x0.21"，global 是 "x0.34 credits"。
// 统一取 x 后的数字；缺失（上游未给）显示 —，形态不认识时原样展示。
// 展示只管数字与颜色（x0.00 绿底即"限时免费"，不再写"免费"二字，省列宽）。
function creditRate(raw?: string): { text: string; tone: 'ok' | 'warn' | 'outline' } {
  const v = (raw ?? '').trim()
  if (!v) return { text: '—', tone: 'outline' }
  const hit = /x\s*([0-9]+(?:\.[0-9]+)?)/i.exec(v)
  if (!hit) return { text: v, tone: 'outline' }
  const val = Number(hit[1])
  if (val === 0) return { text: 'x0.00', tone: 'ok' }
  if (val >= 1) return { text: `x${val.toFixed(2)}`, tone: 'warn' }
  return { text: `x${val.toFixed(2)}`, tone: 'outline' }
}

// 排序：点表头在正序/倒序之间切换。缺值的模型（上游没给倍率的那些）恒定排最后，
// 不受方向影响——否则倒序时"没有数据"会跑到最前面冒充"最便宜"。
type SortKey = 'name' | 'rate' | 'context' | 'maxout'

const query = ref('')
const sortKey = ref<SortKey | ''>('')
const sortDir = ref<'asc' | 'desc'>('asc')

function toggleSort(key: SortKey) {
  if (sortKey.value === key) {
    sortDir.value = sortDir.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = key
    sortDir.value = 'asc'
  }
}

/** 倍率取数值用于排序；上游未给或形态不认识 → null（排最后）。 */
function rateValue(raw?: string): number | null {
  const v = (raw ?? '').trim()
  if (!v) return null
  const hit = /x\s*([0-9]+(?:\.[0-9]+)?)/i.exec(v)
  return hit ? Number(hit[1]) : null
}

function sortValue(m: ModelRow, key: SortKey): string | number | null {
  switch (key) {
    case 'name':
      return m.name.toLowerCase()
    case 'rate':
      return rateValue(m.credits)
    case 'context':
      return m.context_length ?? null
    case 'maxout':
      return m.max_output_tokens ?? null
  }
}

// id 形如 "cn:deepseek-v4-flash" / "global:gpt-5.4"：拆出域做标签。
// 展示名一律用上游的 name（"Deepseek-V4.1-Flash" / "GLM-5.3-Flash" 这种正经格式），
// **不要**用 id 去覆盖它——那样等于把上游给的显示名丢掉，只剩裸 id（踩过）。
// 只有 name 缺失时才回落裸 id；重名模型（如 cn:hy3 与 cn:hy3-x 都叫 Hy3）靠下面的完整 id 区分。
const sorted = computed<ModelRow[]>(() =>
  [...models.value]
    .map((m) => {
      const [maybeRealm, ...rest] = m.id.split(':')
      const hasRealm = rest.length > 0
      const bare = hasRealm ? rest.join(':') : m.id
      return { ...m, realm: hasRealm ? maybeRealm : '', name: m.name || bare }
    })
    .sort((a, b) => a.id.localeCompare(b.id)),
)

/** 搜索：匹配显示名与完整 id（大小写不敏感）。 */
const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return sorted.value
  return sorted.value.filter((m) => m.name.toLowerCase().includes(q) || m.id.toLowerCase().includes(q))
})

const view = computed<ModelRow[]>(() => {
  const key = sortKey.value
  if (!key) return filtered.value
  const dir = sortDir.value === 'asc' ? 1 : -1
  return [...filtered.value].sort((a, b) => {
    const av = sortValue(a, key)
    const bv = sortValue(b, key)
    if (av === null && bv === null) return a.id.localeCompare(b.id)
    if (av === null) return 1
    if (bv === null) return -1
    if (typeof av === 'string' && typeof bv === 'string') return av.localeCompare(bv, 'zh') * dir
    if (av === bv) return a.id.localeCompare(b.id)
    return (av < bv ? -1 : 1) * dir
  })
})

/** 域显示：只两个字（国内 / 国际），无前缀的裸 id 归国内。 */
function realmText(realm: string) {
  return realm === 'global' ? '国际' : '国内'
}

/** 能力列：只区分能不能识图，其余能力进弹窗（列里写"识图+工具+思考"太长）。 */
function visionText(m: ModelInfo) {
  return m.supports_images ? '识图' : '文字'
}

async function copyID(id: string) {
  try {
    await navigator.clipboard.writeText(id)
    toast(`已复制 ${id}`)
  } catch {
    toast('浏览器不允许自动复制，请手动选中', 'warn')
  }
}

// 「更多」弹窗内容：推理档位 / 能力明细 / 其余有效字段。
const CAPABILITIES: { key: keyof ModelInfo; label: string; note?: string }[] = [
  { key: 'supports_images', label: '识图', note: '可接收图片输入（多模态）' },
  { key: 'supports_tool_call', label: '工具调用', note: '支持 function calling' },
  { key: 'supports_reasoning', label: '思考', note: '支持推理/思维链' },
  { key: 'only_reasoning', label: '仅思考', note: '只能以思考模式运行，关不掉' },
]

/**
 * 弹窗里的"其他有效字段"：把上游给的非空字段平铺出来。
 * 排除三类：表格里已展示的、能力清单里已展示的、推理档位段里已展示的，
 * 以及协议样板（object/owned_by/created 对所有模型都是常量，放出来只是噪音）。
 */
const SHOWN_ELSEWHERE = new Set([
  // 表格 / 弹窗基本信息
  'id', 'name', 'context_length', 'max_output_tokens', 'credits',
  // 能力清单
  'supports_images', 'supports_tool_call', 'supports_reasoning', 'only_reasoning',
  // 推理档位段
  'reasoning_supported_efforts', 'reasoning_default_effort', 'reasoning_effort', 'reasoning_summary',
  // 协议样板（上游常量）
  'object', 'owned_by', 'created',
])

function extraFields(m: ModelRow): { k: string; v: string }[] {
  const out: { k: string; v: string }[] = []
  for (const [k, v] of Object.entries(m)) {
    if (SHOWN_ELSEWHERE.has(k) || k === 'realm') continue
    if (v === undefined || v === null || v === '' || v === false) continue
    if (Array.isArray(v) && v.length === 0) continue
    out.push({ k, v: Array.isArray(v) ? v.join(', ') : String(v) })
  }
  return out.sort((a, b) => a.k.localeCompare(b.k))
}
</script>

<template>
  <div class="view">
    <PageHead
      title="模型"
      lede="网关透传的上游模型清单。客户端拿 /v1/models 得到的就是这份数据，用哪个模型由请求里的 model 字段决定。"
    >
      <template #actions>
        <span class="num text-xs text-muted-foreground">{{ models.length }} 个模型</span>
        <Button variant="outline" size="sm" :disabled="busy" @click="load">
          <RefreshCw class="size-3.5" />
          {{ busy ? '拉取中…' : '刷新' }}
        </Button>
      </template>
    </PageHead>

    <Panel
      title="可用模型"
      :note="query ? `匹配 ${filtered.length} / ${sorted.length}` : loadedAt ? `更新于 ${new Date(loadedAt).toLocaleTimeString('zh-CN')}` : ''"
      class="rise"
      flush
    >
      <template #actions>
        <div class="relative w-full sm:w-64">
          <Search
            class="pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            v-model="query"
            class="h-8 pl-9 text-xs"
            type="search"
            placeholder="搜索模型名或 ID"
            aria-label="搜索模型"
          />
        </div>
      </template>

      <div v-if="busy && !models.length" class="space-y-4 px-5 py-5 sm:px-6">
        <div v-for="i in 6" :key="i" class="flex items-center gap-4">
          <Skeleton class="h-4 w-40" />
          <Skeleton class="h-5 w-14 rounded-full" />
          <Skeleton class="h-4 w-16" />
          <Skeleton class="h-5 w-14 rounded-full" />
          <Skeleton class="ml-auto h-4 w-20" />
        </div>
      </div>
      <EmptyState v-else-if="!sorted.length" :icon="Boxes" title="没拿到模型列表">
        模型表是纯动态拉取的：网关会向池内任一健康账号请求，池里没有可用账号时这里就是空的。先看「总览」的接活状态。
        <template #action>
          <Button as-child variant="outline" size="sm">
            <RouterLink to="/overview">看总览</RouterLink>
          </Button>
        </template>
      </EmptyState>
      <EmptyState v-else-if="!view.length" :icon="Search" :title="`没有匹配「${query}」的模型`">
        换个关键词，或者清空搜索。
        <template #action>
          <Button variant="outline" size="sm" @click="query = ''">清空搜索</Button>
        </template>
      </EmptyState>

      <Table v-else class="min-w-[48rem]">
        <TableHeader>
          <TableRow>
            <TableHead class="w-[38%]">
              <button
                type="button"
                class="inline-flex items-center gap-1.5 text-[0.72rem] font-medium transition-colors hover:text-foreground"
                title="按模型名排序"
                @click="toggleSort('name')"
              >
                模型
                <ChevronUp v-if="sortKey === 'name' && sortDir === 'asc'" class="size-3 text-primary" />
                <ChevronDown v-else-if="sortKey === 'name'" class="size-3 text-primary" />
                <ArrowUpDown v-else class="size-3 opacity-50" />
              </button>
            </TableHead>
            <TableHead>域</TableHead>
            <TableHead class="text-right">
              <button
                type="button"
                class="inline-flex items-center gap-1.5 text-[0.72rem] font-medium transition-colors hover:text-foreground"
                title="按积分倍率排序（缺倍率的排最后）"
                @click="toggleSort('rate')"
              >
                积分倍率
                <ChevronUp v-if="sortKey === 'rate' && sortDir === 'asc'" class="size-3 text-primary" />
                <ChevronDown v-else-if="sortKey === 'rate'" class="size-3 text-primary" />
                <ArrowUpDown v-else class="size-3 opacity-50" />
              </button>
            </TableHead>
            <TableHead>能力</TableHead>
            <TableHead class="text-right">
              <button
                type="button"
                class="inline-flex items-center gap-1.5 text-[0.72rem] font-medium transition-colors hover:text-foreground"
                title="按上下文长度排序"
                @click="toggleSort('context')"
              >
                上下文长度
                <ChevronUp v-if="sortKey === 'context' && sortDir === 'asc'" class="size-3 text-primary" />
                <ChevronDown v-else-if="sortKey === 'context'" class="size-3 text-primary" />
                <ArrowUpDown v-else class="size-3 opacity-50" />
              </button>
            </TableHead>
            <TableHead class="text-right">
              <button
                type="button"
                class="inline-flex items-center gap-1.5 text-[0.72rem] font-medium transition-colors hover:text-foreground"
                title="按单次最大输出排序（缺值的排最后）"
                @click="toggleSort('maxout')"
              >
                单次最大输出
                <ChevronUp v-if="sortKey === 'maxout' && sortDir === 'asc'" class="size-3 text-primary" />
                <ChevronDown v-else-if="sortKey === 'maxout'" class="size-3 text-primary" />
                <ArrowUpDown v-else class="size-3 opacity-50" />
              </button>
            </TableHead>
            <TableHead class="text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="m in view" :key="m.id">
            <TableCell>
              <div class="text-[0.88rem] font-medium">{{ m.name }}</div>
              <div class="mono mt-0.5 text-[0.7rem] text-muted-foreground">{{ m.id }}</div>
            </TableCell>
            <TableCell>
              <StatusBadge tone="outline">{{ realmText(m.realm) }}</StatusBadge>
            </TableCell>
            <TableCell class="text-right">
              <StatusBadge :tone="creditRate(m.credits).tone">{{ creditRate(m.credits).text }}</StatusBadge>
            </TableCell>
            <TableCell>
              <StatusBadge :tone="m.supports_images ? 'ok' : 'outline'">{{ visionText(m) }}</StatusBadge>
            </TableCell>
            <TableCell class="num text-right text-[0.8rem]">{{ n(m.context_length) }}</TableCell>
            <TableCell class="num text-right text-[0.8rem]">
              {{ m.max_output_tokens ? n(m.max_output_tokens) : '—' }}
            </TableCell>
            <TableCell class="text-right">
              <div class="flex items-center justify-end gap-1">
                <Button
                  variant="ghost"
                  size="icon-xs"
                  :title="`复制 ID：${m.id}`"
                  :aria-label="`复制 ID：${m.id}`"
                  @click="copyID(m.id)"
                >
                  <Copy class="size-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  title="更多信息"
                  aria-label="更多信息"
                  @click="detail = m"
                >
                  <Ellipsis class="size-3.5" />
                </Button>
              </div>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </Panel>

    <Panel title="怎么用" class="rise rise-1">
      <p class="text-[0.8rem] leading-relaxed text-muted-foreground">
        任何 OpenAI 兼容客户端把 baseURL 指向网关即可，模型名照上面填（点复制按钮拿到的就是完整 id）：
      </p>
      <pre class="mono mt-4 overflow-x-auto rounded-md border border-border bg-muted/60 px-4 py-3 text-[0.72rem] leading-relaxed">baseURL: http://&lt;网关地址&gt;:7863/v1
apiKey:  &lt;调用密钥页里生成的 key&gt;
model:   {{ sorted[0]?.id || 'cn:deepseek-v4-flash' }}</pre>
    </Panel>

    <Dialog :open="detail !== null" @update:open="detail = null">
      <DialogContent class="max-w-2xl">
        <DialogHeader class="border-b border-border px-6 py-4 pr-14">
          <DialogTitle>{{ detail?.name }}</DialogTitle>
          <DialogDescription class="mono text-[0.68rem]">{{ detail?.id }}</DialogDescription>
        </DialogHeader>

        <div v-if="detail" class="max-h-[68vh] space-y-5 overflow-y-auto px-6 py-5">
          <section>
            <h3 class="text-[0.65rem] font-medium tracking-[0.1em] text-muted-foreground">推理档位</h3>
            <dl class="mt-2 grid gap-x-6 gap-y-2 text-[0.8rem] sm:grid-cols-[9rem_minmax(0,1fr)]">
              <dt class="text-muted-foreground">可传档位</dt>
              <dd>
                <template v-if="detail.reasoning_supported_efforts?.length">
                  {{ detail.reasoning_supported_efforts.join(' / ') }}
                </template>
                <span v-else class="text-muted-foreground">该模型无档位（不支持传 reasoning_effort）</span>
              </dd>
              <dt class="text-muted-foreground">默认档位</dt>
              <dd>{{ detail.reasoning_default_effort || '—' }}</dd>
              <dt class="text-muted-foreground">上游默认 effort</dt>
              <dd>{{ detail.reasoning_effort || '—' }}</dd>
              <dt class="text-muted-foreground">思考摘要</dt>
              <dd>{{ detail.reasoning_summary || '—' }}</dd>
            </dl>
          </section>

          <Separator />

          <section>
            <h3 class="text-[0.65rem] font-medium tracking-[0.1em] text-muted-foreground">能力</h3>
            <ul class="mt-2 divide-y divide-border/70">
              <li
                v-for="c in CAPABILITIES"
                :key="String(c.key)"
                class="flex items-center gap-3 py-2 text-[0.8rem]"
                :class="{ 'text-muted-foreground': !detail[c.key] }"
              >
                <span
                  class="grid size-5 shrink-0 place-items-center rounded-full border"
                  :class="detail[c.key] ? 'border-status-ok/40 bg-status-ok/10 text-status-ok' : 'border-border text-muted-foreground'"
                >
                  <Check v-if="detail[c.key]" class="size-3" />
                  <X v-else class="size-3" />
                </span>
                <span class="w-20 shrink-0">{{ c.label }}</span>
                <span class="text-[0.72rem] text-muted-foreground">{{ c.note }}</span>
              </li>
            </ul>
          </section>

          <Separator />

          <section>
            <h3 class="text-[0.65rem] font-medium tracking-[0.1em] text-muted-foreground">其他字段</h3>
            <dl class="mt-2 grid gap-x-6 gap-y-2 text-[0.8rem] sm:grid-cols-[9rem_minmax(0,1fr)]">
              <dt class="text-muted-foreground">域</dt>
              <dd>{{ realmText(detail.realm) }}</dd>
              <dt class="text-muted-foreground">积分倍率</dt>
              <dd>{{ detail.credits || '上游未给' }}</dd>
              <dt class="text-muted-foreground">上下文长度</dt>
              <dd>{{ n(detail.context_length) }}</dd>
              <dt class="text-muted-foreground">单次最大输出</dt>
              <dd>{{ detail.max_output_tokens ? n(detail.max_output_tokens) : '—' }}</dd>
              <template v-for="f in extraFields(detail)" :key="f.k">
                <dt class="mono text-[0.72rem] text-muted-foreground">{{ f.k }}</dt>
                <dd class="break-words">{{ f.v }}</dd>
              </template>
            </dl>
          </section>
        </div>

        <DialogFooter class="border-t border-border px-6 py-3.5">
          <Button variant="outline" size="sm" @click="detail && copyID(detail.id)">
            <Copy class="size-3.5" />
            复制 ID
          </Button>
          <DialogClose as-child>
            <Button size="sm">关闭</Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
