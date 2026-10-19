import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// 产物直接落进 internal/admin/dist，由 go:embed 打进二进制；
// base 固定 /admin/，与 Go 侧路由前缀一致（换前缀要两边同时改）。
export default defineConfig({
  base: '/admin/',
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      // 用 '@/xxx' 而不是 '@xxx'：后者会抢在 Vite 内部别名之前匹配 @vite/client。
      '@/': `${fileURLToPath(new URL('./src', import.meta.url))}/`,
    },
  },
  build: {
    outDir: '../internal/admin/dist',
    emptyOutDir: true,
    // 单文件上限调高一点：内嵌资源不需要 CDN 分包优化。
    chunkSizeWarningLimit: 800,
  },
  server: {
    // 开发期直接代理到本机网关，前端跑 vite dev 也能吃真实数据。
    proxy: {
      '/api/admin': {
        target: 'http://127.0.0.1:7863',
        changeOrigin: true,
      },
    },
  },
})
