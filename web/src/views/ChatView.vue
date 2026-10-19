<script setup lang="ts">
// 模型对话：选一个模型，直接在面板里聊。
//
// 请求走 /api/admin/v1/chat/completions（后端代理进数据面），所以这里的对话和真实
// 客户端请求同构：进请求日志、进调用统计、出错会冷却账号并自动换号。池子满时这里
// 同样会收到 503 —— 那是真实语义，不是本页的 bug。
import { computed, nextTick, ref } from 'vue'
import { useRouter } from 'vue-router'
import { APIError } from 'openai'
import { CircleAlert, MessageSquare, RefreshCw, Send, Square, Trash2 } from '@lucide/vue'
import { api, type ModelInfo } from '../api'
import { chatClient } from '../chat'
import { renderMarkdown } from '../markdown'
import { guard, logout, toast } from '../store'
import PageHead from '../components/PageHead.vue'
import Panel from '../components/Panel.vue'
import EmptyState from '../components/EmptyState.vue'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { n } from '../format'

interface Msg {
  role: 'user' | 'assistant'
  content: string
  /** 正在流式接收：用于显示光标、并在构造下一轮历史时排除自己。 */
  streaming?: boolean
}

const router = useRouter()

const models = ref<ModelInfo[]>([])
/** 模型清单是否已经拉过一轮：没拉完之前不能下"没有可用模型"的结论（冷缓存首拉要等几秒）。 */
const modelsLoaded = ref(false)
const modelsBusy = ref(false)
const model = ref('')
const messages = ref<Msg[]>([])
const input = ref('')
const busy = ref(false)
/** 正在等思考模型吐正文：只做状态提示，不展示思考内容本身。 */
const thinking = ref(false)
const error = ref('')
const scroller = ref<HTMLElement | null>(null)
let ctrl: AbortController | null = null

interface Group {
  realm: string
  label: string
  items: ModelInfo[]
}

// id 形如 "cn:xxx" / "global:xxx"：按域分组，组内按显示名排序。
const groups = computed<Group[]>(() => {
  const buckets = new Map<string, ModelInfo[]>()
  for (const m of models.value) {
    const realm = m.id.includes(':') ? m.id.split(':')[0] : 'cn'
    const list = buckets.get(realm)
    if (list) list.push(m)
    else buckets.set(realm, [m])
  }
  const realmLabel = (realm: string) => (realm === 'global' ? '国际' : '国内')
  return [...buckets.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([realm, items]) => ({
      realm,
      label: `${realmLabel(realm)}（${items.length}）`,
      items: [...items].sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id, 'zh')),
    }))
})

const current = computed(() => models.value.find((m) => m.id === model.value))

const modelPlaceholder = computed(() =>
  modelsLoaded.value ? '没有可用模型' : '正在拉取模型清单…',
)

// 模型清单变化很慢（网关侧缓存 1 小时），进页面拉一次即可。
// 冷缓存时网关要去上游现拉，可能好几秒——这段窗口里清单是空的，别当成"没有账号"。
async function load() {
  modelsBusy.value = true
  const res = await guard(() => api.models())
  modelsBusy.value = false
  modelsLoaded.value = true
  if (!res) return
  models.value = res.models
  if (!models.value.some((m) => m.id === model.value)) {
    model.value = models.value[0]?.id ?? ''
  }
}

