<script setup lang="ts">
// 登录页：只做一件事——把这台网关的调用 key 填进来。
// 不用用户名/密码：网关本身就没有账号体系，凭据就是 keys.json 里的 key。
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, ApiError, tokenStore } from '../api'
import { state, toast } from '../store'

const token = ref('')
const busy = ref(false)
const error = ref('')
const route = useRoute()
const router = useRouter()

async function submit() {
  const value = token.value.trim()
  if (!value) {
    error.value = '请输入调用 key'
    return
  }
  busy.value = true
  error.value = ''
  tokenStore.set(value)
  try {
    const info = await api.session()
    state.service = info.service
    state.version = info.version
    state.authRequired = info.auth_required
    state.authed = true
    toast('已登录管理后台')
    const next = typeof route.query.next === 'string' ? route.query.next : '/overview'
    await router.push(next)
  } catch (e) {
    tokenStore.clear()
    state.authed = false
    if (e instanceof ApiError && e.status === 401) {
      error.value = '这个 key 不对：网关拒绝了它。请核对 keys.json 里的值。'
    } else {
      error.value = e instanceof Error ? e.message : '登录失败'
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="gate">
    <div class="card">
      <span class="lamp ok" aria-hidden="true"></span>
      <h1>网关后台</h1>
      <p>填一个调用 key 进入。这个界面能改账号池状态和密钥，和 API key 同级敏感。</p>

      <form @submit.prevent="submit">
        <label class="field" for="token">调用 key</label>
        <input
          id="token"
          v-model="token"
          type="password"
          autocomplete="current-password"
          spellcheck="false"
          placeholder="sk-..."
          :disabled="busy"
        />
        <button class="primary" type="submit" :disabled="busy">
          {{ busy ? '正在验证…' : '进入后台' }}
        </button>
        <p v-if="error" class="err">{{ error }}</p>
      </form>

      <div class="help">
        <p style="margin: 0 0 6px">key 从哪来：</p>
        <p style="margin: 0 0 4px">
          网关本机：<code>wbapi key list</code> 或看 <code>keys.json</code>
        </p>
        <p style="margin: 0">
          改了 <code>admin.token</code> 的话，用那个；没配则回落到 keys.json 第一个 key。
        </p>
      </div>
    </div>
  </div>
</template>