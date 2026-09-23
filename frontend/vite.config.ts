import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '')
  return {
    plugins: [react()],
    // `@/…` imports resolve from src/ (mirrors "paths" in tsconfig.json).
    resolve: { alias: [{ find: /^@\//, replacement: '/src/' }] },
    server: {
      port: 5173,
      proxy: {
        '/api': env.VITE_API_PROXY_TARGET || 'http://localhost:8080',
      },
    },
  }
})
