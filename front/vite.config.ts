import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      // 开发代理：/api → server（默认 8090，可用 env VITE_API_TARGET 覆盖）
      '/api': {
        target: process.env.VITE_API_TARGET || 'http://127.0.0.1:8090',
        changeOrigin: true,
      },
    },
  },
})
