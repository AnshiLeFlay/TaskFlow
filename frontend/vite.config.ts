import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '')
  return {
    plugins: [vue()],
    server: {
      port: Number(env.VITE_PORT || 5173),
      proxy: {
        '/api': { target: env.VITE_BACKEND_PROXY || 'http://localhost:8080', changeOrigin: true },
        '/ws': { target: env.VITE_BACKEND_PROXY || 'http://localhost:8080', ws: true, changeOrigin: true },
      },
    },
    test: { environment: 'jsdom' },
  }
})
