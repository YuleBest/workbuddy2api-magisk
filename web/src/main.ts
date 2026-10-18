import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { state, setTheme } from './store'
import { api, ApiError, tokenStore } from './api'
import './fonts.css'
import './style.css'


setTheme(state.theme)

// 启动先探一次 /session：既确认 token 是否还有效，也拿到 service/version 与
// "是否需要鉴权"（服务端 token 为空时后台直接可用，不该出现登录页）。
async function bootstrap() {
  try {
    const info = await api.session()
    state.service = info.service
    state.version = info.version
    state.authRequired = info.auth_required
    state.authed = true
  } catch (e) {
    // 401 = token 过期/未登录，交给路由守卫送去登录页；网络不通也照样挂载，
    // 登录页的提交会给出"连不上网关"的明确报错。
    if (e instanceof ApiError && e.status === 401) {
      tokenStore.clear()
      state.authed = false
    }
  }
  createApp(App).use(router).mount('#app')
}

void bootstrap()