function scrollDown() {
  void nextTick(() => {
    const el = scroller.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

function stop() {
  ctrl?.abort()
}

function reset() {
  stop()
  messages.value = []
  error.value = ''
}

/** Enter 发送、Shift+Enter 换行；isComposing 时放行（否则中文输入法选词的回车会误发）。 */
function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'Enter' || e.shiftKey || e.isComposing) return
  e.preventDefault()
  void send()
}

async function send() {
  const text = input.value.trim()
  if (!text || busy.value || !model.value) return
  error.value = ''
  input.value = ''
  messages.value.push({ role: 'user', content: text })
  messages.value.push({ role: 'assistant', content: '', streaming: true })
  // 取回数组里的响应式引用：直接改 push 进去的那个裸对象不会触发重渲染。
  const reply = messages.value[messages.value.length - 1]
  scrollDown()

  busy.value = true
  // 认 signal.aborted 而不是错误类型：SDK 中断抛的是 APIUserAbortError，它继承
  // APIError 却不设 name='AbortError'（openai v7 core/error.js），按名字判会漏。
  const ac = new AbortController()
  ctrl = ac
  let sawReasoning = false
  let aborted = false
  try {
    const stream = await chatClient().chat.completions.create(
      {
        model: model.value,
        // 每轮都发完整历史（网关无状态）；网关按 messages 推导粘性会话，
        // 所以同一段对话会一直落在同一个账号上。空内容的消息（含本轮占位）不进历史。
        messages: messages.value
          .filter((m) => m.content !== '')
          .map((m) =>
            m.role === 'user'
              ? { role: 'user' as const, content: m.content }
              : { role: 'assistant' as const, content: m.content },
          ),
        stream: true,
      },
      { signal: ac.signal },
    )
    for await (const chunk of stream) {
      // 上游出错时网关会写一个不含 choices 的错误帧：跳过它，而不是让整段回答炸掉。
      // reasoning_content 不在 SDK 的类型里，但网关会原样透传思考帧，这里只用它判断
      // "还在思考"——实测思考模型要先吐 200+ 帧思考（6 秒以上）才开始出正文。
      const delta: { content?: string | null; reasoning_content?: string | null } | undefined =
        chunk.choices?.[0]?.delta
      if (delta?.reasoning_content && !reply.content) {
        sawReasoning = true
        thinking.value = true
      }
      if (delta?.content) {
        thinking.value = false
        reply.content += delta.content
        scrollDown()
      }
    }
  } catch (e) {
    if (ac.signal.aborted) {
      // 用户点了停止：保留已生成的部分，不当错误报。
      aborted = true
    } else if (e instanceof APIError && e.status === 401) {
      logout()
      toast('管理 token 已失效，请重新登录', 'warn')
      void router.push({ name: 'login' })
    } else if (e instanceof APIError) {
      error.value = e.status ? `请求失败（HTTP ${e.status}）：${e.message}` : e.message
    } else if (e instanceof Error) {
      error.value = e.message
    } else {
      error.value = '请求失败'
    }
  } finally {
    reply.streaming = false
    busy.value = false
    thinking.value = false
    ctrl = null
    scrollDown()
  }

  if (aborted) {
    // 主动停止且一个字都没出：把空气泡收掉，别留一个空框外加一句误导性的报错。
    if (!reply.content) messages.value.pop()
  } else if (!reply.content) {
    error.value = sawReasoning
      ? '模型只输出了思考内容、没有正文（通常是输出上限被思考占满，让它直接回答即可）'
      : '模型没有返回内容（可能被上游拦截，或流被中断）'
  }
}

void load()
</script>

<template>
  <section class="view">
    <PageHead
      title="模型对话"
      lede="选一个模型直接对话。请求走网关数据面，与真实客户端同构，池子忙时同样返回 503。"
    >
      <template #actions>
        <Button variant="outline" size="sm" :disabled="modelsBusy" @click="load">
          <RefreshCw class="size-3.5" />
          {{ modelsBusy ? '拉取中…' : '刷新模型' }}
        </Button>
        <Button
          variant="outline"
          size="sm"
          :disabled="busy || messages.length === 0"
          @click="reset"
        >
          <Trash2 class="size-3.5" />
          清空对话
        </Button>
      </template>
    </PageHead>

    <Panel class="rise" flush>
      <div class="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-border px-5 py-3 sm:px-6">
        <span class="text-[0.7rem] text-muted-foreground">模型</span>
        <Select v-model="model" :disabled="busy || models.length === 0">
          <SelectTrigger class="h-8 w-full text-xs sm:w-[22rem]" aria-label="选择模型">
            <SelectValue :placeholder="modelPlaceholder" />
          </SelectTrigger>
          <SelectContent position="popper" class="max-h-[18rem]">
            <SelectGroup v-for="g in groups" :key="g.realm">
              <SelectLabel>{{ g.label }}</SelectLabel>
              <SelectItem v-for="m in g.items" :key="m.id" :value="m.id">
                {{ m.name || m.id }}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <span class="mono ml-auto hidden text-[0.68rem] text-muted-foreground sm:inline">
          {{ model || '未选择模型' }}
        </span>
      </div>

      <div
        ref="scroller"
        :class="messages.length === 0 ? 'justify-center' : ''"
        class="flex max-h-[58vh] min-h-[22rem] flex-col gap-5 overflow-y-auto px-5 py-5 sm:px-6"
      >
        <EmptyState v-if="messages.length === 0" :icon="MessageSquare" title="还没有对话">
          <template v-if="!modelsLoaded">
            正在拉取模型清单（冷缓存时网关要现去上游拉，稍等几秒）…
          </template>
          <template v-else-if="models.length === 0">
            模型清单是空的：网关一个可接活账号都没有，先看「账号池」。
          </template>
          <template v-else>选好模型，在下面输入一句话开始。</template>
        </EmptyState>

        <template v-for="(m, i) in messages" :key="i">
          <div v-if="m.role === 'user'" class="flex justify-end">
            <div
              class="max-w-[85%] whitespace-pre-wrap rounded-lg rounded-br-sm border border-primary/25 bg-primary/10 px-4 py-2.5 text-[0.84rem] leading-relaxed"
            >
              {{ m.content }}
            </div>
          </div>

          <div v-else class="border-l-2 border-primary/30 pl-4 sm:pl-5">
            <p class="mono text-[0.65rem] tracking-[0.12em] text-muted-foreground">模型</p>
            <!-- 助手回复按 markdown 渲染（html:false，裸 HTML 被转义成字面文本）；用户输入按纯文本。 -->
            <div
              v-if="m.content"
              class="chat-markdown mt-1.5"
              v-html="renderMarkdown(m.content)"
            ></div>
            <div v-if="m.streaming" class="mt-2 flex items-center gap-2">
              <span class="caret" aria-hidden="true"></span>
              <span v-if="thinking" class="text-[0.75rem] text-muted-foreground">思考中…</span>
            </div>
          </div>
        </template>

        <Alert v-if="error" variant="destructive">
          <CircleAlert class="size-4" />
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>
      </div>

      <form class="border-t border-border px-5 py-4 sm:px-6" @submit.prevent="send">
        <Textarea
          v-model="input"
          rows="3"
          class="min-h-[5.25rem] resize-none"
          placeholder="Enter 发送，Shift+Enter 换行"
          :disabled="busy || !model"
          @keydown="onKeydown"
        ></Textarea>
        <div class="mt-3 flex flex-wrap items-center gap-3">
          <span class="text-[0.72rem] text-muted-foreground">
            {{
              current
                ? `${current.name || current.id} · 上下文 ${n(current.context_length)}`
                : '未选择模型'
            }}
          </span>
          <span class="flex-1"></span>
          <Button v-if="busy" variant="destructive" size="sm" type="button" @click="stop">
            <Square class="size-3.5" />
            停止
          </Button>
          <Button v-else type="submit" size="sm" :disabled="!input.trim() || !model">
            <Send class="size-3.5" />
            发送
          </Button>
        </div>
      </form>
    </Panel>
  </section>
</template>
