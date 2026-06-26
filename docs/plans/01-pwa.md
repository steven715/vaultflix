# Plan ① — 最小 PWA 化（manifest + 受控 Service Worker）

> 場景：**Feature（純前端）**。建議另開對話實作。
> done = `task test-fast` 綠 + 實機 iPhone 經 ngrok 驗收。

## 目標

讓「加入主畫面」成為真正的 PWA：正確圖示、可靠的 standalone 無框啟動、app shell 快取秒開。

**鐵則：Service Worker 絕不碰 `/api`（尤其 `/stream`）與 `/minio`。**

## 背景（為什麼現在還不是 PWA）

`web/index.html` 目前完全沒有 PWA 設定：無 manifest、無 `apple-mobile-web-app-capable` meta、無 apple-touch-icon，只有一個 `favicon.svg`，`<title>` 還是預設的 `web`。

iOS 從 2008 就能把任何網頁「加入主畫面」，那是書籤捷徑，不是 PWA。PWA 的技術定義 = HTTPS + Web App Manifest + Service Worker，目前只滿足 HTTPS（ngrok）。所以現在拿到的是：截圖圖示、叫做「web」、點開還有網址列的書籤。

## 變更清單

| 檔案 | 動作 |
|---|---|
| `web/package.json` | 加 devDep：`vite-plugin-pwa`、`@vite-pwa/assets-generator` |
| `web/public/` | 放入產生的 icon：`pwa-192.png`、`pwa-512.png`、`maskable-512.png`、`apple-touch-icon-180.png`（從一張 ≥512 來源圖或現有 `favicon.svg` 產） |
| `web/vite.config.ts` | 加 `VitePWA({...})` plugin |
| `web/index.html` | 修 `<title>`、加 iOS meta（`apple-mobile-web-app-capable` 等）+ apple-touch-icon link |
| `web/src/vite-env.d.ts` | 新增，放 `/// <reference types="vite-plugin-pwa/client" />` 讓 tsc 認得虛擬模組 |
| `web/src/components/PWAUpdater.tsx` | 新增，用既有 ToastContext 提示「有新版，點此更新」 |
| `web/src/App.tsx` | 在 ToastProvider 內掛 `<PWAUpdater />` |

## 關鍵實作細節

### vite.config.ts 的 VitePWA 設定（重點是排除清單）

```ts
VitePWA({
  registerType: 'prompt',              // 不自動 reload（避免播放中被打斷）
  devOptions: { enabled: false },      // dev 不啟用，npm run dev 保持乾淨
  manifest: {
    name: 'Vaultflix', short_name: 'Vaultflix',
    start_url: '/', scope: '/', display: 'standalone',
    background_color: '#0D0B0A', theme_color: '#0D0B0A',  // 對齊 @theme --color-bg
    icons: [
      { src: '/pwa-192.png', sizes: '192x192', type: 'image/png' },
      { src: '/pwa-512.png', sizes: '512x512', type: 'image/png' },
      { src: '/maskable-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
    ],
  },
  workbox: {
    globPatterns: ['**/*.{js,css,html,svg,woff2}'],   // 只 precache 靜態殼
    navigateFallback: '/index.html',
    navigateFallbackDenylist: [/^\/api\//, /^\/minio\//],
    runtimeCaching: [                                   // 鐵則：API/MinIO 一律 NetworkOnly
      { urlPattern: ({ url }) => url.pathname.startsWith('/api') || url.pathname.startsWith('/minio'),
        handler: 'NetworkOnly' },
    ],
  },
})
```

### 為什麼這樣排除是安全的（三道保險）

1. WebSocket（`/api/ws`）— SW 的 `fetch` 事件**根本不會對 WS 連線觸發**，天生安全。
2. 影片串流（`/api/videos/:id/stream`）— 是 `<video>` 的非導航請求，不在 precache glob、又被 NetworkOnly 命中 → SW 純放行。
3. `navigateFallbackDenylist` 確保 SPA fallback 不會把 `/api`、`/minio` 導航誤導到 index.html。

### PWAUpdater（接既有 Toast，播放中不強制 reload）

```tsx
import { useRegisterSW } from 'virtual:pwa-register/react'
// needRefresh → 用 useToast() 推一則「有新版本，點此更新」→ onClick: updateServiceWorker(true)
```

### index.html iOS 補強

vite-plugin-pwa 會自動注入 manifest link 與 theme-color，iOS 專屬的要手動加：

```html
<title>Vaultflix</title>
<meta name="apple-mobile-web-app-capable" content="yes" />
<meta name="mobile-web-app-capable" content="yes" />
<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
<link rel="apple-touch-icon" href="/apple-touch-icon-180.png" />
```

## 風險 / Trade-off

- **SW 是雙面刃**：設錯快取會「明明改了卻看到舊版」。緩解：`registerType:'prompt'` + 現有 nginx 的 `location /` 已對 sw.js / index.html 設 `no-cache`、`/assets/` 設 immutable —— 這個快取邊界**剛好正確**，sw.js 永遠抓新、hashed assets 永久快取。
- **iOS standalone 獨立 storage jar**：加到主畫面的 app 與 Safari 不共用 localStorage → 首次啟動可能要重登一次（token 存 localStorage）。屬 iOS 行為，非 bug，先知道即可。
- **離線價值有限**：串流本質要連網，SW 只快取「殼」。這步主要買的是**質感（圖示/standalone）+ 啟動速度**，不是離線看片。投報率最高的其實只有 manifest 那半，SW 是加分。
- **新依賴**：`vite-plugin-pwa` 屬建置工具鏈，需依 CLAUDE.md「不引入未列出第三方依賴（需先討論）」——確認後才加。

## 驗收

- `task test-fast` 綠（tsc 要靠 `vite-env.d.ts` 認虛擬模組、eslint/vitest 不受影響）。
- `npm run build` 後 `dist/` 出現 `manifest.webmanifest` + `sw.js`；`sw.js` 的 precache 清單**不含**任何 `/api`。
- 實機：iPhone Safari 經 ngrok → 加入主畫面 → 圖示正確、名稱是 Vaultflix、點開**無網址列**（standalone）；**播一支影片確認 SW 沒擋串流**（最重要的回歸點）。

## 估時

半天（大半花在產 icon 與實機驗）。
