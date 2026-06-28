import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// jsdom + RTL：元件渲染/互動測試。純函式測試在同一環境照跑。
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
