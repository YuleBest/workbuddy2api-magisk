<script setup lang="ts">
// 调用密钥：增删 keys.json 里的 key。热生效，不需要重启网关。
// 新建的 key 只在创建响应里出现一次完整值，之后一律掩码。
import { computed, ref } from 'vue'
import { CircleCheck, Copy, KeyRound, Plus, ShieldAlert, Trash2 } from '@lucide/vue'
import { api, type KeyInfo } from '../api'
import { guard, toast } from '../store'
import { usePolling } from '../composables/usePolling'
import PageHead from '../components/PageHead.vue'
import Panel from '../components/Panel.vue'
import EmptyState from '../components/EmptyState.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const keys = ref<KeyInfo[]>([])
const file = ref('')
const authRequired = ref(true)
const note = ref('')
const busy = ref(false)
const created = ref('')

const revokeTarget = ref<KeyInfo | null>(null)
const revoking = ref(false)
const revokeOpen = computed({
  get: () => revokeTarget.value !== null,
  set: (open: boolean) => {
    if (!open) revokeTarget.value = null
  },
})

usePolling(async () => {
  const res = await guard(() => api.keys())
  if (res) {
    keys.value = res.keys
    file.value = res.file
    authRequired.value = res.auth_required
  }
})

async function create() {
  busy.value = true
  const res = await guard(() => api.createKey(note.value.trim()))
  busy.value = false
  if (res) {
    created.value = res.key
    note.value = ''
    toast('新 key 已生效')
  }
}

async function confirmRevoke() {
  const target = revokeTarget.value
  if (!target) return
  revoking.value = true
  const res = await guard(() => api.revokeKey(target.prefix))
  revoking.value = false
  if (res) {
    revokeTarget.value = null
    toast(`已吊销，剩 ${res.remaining} 个 key`)
  }
}

async function copy() {
  try {
    await navigator.clipboard.writeText(created.value)
    toast('已复制到剪贴板')
  } catch {
    toast('浏览器不允许自动复制，请手动选中复制', 'warn')
  }
}
</script>

<template>
  <div class="view">
    <PageHead
      title="调用密钥"
      lede="客户端用这些 key 访问 /v1 接口。改动即时生效，不需要重启网关：新增的立刻能用，吊销的立刻 401。"
    >
      <template #actions>
        <span class="num text-xs text-muted-foreground">{{ keys.length }} 个 key</span>
      </template>
    </PageHead>

    <Alert v-if="!authRequired" class="mb-5 border-l-status-warn">
      <ShieldAlert class="size-4" />
      <AlertTitle>网关当前不鉴权</AlertTitle>
      <AlertDescription>
        keys.json 与 config 的 api_key 都是空的，任何人都能直接调用接口。公网部署的话，先在下面建一个 key。
      </AlertDescription>
    </Alert>

    <Alert v-if="created" class="mb-5 border-l-status-ok">
      <CircleCheck class="size-4" />
      <AlertTitle>新 key 已生成，只显示这一次</AlertTitle>
      <div class="col-start-2 mt-2 flex flex-wrap items-center gap-2">
        <code class="mono rounded border border-border bg-muted px-2.5 py-1.5 text-[0.75rem]">{{ created }}</code>
        <Button variant="outline" size="sm" @click="copy">
          <Copy class="size-3.5" />
          复制
        </Button>
        <Button variant="ghost" size="sm" @click="created = ''">我已保存</Button>
      </div>
    </Alert>

    <Panel title="新建 key" note="备注只是给你自己看的，方便以后认出这是哪台设备" class="rise mb-6">
      <form class="flex w-full max-w-2xl flex-col gap-3 sm:flex-row sm:items-end" @submit.prevent="create">
        <div class="min-w-0 flex-1 space-y-2">
          <Label for="key-note">备注</Label>
          <Input
            id="key-note"
            v-model="note"
            type="text"
            placeholder="例如：公司笔记本 / 手机 / VSCode"
          />
        </div>
        <Button type="submit" :disabled="busy">
          <Plus class="size-4" />
          {{ busy ? '生成中…' : '生成 key' }}
        </Button>
      </form>
    </Panel>

    <Panel
      title="现有 key"
      :note="file ? `来自 ${file}` : ''"
      class="rise rise-1"
      flush
    >
      <EmptyState v-if="!keys.length" :icon="KeyRound" title="一个 key 都没有">
        没有 key 意味着网关不校验鉴权，任何知道地址的人都能调用。建议至少建一个。
      </EmptyState>

      <Table v-else class="min-w-[38rem]">
        <TableHeader>
          <TableRow>
            <TableHead>key</TableHead>
            <TableHead>备注</TableHead>
            <TableHead>添加日期</TableHead>
            <TableHead class="text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="k in keys" :key="k.prefix">
            <TableCell class="mono text-[0.8rem]">{{ k.masked }}</TableCell>
            <TableCell class="text-[0.82rem]">{{ k.note || '—' }}</TableCell>
            <TableCell class="num text-[0.78rem] text-muted-foreground">{{ k.added || '—' }}</TableCell>
            <TableCell class="text-right">
              <Button
                variant="ghost"
                size="sm"
                class="text-muted-foreground hover:text-destructive"
                :disabled="revoking && revokeTarget?.prefix === k.prefix"
                @click="revokeTarget = k"
              >
                <Trash2 class="size-3.5" />
                吊销
              </Button>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </Panel>

    <ConfirmDialog
      v-model:open="revokeOpen"
      title="吊销这个 key"
      :description="
        revokeTarget
          ? `${revokeTarget.masked}${revokeTarget.note ? `（${revokeTarget.note}）` : ''} 吊销后，用它的客户端会立刻开始 401。`
          : ''
      "
      confirm-text="吊销"
      destructive
      :busy="revoking"
      @confirm="confirmRevoke"
    />
  </div>
</template>
