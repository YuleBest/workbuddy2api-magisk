<script setup lang="ts">
// 调用密钥：增删 keys.json 里的 key。热生效，不需要重启网关。
// 新建的 key 只在创建响应里出现一次完整值，之后一律掩码。
import { ref } from 'vue'
import { api, type KeyInfo } from '../api'
import { guard, toast } from '../store'
import { usePolling } from '../composables/usePolling'
import PageHead from '../components/PageHead.vue'

const keys = ref<KeyInfo[]>([])
const file = ref('')
const authRequired = ref(true)
const note = ref('')
const busy = ref(false)
const created = ref('')
const revoking = ref('')

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

async function revoke(k: KeyInfo) {
  if (!window.confirm(`吊销 ${k.masked}？${k.note ? `（${k.note}）` : ''}\n用这个 key 的客户端会立刻开始 401。`)) return
  revoking.value = k.prefix
  const res = await guard(() => api.revokeKey(k.prefix))
  revoking.value = ''
  if (res) toast(`已吊销，剩 ${res.remaining} 个 key`)
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
      lede="客户端用这些 key 访问 /v1 接口。改动即时生效：新增的立刻能用，吊销的立刻 401，都不需要重启网关。"
    >
      <template #actions>
        <span class="muted small">{{ keys.length }} 个 key</span>
      </template>
    </PageHead>

    <div v-if="!authRequired" class="notice">
      <strong>网关当前不鉴权</strong>：keys.json 与 config 的 api_key 都是空的，任何人都能直接调用接口。
      公网部署的话，先在下面建一个 key。
    </div>

    <div v-if="created" class="notice ok">
      <strong>新 key 已生成，只显示这一次</strong>
      <div class="row wrap" style="margin-top: 8px">
        <code style="padding: 4px 8px">{{ created }}</code>
        <button type="button" @click="copy">复制</button>
        <button class="ghost" type="button" @click="created = ''">我已保存</button>
      </div>
    </div>

    <section class="panel">
      <div class="panel-head">
        <h2>新建 key</h2>
        <span class="note">备注只是给你自己看的，方便以后认出这是哪台设备</span>
      </div>
      <div class="panel-body">
        <form class="row wrap" @submit.prevent="create">
          <input v-model="note" type="text" placeholder="备注，例如：公司笔记本 / 手机 / VSCode" style="flex: 1 1 260px" />
          <button class="primary" type="submit" :disabled="busy">{{ busy ? '生成中…' : '生成 key' }}</button>
        </form>
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <h2>现有 key</h2>
        <span v-if="file" class="note">来自 {{ file }}</span>
      </div>

      <div v-if="!keys.length" class="empty">
        <strong>一个 key 都没有</strong>
        没有 key 意味着网关不校验鉴权，任何知道地址的人都能调用。建议至少建一个。
      </div>

      <table v-else class="data">
        <thead>
          <tr>
            <th>key</th>
            <th>备注</th>
            <th>添加日期</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="k in keys" :key="k.prefix">
            <td class="mono">{{ k.masked }}</td>
            <td>{{ k.note || '—' }}</td>
            <td class="num muted">{{ k.added || '—' }}</td>
            <td class="n">
              <button class="danger" type="button" :disabled="revoking === k.prefix" @click="revoke(k)">吊销</button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>