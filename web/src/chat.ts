// chat.ts 面板对话用的 OpenAI 客户端。
//
// baseURL 用 /api/admin/v1 而不是数据面的 /v1：面板手里只有 admin token，没有调用
// key，后端会把这个请求代理进数据面并换成真实 key（见 internal/admin/chat.go）。
// 带 v1 前缀是为了跟 /api/admin/* 那批自定义信封的端点分开——那些返回的不是
// OpenAI 格式（如 /models 的 {models:[...]}），混在同一前缀下 SDK 会解析错。

import OpenAI from 'openai'
import { tokenStore } from './api'

export function chatClient(): OpenAI {
  return new OpenAI({
    // 必须给绝对地址：SDK 内部用 new URL(baseURL) 拼请求地址（openai v7 的
    // client.js:483），相对路径会直接抛 "Failed to construct 'URL': Invalid URL"。
    // 用 location.origin 补全，仍是同源请求，不涉及 CORS。
    baseURL: new URL('/api/admin/v1', window.location.origin).href,
    // 面板未启用鉴权时 token 为空，而 SDK 不接受空 apiKey：用占位串（服务端此时也不校验）。
    apiKey: tokenStore.get() || 'unused',
    // SDK 默认拒绝在浏览器里跑，怕把服务端 key 泄露给前端。这里的 key 就是用户刚在
    // 登录页输入、本来就存在本页 localStorage 里的 admin token，不构成新增暴露。
    dangerouslyAllowBrowser: true,
  })
}
