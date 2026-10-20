import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  publicDir: process.env.VITE_PUBLIC_DIR ?? 'public',
  server: {
    port: 5173,
  proxy: {
    '/api': {
      target: 'http://127.0.0.1:8081',
      timeout: 0,
      proxyTimeout: 0,
    },
  },
  },
})
