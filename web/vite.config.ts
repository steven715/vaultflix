import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    VitePWA({
      registerType: 'prompt', // 不自動 reload，避免播放中被打斷
      devOptions: { enabled: false }, // dev 不啟用，npm run dev 保持乾淨
      manifest: {
        name: 'Vaultflix',
        short_name: 'Vaultflix',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        background_color: '#0D0B0A', // 對齊 @theme --color-bg
        theme_color: '#0D0B0A',
        icons: [
          { src: '/pwa-192.png', sizes: '192x192', type: 'image/png' },
          { src: '/pwa-512.png', sizes: '512x512', type: 'image/png' },
          { src: '/maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      workbox: {
        globPatterns: ['**/*.{js,css,html,svg,woff2}'], // 只 precache 靜態殼
        navigateFallback: '/index.html',
        navigateFallbackDenylist: [/^\/api\//, /^\/minio\//],
        // 鐵則：API / MinIO 一律 NetworkOnly，SW 絕不碰 /api（尤其 /stream）與 /minio
        runtimeCaching: [
          {
            urlPattern: ({ url }) =>
              url.pathname.startsWith('/api') || url.pathname.startsWith('/minio'),
            handler: 'NetworkOnly',
          },
        ],
      },
    }),
  ],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
