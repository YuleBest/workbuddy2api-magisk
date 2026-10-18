<script setup lang="ts">
// 模型：上游当前开放的模型清单。数据由网关动态拉取（缓存 1 小时），纯动态无静态兜底，
// 一个可用账号都没有时会是空列表。
//
// 列的选择原则：只放"选模型时会用来判断"的字段（域 / 倍率 / 能不能识图 / 上下文与输出上限），
// 其余（推理档位、能力细节、上游描述等）收进「更多」弹窗——表格宽了反而不好扫。
import { computed, ref } from 'vue'
import { api, type ModelInfo } from '../api'
import { guard, toast } from '../store'
import PageHead from '../components/PageHead.vue'
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
function creditRate(raw?: string): { text: string; tone: string } {
  const v = (raw ?? '').trim()
  if (!v) return { text: '—', tone: '' }
  const hit = /x\s*([0-9]+(?:\.[0-9]+)?)/i.exec(v)
  if (!hit) return { text: v, tone: '' }
  const val = Number(hit[1])
  if (val === 0) return { text: 'x0.00', tone: 'ok' }
  if (val >= 1) return { text: `x${val.toFixed(2)}`, tone: 'warn' }
  return { text: `x${val.toFixed(2)}`, tone: '' }
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
        <span class="muted small">{{ models.length }} 个模型</span>
        <button type="button" :disabled="busy" @click="load">{{ busy ? '拉取中…' : '刷新' }}</button>
      </template>
    </PageHead>

    <section class="panel">
      <div class="panel-head">
        <h2>可用模型</h2>
        <input
          v-model="query"
          class="search"
          type="search"
          placeholder="搜索模型名或 ID"
          aria-label="搜索模型"
        />
        <span v-if="query" class="note">匹配 {{ filtered.length }} / {{ sorted.length }}</span>
        <span class="spacer"></span>
        <span v-if="loadedAt" class="note">更新于 {{ new Date(loadedAt).toLocaleTimeString('zh-CN') }}</span>
      </div>

      <div v-if="!sorted.length" class="empty">
        <strong>没拿到模型列表</strong>
        模型表是纯动态拉取的：网关会向池内任一健康账号请求，池里没有可用账号时这里就是空的。先看「总览」的接活状态。
      </div>
      <div v-else-if="!view.length" class="empty">
        <strong>没有匹配「{{ query }}」的模型</strong>
        换个关键词，或者
        <button class="ghost" type="button" @click="query = ''">清空搜索</button>
      </div>

      <div v-else class="table-scroll">
        <table class="data models">
        <thead>
          <tr>
            <th>
              <button class="sort" type="button" title="按模型名排序" @click="toggleSort('name')">
                模型
                <svg class="sort-icon" :class="{ on: sortKey === 'name' }" width="9" height="11" viewBox="0 0 10 12" aria-hidden="true">
                  <path v-if="sortKey === 'name'" :d="sortDir === 'asc' ? 'M1 8 5 3 9 8' : 'M1 4 5 9 9 4'" />
                  <path v-else d="M1 4.5 5 1.5 9 4.5M1 7.5 5 10.5 9 7.5" />
                </svg>
              </button>
            </th>
            <th>域</th>
            <th class="n">
              <button class="sort" type="button" title="按积分倍率排序（缺倍率的排最后）" @click="toggleSort('rate')">
                积分倍率
                <svg class="sort-icon" :class="{ on: sortKey === 'rate' }" width="9" height="11" viewBox="0 0 10 12" aria-hidden="true">
                  <path v-if="sortKey === 'rate'" :d="sortDir === 'asc' ? 'M1 8 5 3 9 8' : 'M1 4 5 9 9 4'" />
                  <path v-else d="M1 4.5 5 1.5 9 4.5M1 7.5 5 10.5 9 7.5" />
                </svg>
              </button>
            </th>
            <th>能力</th>
            <th class="n">
              <button class="sort" type="button" title="按上下文长度排序" @click="toggleSort('context')">
                上下文长度
                <svg class="sort-icon" :class="{ on: sortKey === 'context' }" width="9" height="11" viewBox="0 0 10 12" aria-hidden="true">
                  <path v-if="sortKey === 'context'" :d="sortDir === 'asc' ? 'M1 8 5 3 9 8' : 'M1 4 5 9 9 4'" />
                  <path v-else d="M1 4.5 5 1.5 9 4.5M1 7.5 5 10.5 9 7.5" />
                </svg>
              </button>
            </th>
            <th class="n">
              <button class="sort" type="button" title="按单次最大输出排序（缺值的排最后）" @click="toggleSort('maxout')">
                单次最大输出
                <svg class="sort-icon" :class="{ on: sortKey === 'maxout' }" width="9" height="11" viewBox="0 0 10 12" aria-hidden="true">
                  <path v-if="sortKey === 'maxout'" :d="sortDir === 'asc' ? 'M1 8 5 3 9 8' : 'M1 4 5 9 9 4'" />
                  <path v-else d="M1 4.5 5 1.5 9 4.5M1 7.5 5 10.5 9 7.5" />
                </svg>
              </button>
            </th>
            <th class="n">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in view" :key="m.id">
            <td>
              <div>{{ m.name }}</div>
              <div class="mono small muted">{{ m.id }}</div>
            </td>
            <td>
              <span class="tag">{{ realmText(m.realm) }}</span>
            </td>
            <td class="n">
              <span class="tag" :class="creditRate(m.credits).tone">{{ creditRate(m.credits).text }}</span>
            </td>
            <td>
              <span class="tag" :class="m.supports_images ? 'ok' : ''">{{ visionText(m) }}</span>
            </td>
            <td class="n">{{ n(m.context_length) }}</td>
            <td class="n">{{ m.max_output_tokens ? n(m.max_output_tokens) : '—' }}</td>
            <td class="n">
              <div class="row" style="justify-content: flex-end; gap: 4px">
                <button class="icon" type="button" :title="`复制 ID：${m.id}`" @click="copyID(m.id)">
                  <svg width="13" height="13" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                    <rect x="5.5" y="5.5" width="8" height="9" rx="1.5" stroke="currentColor" />
                    <path d="M10.5 5.5V3.5A1.5 1.5 0 0 0 9 2H4A1.5 1.5 0 0 0 2.5 3.5v7A1.5 1.5 0 0 0 4 12h1.5" stroke="currentColor" />
                  </svg>
                </button>
                <button class="icon" type="button" title="更多信息" @click="detail = m">
                  <svg width="13" height="13" viewBox="0 0 16 16" fill="none" aria-hidden="true">
                    <circle cx="3" cy="8" r="1.3" fill="currentColor" />
                    <circle cx="8" cy="8" r="1.3" fill="currentColor" />
                    <circle cx="13" cy="8" r="1.3" fill="currentColor" />
                  </svg>
                </button>
              </div>
            </td>
          </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>怎么用</h2>
      </div>
      <div class="panel-body">
        <p class="muted" style="margin-top: 0">
          任何 OpenAI 兼容客户端把 baseURL 指向网关即可，模型名照上面填（点复制按钮拿到的就是完整 id）：
        </p>
        <pre style="font-family: var(--mono); font-size: 12px; background: var(--surface-2); border: 1px solid var(--line); border-radius: 3px; padding: 12px; overflow-x: auto; margin: 0">baseURL: http://&lt;网关地址&gt;:7863/v1
apiKey:  &lt;调用密钥页里生成的 key&gt;
model:   {{ sorted[0]?.id || 'cn:deepseek-v4-flash' }}</pre>
      </div>
    </section>
  </div>

  <!-- 模型详情弹窗 -->
  <div v-if="detail" class="modal-backdrop" @click.self="detail = null">
    <div class="modal" role="dialog" aria-modal="true">
      <header class="modal-head">
        <div>
          <strong>{{ detail.name }}</strong>
          <div class="mono small muted">{{ detail.id }}</div>
        </div>
        <span class="spacer"></span>
        <button class="icon" type="button" title="复制 ID" @click="copyID(detail.id)">
          <svg width="13" height="13" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <rect x="5.5" y="5.5" width="8" height="9" rx="1.5" stroke="currentColor" />
            <path d="M10.5 5.5V3.5A1.5 1.5 0 0 0 9 2H4A1.5 1.5 0 0 0 2.5 3.5v7A1.5 1.5 0 0 0 4 12h1.5" stroke="currentColor" />
          </svg>
        </button>
        <button class="ghost" type="button" @click="detail = null">关闭</button>
      </header>

      <div class="modal-body">
        <h3>推理档位</h3>
        <dl class="kv">
          <dt>可传档位</dt>
          <dd>
            <template v-if="detail.reasoning_supported_efforts?.length">
              {{ detail.reasoning_supported_efforts.join(' / ') }}
            </template>
            <span v-else class="muted">该模型无档位（不支持传 reasoning_effort）</span>
          </dd>
          <dt>默认档位</dt>
          <dd>{{ detail.reasoning_default_effort || '—' }}</dd>
          <dt>上游默认 effort</dt>
          <dd>{{ detail.reasoning_effort || '—' }}</dd>
          <dt>思考摘要</dt>
          <dd>{{ detail.reasoning_summary || '—' }}</dd>
        </dl>

        <h3>能力</h3>
        <ul class="caps">
          <li v-for="c in CAPABILITIES" :key="String(c.key)" :class="{ off: !detail[c.key] }">
            <span class="lamp" :class="detail[c.key] ? 'ok' : 'idle'"></span>
            <span>{{ c.label }}</span>
            <span class="muted small">{{ c.note }}</span>
          </li>
        </ul>

        <h3>其他字段</h3>
        <dl class="kv">
          <dt>域</dt>
          <dd>{{ realmText(detail.realm) }}</dd>
          <dt>积分倍率</dt>
          <dd>{{ detail.credits || '上游未给' }}</dd>
          <dt>上下文长度</dt>
          <dd>{{ n(detail.context_length) }}</dd>
          <dt>单次最大输出</dt>
          <dd>{{ detail.max_output_tokens ? n(detail.max_output_tokens) : '—' }}</dd>
          <template v-for="f in extraFields(detail)" :key="f.k">
            <dt class="mono">{{ f.k }}</dt>
            <dd>{{ f.v }}</dd>
          </template>
        </dl>
      </div>
    </div>
  </div>
</template>