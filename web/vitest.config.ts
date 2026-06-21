import { defineConfig } from 'vitest/config'

// Minimal, build-config-independent: the pure helpers under test take plain
// data, so a node environment (no jsdom) is enough and we avoid loading the
// react / tailwind plugins from vite.config.ts.
export default defineConfig({
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
