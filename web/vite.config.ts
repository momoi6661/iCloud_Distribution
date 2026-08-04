import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 构建产物输出到 Go embed 目录,实现单二进制部署
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/server/web/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8081',
    },
  },
})
