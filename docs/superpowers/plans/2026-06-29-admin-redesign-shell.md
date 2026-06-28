# Admin 後台重設計 Slice 1 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 Vaultflix admin 後台(`/admin/*`)從舊灰色介面升級為與使用者端「私人放映室」一致的深色視覺系統,加上 `AdminLayout` 殼層(圖示軌側邊欄 + topbar),並把影片庫/精選/使用者/來源四頁改版 + 新增標籤管理頁。純前端,不新增任何業務 API。

**Architecture:** React 18 + TS + Tailwind v4(`@theme` token 已存在於 `web/src/index.css`)。把現有扁平 `/admin/*` 路由改成巢狀 layout route,`AdminLayout`(side rail + topbar + `<Outlet/>`)外層包住所有 admin 頁。新邏輯一律抽成純函式單元(nav config、posterGradient、標籤分組、影片庫排序/篩選/批次的 state 變換),元件用 React Testing Library 測渲染與互動。

**Tech Stack:** React 18, TypeScript, React Router v7, Tailwind CSS v4, Vitest 4 + (新增)@testing-library/react + jsdom。

## Global Constraints

- Go 1.24+ / Node.js 20+(本 slice 只動 `web/`)。
- 每個 `.tsx`/`.ts` 檔 < 300 行;每個 function < 50 行。超過要拆。
- 前端 response 契約:axios interceptor 已解 `{data}` envelope;呼叫端直接拿 `data`,不得 `res.data.data`。
- 設計 token 一律走 Tailwind v4 `@theme` class(`bg-bg`/`text-cream`/`text-accent`/`rounded-card`/`font-display`/`font-mono` 等),不寫死 hex、不用 inline style。
- 不引入計畫外第三方依賴(本計畫已明列:`@testing-library/react`、`@testing-library/jest-dom`、`@testing-library/user-event`、`jsdom`)。
- 自動重試/事件驅動更新要有中斷條件;`useEffect` async 用 cleanup flag。
- 完成條件:`task verify`(= `task test-fast`:`go vet` + `gofmt` + `go test ./...` + web `tsc` + `eslint` zero error/warning + `vitest`)綠。本 slice 不碰 import/影片掃描/串流,免 `task test-integration`。
- 提交訊息走 `<type>: <description>`(feat/fix/refactor/docs/chore/test)。
- 分支:`feat/admin-redesign-shell`(已建立)。spec:`docs/superpowers/specs/2026-06-29-admin-redesign-shell-design.md`。

---

## File Structure

**新增**
- `web/src/test/setup.ts` — vitest setup(jest-dom matchers)。
- `web/src/test/renderWithRouter.tsx` — 測試用 render helper(包 `MemoryRouter`)。
- `web/src/lib/posterGradient.ts` — id 雜湊 → 8 色漸層 fallback(純函式)。
- `web/src/lib/posterGradient.test.ts`
- `web/src/lib/adminNav.ts` — 側邊欄 nav config + active/title 純函式。
- `web/src/lib/adminNav.test.ts`
- `web/src/lib/libraryParams.ts` — 影片庫排序/標籤/批次選取的純 state 變換。
- `web/src/lib/libraryParams.test.ts`
- `web/src/lib/tagGroups.ts` — 標籤依 category 分組(純函式)。
- `web/src/lib/tagGroups.test.ts`
- `web/src/components/admin/AdminLayout.tsx` — 殼層(side rail + topbar + Outlet)。
- `web/src/components/admin/AdminLayout.test.tsx`
- `web/src/components/admin/AdminSidebar.tsx` — 78px 圖示軌。
- `web/src/components/admin/AdminSidebar.test.tsx`
- `web/src/components/admin/AdminTopbar.tsx` — topbar(麵包屑 + 搜尋 + 狀態 + 使用者)。
- `web/src/components/admin/AdminTopbar.test.tsx`
- `web/src/components/admin/NavIcon.tsx` — inline SVG icon 集(by key)。
- `web/src/components/admin/PosterThumb.tsx` — 縮圖(真圖優先、缺圖走漸層)。
- `web/src/pages/admin/TagManagePage.tsx` — 全新標籤管理頁。
- `web/src/pages/admin/TagManagePage.test.tsx`

**修改**
- `web/vitest.config.ts` — 改 jsdom 環境 + react plugin + setupFiles。
- `web/package.json` — 新增 4 個 devDependencies。
- `web/src/index.css` — `@theme` 新增 3 個 token。
- `web/src/App.tsx` — admin 路由改巢狀 layout route。
- `web/src/pages/admin/VideoManagePage.tsx` → 改版為影片庫(reskin + 排序/標籤篩選/批次)。
- `web/src/pages/admin/RecommendationManagePage.tsx` → reskin。
- `web/src/pages/admin/UserManagePage.tsx` → reskin(從 inline style 改 Tailwind token)。
- `web/src/pages/admin/MediaSourcePage.tsx` → reskin。

**設計參考(只讀,不移植)**:`design_handoff_vaultflix_admin/Vaultflix Admin.dc.html`(高擬真原型)+ `README.md`。

---

## Token 對照表(reskin 一律照此換)

舊頁面用的灰色/靛色 → 新 token class:

| 舊(gray/indigo/inline) | 新 class |
|---|---|
| `bg-gray-950` / `bg-gray-900`(頁底) | `bg-bg` |
| `bg-gray-900`(卡片/側欄/表底) | `bg-surface` |
| `bg-gray-800`(表頭/hover/input/次按鈕) | `bg-surface-2` |
| `border-gray-800` / `border-gray-800/50` | `border-border` |
| `text-white`(主文字) | `text-cream` |
| `text-gray-400`(次文字) | `text-muted` |
| `text-gray-500` / `text-gray-600`(第三層/檔名/時間戳) | `text-faint` |
| `bg-indigo-600 hover:bg-indigo-500`(主 CTA) | `bg-accent text-accent-ink hover:brightness-110` |
| `text-indigo-400`(連結/次強調) | `text-accent` |
| `focus:ring-indigo-500` | `focus:ring-accent` |
| `bg-red-600 hover:bg-red-500`(危險) | `bg-fav text-cream hover:brightness-110` |
| `text-red-400` | `text-fav` |
| `text-green-400` / `#22c55e` | `text-live` |
| `rounded`(按鈕) | `rounded-btn` |
| `rounded-lg`(卡/彈窗) | `rounded-card`(卡)/ `rounded-lg`(彈窗 18px) |
| 標題 `text-xl font-bold` | `text-xl font-bold font-display tracking-tight` |
| metadata(時長/大小/路徑/日期) | 加 `font-mono` |
| 彈窗遮罩 `bg-black/60` | `bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px]` |

reskin 規則:**只換呈現,不動任何 handler / state / API 呼叫 / 既有行為**(modal 開關、`stopPropagation`、debounce、cleanup flag 全部保留)。

---

### Task 1: 測試基建(RTL + jsdom)

把 vitest 從 node 環境升級為 jsdom + React Testing Library,讓後續元件能寫渲染/互動測試。

**Files:**
- Modify: `web/package.json`(devDependencies)
- Modify: `web/vitest.config.ts`
- Create: `web/src/test/setup.ts`
- Create: `web/src/test/renderWithRouter.tsx`
- Create: `web/src/test/smoke.test.tsx`(驗證基建,之後可留作範例)

**Interfaces:**
- Produces: `renderWithRouter(ui: React.ReactElement, opts?: { route?: string; path?: string }): RenderResult & { router }` — 用 `MemoryRouter` 包住 `ui`,`route` 設定初始網址。供所有元件測試使用。

- [ ] **Step 1: 安裝依賴**

Run:
```bash
cd web && npm install -D @testing-library/react@^16 @testing-library/jest-dom@^6 @testing-library/user-event@^14 jsdom@^25
```
Expected: `package.json` devDependencies 多出 4 筆,`npm install` 成功。

- [ ] **Step 2: 改 vitest.config.ts**

```ts
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
```

- [ ] **Step 3: 建立 setup.ts**

```ts
// 擴充 vitest 的 expect 帶 jest-dom matchers（含 TS 型別）
import '@testing-library/jest-dom/vitest'
```

- [ ] **Step 4: 建立 renderWithRouter helper**

```tsx
import { type ReactElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { render, type RenderResult } from '@testing-library/react'

// 包 MemoryRouter，讓需要 router context 的元件可被測試。
export function renderWithRouter(
  ui: ReactElement,
  opts: { route?: string } = {},
): RenderResult {
  const { route = '/admin/library' } = opts
  return render(<MemoryRouter initialEntries={[route]}>{ui}</MemoryRouter>)
}
```

- [ ] **Step 5: 寫 smoke 測試**

```tsx
import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithRouter } from './renderWithRouter'

describe('test infra', () => {
  it('renders into jsdom with router + jest-dom matchers', () => {
    renderWithRouter(<h1>hello admin</h1>)
    expect(screen.getByRole('heading', { name: 'hello admin' })).toBeInTheDocument()
  })
})
```

- [ ] **Step 6: 跑測試,確認綠**

Run: `cd web && npx vitest run src/test/smoke.test.tsx`
Expected: PASS。再跑 `npx vitest run`(全部)確認既有 `playbackStats.test.ts` 仍綠。

- [ ] **Step 7: 確認 typecheck + lint 綠**

Run: `cd web && npx tsc -b && npm run lint`
Expected: 0 error / 0 warning。(若 jest-dom 型別未生效,確認 setup.ts 在 `src/` 下被 tsc 納入。)

- [ ] **Step 8: Commit**

```bash
git add web/package.json web/package-lock.json web/vitest.config.ts web/src/test
git commit -m "test: add RTL + jsdom test infra for admin redesign"
```

---

### Task 2: 設計 token 補齊

`@theme` 新增 reskin 會用到的 3 個 token。

**Files:**
- Modify: `web/src/index.css`(`@theme` 區塊,line 5-40 內)

**Interfaces:**
- Produces: Tailwind class `bg-surface-2` / `text-surface-2`、`text-data-blue`/`bg-data-blue`、`text-data-purple`/`bg-data-purple`。綠/紅沿用既有 `live`/`fav`。

- [ ] **Step 1: 在 `@theme` 的 Colors 區塊末尾(line 17 `--color-live` 之後)新增**

```css
  --color-surface-2: #1A1512;
  --color-data-blue: #43C6FF;
  --color-data-purple: #B15CFF;
```

- [ ] **Step 2: 確認 Tailwind 能解析(typecheck 不涉 CSS,改用 build 驗證)**

Run: `cd web && npx vite build`
Expected: build 成功(token 被 `@theme` 接受,無 CSS 解析錯誤)。

- [ ] **Step 3: Commit**

```bash
git add web/src/index.css
git commit -m "feat: add surface-2 + data-viz color tokens for admin redesign"
```

---

### Task 3: posterGradient 純函式

由 video id 穩定雜湊出 8 色盤之一的 135deg 漸層,作為缺縮圖時的 poster fallback。

**Files:**
- Create: `web/src/lib/posterGradient.ts`
- Test: `web/src/lib/posterGradient.test.ts`

**Interfaces:**
- Produces: `posterGradient(id: string): string` — 回傳 CSS `linear-gradient(...)` 字串,同 id 必回同值。

- [ ] **Step 1: 寫失敗測試**

```ts
import { describe, it, expect } from 'vitest'
import { posterGradient } from './posterGradient'

describe('posterGradient', () => {
  it('is deterministic for the same id', () => {
    expect(posterGradient('abc')).toBe(posterGradient('abc'))
  })
  it('always returns a linear-gradient string', () => {
    for (const id of ['', 'x', 'a-very-long-uuid-1234', '中文']) {
      expect(posterGradient(id)).toMatch(/^linear-gradient\(135deg,/)
    }
  })
  it('spreads different ids across the palette (not all identical)', () => {
    const ids = Array.from({ length: 40 }, (_, i) => `id-${i}`)
    const distinct = new Set(ids.map(posterGradient))
    expect(distinct.size).toBeGreaterThan(3)
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/lib/posterGradient.test.ts`
Expected: FAIL（`posterGradient` 未定義）。

- [ ] **Step 3: 實作**

```ts
// 缺縮圖時的 poster placeholder：由 id 穩定雜湊取 8 色盤之一（與設計稿一致）。
const POSTER_GRADIENTS: readonly string[] = [
  'linear-gradient(135deg, #FF8A3D 0%, #7A1F4B 100%)',
  'linear-gradient(135deg, #3E7BFF 0%, #0B1A3A 100%)',
  'linear-gradient(135deg, #1FB588 0%, #06241C 100%)',
  'linear-gradient(135deg, #FFC83D 0%, #7A3B00 100%)',
  'linear-gradient(135deg, #B15CFF 0%, #1A0B33 100%)',
  'linear-gradient(135deg, #FF5470 0%, #2A0810 100%)',
  'linear-gradient(135deg, #43C6FF 0%, #062033 100%)',
  'linear-gradient(135deg, #9BD64B 0%, #16280A 100%)',
]

export function posterGradient(id: string): string {
  let hash = 0
  for (let i = 0; i < id.length; i++) {
    hash = (hash * 31 + id.charCodeAt(i)) | 0
  }
  const idx = Math.abs(hash) % POSTER_GRADIENTS.length
  return POSTER_GRADIENTS[idx]
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `cd web && npx vitest run src/lib/posterGradient.test.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/posterGradient.ts web/src/lib/posterGradient.test.ts
git commit -m "feat: add posterGradient fallback util"
```

---

### Task 4: adminNav 純函式(nav config + active/title)

側邊欄資料來源 + active 判定 + 麵包屑標題,全抽成可測純函式。

**Files:**
- Create: `web/src/lib/adminNav.ts`
- Test: `web/src/lib/adminNav.test.ts`

**Interfaces:**
- Produces:
  - `interface AdminNavItem { key: string; label: string; path: string; enabled: boolean; icon: AdminIconKey }`
  - `type AdminIconKey = 'dashboard' | 'video' | 'star' | 'tag' | 'folder' | 'users' | 'chart'`
  - `const ADMIN_NAV: readonly AdminNavItem[]`（7 項,`dashboard`/`analytics` `enabled:false`）
  - `isNavItemActive(itemPath: string, currentPath: string): boolean`
  - `adminPageTitle(currentPath: string): string`

- [ ] **Step 1: 寫失敗測試**

```ts
import { describe, it, expect } from 'vitest'
import { ADMIN_NAV, isNavItemActive, adminPageTitle } from './adminNav'

describe('ADMIN_NAV', () => {
  it('has 7 items in design order with dashboard + analytics disabled', () => {
    expect(ADMIN_NAV.map((n) => n.key)).toEqual([
      'dashboard', 'library', 'recommendations', 'tags', 'media-sources', 'users', 'analytics',
    ])
    expect(ADMIN_NAV.find((n) => n.key === 'dashboard')!.enabled).toBe(false)
    expect(ADMIN_NAV.find((n) => n.key === 'analytics')!.enabled).toBe(false)
    expect(ADMIN_NAV.find((n) => n.key === 'library')!.enabled).toBe(true)
  })
})

describe('isNavItemActive', () => {
  it('treats /admin as the library route', () => {
    expect(isNavItemActive('/admin/library', '/admin')).toBe(true)
    expect(isNavItemActive('/admin/library', '/admin/library')).toBe(true)
  })
  it('matches exact path and sub-paths', () => {
    expect(isNavItemActive('/admin/users', '/admin/users')).toBe(true)
    expect(isNavItemActive('/admin/media-sources', '/admin/media-sources')).toBe(true)
    expect(isNavItemActive('/admin/users', '/admin/tags')).toBe(false)
  })
})

describe('adminPageTitle', () => {
  it('returns library label for /admin and /admin/library', () => {
    expect(adminPageTitle('/admin')).toBe('影片')
    expect(adminPageTitle('/admin/library')).toBe('影片')
  })
  it('returns the matching item label', () => {
    expect(adminPageTitle('/admin/tags')).toBe('標籤')
    expect(adminPageTitle('/admin/users')).toBe('帳號')
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/lib/adminNav.test.ts`
Expected: FAIL（模組未定義）。

- [ ] **Step 3: 實作**

```ts
export type AdminIconKey =
  | 'dashboard' | 'video' | 'star' | 'tag' | 'folder' | 'users' | 'chart'

export interface AdminNavItem {
  key: string
  label: string
  path: string
  enabled: boolean
  icon: AdminIconKey
}

// 側邊欄 7 項。dashboard / analytics 尚未實作 → enabled:false（置灰 + 即將推出）。
export const ADMIN_NAV: readonly AdminNavItem[] = [
  { key: 'dashboard', label: '總覽', path: '/admin', enabled: false, icon: 'dashboard' },
  { key: 'library', label: '影片', path: '/admin/library', enabled: true, icon: 'video' },
  { key: 'recommendations', label: '精選', path: '/admin/recommendations', enabled: true, icon: 'star' },
  { key: 'tags', label: '標籤', path: '/admin/tags', enabled: true, icon: 'tag' },
  { key: 'media-sources', label: '來源', path: '/admin/media-sources', enabled: true, icon: 'folder' },
  { key: 'users', label: '帳號', path: '/admin/users', enabled: true, icon: 'users' },
  { key: 'analytics', label: '分析', path: '/admin/analytics', enabled: false, icon: 'chart' },
]

// /admin 是 Dashboard 完成前的暫時 landing，對應到影片庫。
export function isNavItemActive(itemPath: string, currentPath: string): boolean {
  if (itemPath === '/admin/library') {
    return currentPath === '/admin' || currentPath === '/admin/library' ||
      currentPath.startsWith('/admin/library/')
  }
  return currentPath === itemPath || currentPath.startsWith(itemPath + '/')
}

// 麵包屑標題：reverse 讓 library 的特例優先於 dashboard。
export function adminPageTitle(currentPath: string): string {
  const match = [...ADMIN_NAV].reverse().find((n) => isNavItemActive(n.path, currentPath))
  return match ? match.label : '管理後台'
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `cd web && npx vitest run src/lib/adminNav.test.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/adminNav.ts web/src/lib/adminNav.test.ts
git commit -m "feat: add admin nav config + active/title helpers"
```

---

### Task 5: NavIcon + PosterThumb 共用元件

兩個無狀態 presentational 元件:側邊欄 icon 與縮圖(真圖優先、缺圖走漸層)。

**Files:**
- Create: `web/src/components/admin/NavIcon.tsx`
- Create: `web/src/components/admin/PosterThumb.tsx`
- Test: `web/src/components/admin/PosterThumb.test.tsx`

**Interfaces:**
- Produces:
  - `NavIcon({ name, className }: { name: AdminIconKey; className?: string }): JSX.Element` — 回傳對應 inline `<svg>`。
  - `PosterThumb({ id, src, alt, className }: { id: string; src?: string | null; alt?: string; className?: string }): JSX.Element` — 有 `src` 用 `<img>`,否則 `<div>` 套 `posterGradient(id)` 背景。

- [ ] **Step 1: 寫 PosterThumb 失敗測試**

```tsx
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import PosterThumb from './PosterThumb'

describe('PosterThumb', () => {
  it('renders an img when src is provided', () => {
    render(<PosterThumb id="v1" src="http://x/thumb.jpg" alt="poster" />)
    const img = screen.getByRole('img', { name: 'poster' })
    expect(img).toHaveAttribute('src', 'http://x/thumb.jpg')
  })
  it('renders a gradient fallback when src is missing', () => {
    const { container } = render(<PosterThumb id="v1" />)
    expect(screen.queryByRole('img')).toBeNull()
    const fallback = container.querySelector('[data-poster-fallback]') as HTMLElement
    expect(fallback).toBeTruthy()
    expect(fallback.style.background).toContain('linear-gradient')
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/components/admin/PosterThumb.test.tsx`
Expected: FAIL（元件未定義）。

- [ ] **Step 3: 實作 PosterThumb**

```tsx
import { posterGradient } from '../../lib/posterGradient'

interface PosterThumbProps {
  id: string
  src?: string | null
  alt?: string
  className?: string
}

// 縮圖：有真圖用 img，缺圖用 id 雜湊出的漸層 placeholder（維持一致性）。
export default function PosterThumb({ id, src, alt = '', className = '' }: PosterThumbProps) {
  const base = 'aspect-video rounded-btn overflow-hidden bg-surface-2'
  if (src) {
    return (
      <div className={`${base} ${className}`}>
        <img src={src} alt={alt} className="w-full h-full object-cover" loading="lazy" />
      </div>
    )
  }
  return (
    <div
      data-poster-fallback
      className={`${base} ${className}`}
      style={{ background: posterGradient(id) }}
      aria-hidden="true"
    />
  )
}
```

- [ ] **Step 4: 實作 NavIcon**

```tsx
import type { AdminIconKey } from '../../lib/adminNav'

// Heroicons outline 風格 stroke icons（24x24，stroke=currentColor）。
const PATHS: Record<AdminIconKey, string> = {
  dashboard: 'M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 8.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z',
  video: 'M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z',
  star: 'M11.48 3.499a.562.562 0 011.04 0l2.125 5.111a.563.563 0 00.475.345l5.518.442c.499.04.701.663.321.988l-4.204 3.602a.563.563 0 00-.182.557l1.285 5.385a.562.562 0 01-.84.61l-4.725-2.885a.563.563 0 00-.586 0L6.982 20.54a.562.562 0 01-.84-.61l1.285-5.386a.562.562 0 00-.182-.557l-4.204-3.602a.563.563 0 01.321-.988l5.518-.442a.563.563 0 00.475-.345L11.48 3.5z',
  tag: 'M9.568 3H5.25A2.25 2.25 0 003 5.25v4.318c0 .597.237 1.17.659 1.591l9.581 9.581c.699.699 1.78.872 2.607.33a18.095 18.095 0 005.223-5.223c.542-.827.369-1.908-.33-2.607L11.16 3.66A2.25 2.25 0 009.568 3z M6 6h.008v.008H6V6z',
  folder: 'M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z',
  users: 'M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z',
  chart: 'M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z',
}

export default function NavIcon({ name, className = 'w-5 h-5' }: { name: AdminIconKey; className?: string }) {
  return (
    <svg className={className} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.6} aria-hidden="true">
      <path strokeLinecap="round" strokeLinejoin="round" d={PATHS[name]} />
    </svg>
  )
}
```

- [ ] **Step 5: 跑測試確認通過**

Run: `cd web && npx vitest run src/components/admin/PosterThumb.test.tsx`
Expected: PASS。

- [ ] **Step 6: typecheck + lint**

Run: `cd web && npx tsc -b && npm run lint`
Expected: 0 error / 0 warning。

- [ ] **Step 7: Commit**

```bash
git add web/src/components/admin/NavIcon.tsx web/src/components/admin/PosterThumb.tsx web/src/components/admin/PosterThumb.test.tsx
git commit -m "feat: add NavIcon + PosterThumb admin components"
```

---

### Task 6: AdminSidebar

78px 圖示軌:品牌盾 + 7 個 nav 項(enabled 用 `<Link>`,disabled 置灰 + tooltip「即將推出」)。

**Files:**
- Create: `web/src/components/admin/AdminSidebar.tsx`
- Test: `web/src/components/admin/AdminSidebar.test.tsx`

**Interfaces:**
- Consumes: `ADMIN_NAV`, `isNavItemActive`(Task 4);`NavIcon`(Task 5);`useLocation`(react-router)。
- Produces: `AdminSidebar(): JSX.Element` — 無 props,自取目前路徑。

- [ ] **Step 1: 寫失敗測試**

```tsx
import { describe, it, expect } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import AdminSidebar from './AdminSidebar'

describe('AdminSidebar', () => {
  it('renders all 7 nav labels', () => {
    renderWithRouter(<AdminSidebar />, { route: '/admin/library' })
    for (const label of ['總覽', '影片', '精選', '標籤', '來源', '帳號', '分析']) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }
  })
  it('renders enabled items as links and disabled items as non-links', () => {
    renderWithRouter(<AdminSidebar />, { route: '/admin/library' })
    // 影片 is enabled → its label sits inside an anchor
    expect(screen.getByText('影片').closest('a')).not.toBeNull()
    // 總覽 is disabled → no anchor, marked aria-disabled
    expect(screen.getByText('總覽').closest('a')).toBeNull()
    expect(screen.getByText('總覽').closest('[aria-disabled="true"]')).not.toBeNull()
  })
  it('marks the active item with aria-current', () => {
    renderWithRouter(<AdminSidebar />, { route: '/admin/users' })
    expect(screen.getByText('帳號').closest('a')).toHaveAttribute('aria-current', 'page')
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/components/admin/AdminSidebar.test.tsx`
Expected: FAIL。

- [ ] **Step 3: 實作**

```tsx
import { Link, useLocation } from 'react-router-dom'
import { ADMIN_NAV, isNavItemActive } from '../../lib/adminNav'
import NavIcon from './NavIcon'

export default function AdminSidebar() {
  const { pathname } = useLocation()
  return (
    <aside className="fixed inset-y-0 left-0 w-[78px] bg-surface border-r border-border flex flex-col items-center py-4 z-30">
      <Link
        to="/admin/library"
        aria-label="Vaultflix 管理後台"
        className="w-[42px] h-[42px] rounded-card flex items-center justify-center shrink-0"
        style={{ background: 'linear-gradient(150deg, #FF8A3D, #7A1F4B)' }}
      >
        <svg className="w-6 h-6 text-cream" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
          <path d="M12 2l8 3.5v6c0 5-3.4 8.7-8 10.5-4.6-1.8-8-5.5-8-10.5v-6L12 2z" opacity="0.92" />
        </svg>
      </Link>

      <nav className="flex flex-col items-stretch gap-1 mt-6 w-full px-2">
        {ADMIN_NAV.map((item) => {
          const active = item.enabled && isNavItemActive(item.path, pathname)
          const inner = (
            <>
              <NavIcon name={item.icon} />
              <span className="text-[10px] mt-1 leading-none">{item.label}</span>
            </>
          )
          const shape = 'flex flex-col items-center justify-center py-2 rounded-btn transition-colors'
          if (!item.enabled) {
            return (
              <div
                key={item.key}
                aria-disabled="true"
                title="即將推出"
                className={`${shape} text-faint/50 cursor-not-allowed`}
              >
                {inner}
              </div>
            )
          }
          return (
            <Link
              key={item.key}
              to={item.path}
              aria-current={active ? 'page' : undefined}
              className={`${shape} ${
                active ? 'text-accent bg-accent/15' : 'text-muted hover:text-cream hover:bg-surface-2'
              }`}
            >
              {inner}
            </Link>
          )
        })}
      </nav>
    </aside>
  )
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `cd web && npx vitest run src/components/admin/AdminSidebar.test.tsx`
Expected: PASS。

- [ ] **Step 5: typecheck + lint, then commit**

```bash
cd web && npx tsc -b && npm run lint
git add web/src/components/admin/AdminSidebar.tsx web/src/components/admin/AdminSidebar.test.tsx
git commit -m "feat: add AdminSidebar icon rail"
```

---

### Task 7: AdminTopbar

64px sticky header:麵包屑 + 標題、全域搜尋(輸入即導向 `/admin/library?q=`)、API 狀態膠囊、使用者膠囊(含登出)。

**Files:**
- Create: `web/src/components/admin/AdminTopbar.tsx`
- Test: `web/src/components/admin/AdminTopbar.test.tsx`

**Interfaces:**
- Consumes: `adminPageTitle`(Task 4);`useAuth`(`../../contexts/AuthContext` → `{ user, logout }`);`useNavigate`/`useLocation`。
- Produces: `AdminTopbar(): JSX.Element`。
- 搜尋行為:輸入後 debounce 300ms,呼叫 `navigate('/admin/library?q=' + encodeURIComponent(value))`(空字串則 `navigate('/admin/library')`)。

- [ ] **Step 1: 寫失敗測試**

```tsx
import { describe, it, expect, vi } from 'vitest'
import { screen, render, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom'
import AdminTopbar from './AdminTopbar'

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ user: { username: 'steven', role: 'admin' }, logout: vi.fn() }),
}))

function LocationProbe() {
  const loc = useLocation()
  return <div data-testid="loc">{loc.pathname + loc.search}</div>
}

describe('AdminTopbar', () => {
  it('shows breadcrumb + current page title', () => {
    render(
      <MemoryRouter initialEntries={['/admin/users']}>
        <AdminTopbar />
      </MemoryRouter>,
    )
    expect(screen.getByText('管理後台 /')).toBeInTheDocument()
    expect(screen.getByText('帳號')).toBeInTheDocument()
  })

  it('navigates to library with q on search input', async () => {
    render(
      <MemoryRouter initialEntries={['/admin/users']}>
        <AdminTopbar />
        <LocationProbe />
      </MemoryRouter>,
    )
    await userEvent.type(screen.getByPlaceholderText('搜尋影片、檔名、標籤...'), 'matrix')
    await waitFor(() =>
      expect(screen.getByTestId('loc').textContent).toBe('/admin/library?q=matrix'),
    )
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/components/admin/AdminTopbar.test.tsx`
Expected: FAIL。

- [ ] **Step 3: 實作**

```tsx
import { useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../../contexts/AuthContext'
import { adminPageTitle } from '../../lib/adminNav'

export default function AdminTopbar() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const { user, logout } = useAuth()
  const [menuOpen, setMenuOpen] = useState(false)
  const [search, setSearch] = useState('')
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  // debounce 後導向影片庫並帶 q；空字串回乾淨影片庫
  useEffect(() => () => clearTimeout(debounceRef.current), [])
  function handleSearch(value: string) {
    setSearch(value)
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      navigate(value ? `/admin/library?q=${encodeURIComponent(value)}` : '/admin/library')
    }, 300)
  }

  const initial = (user?.username?.[0] ?? '?').toUpperCase()

  return (
    <header className="sticky top-0 h-16 flex items-center gap-4 px-7 border-b border-border z-20"
      style={{ background: 'rgba(13,11,10,0.86)', backdropFilter: 'blur(16px)' }}>
      <div className="flex items-baseline gap-2 shrink-0">
        <span className="text-muted text-sm">管理後台 /</span>
        <span className="font-display font-bold text-lg tracking-tight text-cream">{adminPageTitle(pathname)}</span>
      </div>

      <div className="flex-1 flex justify-center">
        <input
          value={search}
          onChange={(e) => handleSearch(e.target.value)}
          placeholder="搜尋影片、檔名、標籤..."
          className="w-full max-w-[420px] bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none border border-transparent focus:border-accent placeholder-faint"
        />
      </div>

      <div className="flex items-center gap-3 shrink-0">
        <span className="hidden md:flex items-center gap-1.5 text-xs font-mono text-muted bg-surface-2 rounded-pill px-2.5 py-1">
          <span className="w-1.5 h-1.5 rounded-full bg-live" /> API 正常 · 8080
        </span>
        <div className="relative">
          <button onClick={() => setMenuOpen((o) => !o)} className="flex items-center gap-2">
            <span className="w-7 h-7 rounded-full flex items-center justify-center text-accent-ink text-sm font-bold"
              style={{ background: 'linear-gradient(150deg, #FFB23F, #FF8A3D)' }}>{initial}</span>
            <span className="text-sm text-cream">{user?.username}</span>
            <span className="text-xs text-accent">admin</span>
          </button>
          {menuOpen && (
            <>
              <div className="fixed inset-0" onClick={() => setMenuOpen(false)} />
              <div className="absolute right-0 mt-2 w-36 bg-surface-2 rounded-card shadow-float py-1 z-10 border border-border">
                <button onClick={() => { logout(); setMenuOpen(false) }}
                  className="w-full text-left px-3 py-1.5 text-sm text-muted hover:bg-surface hover:text-cream">登出</button>
              </div>
            </>
          )}
        </div>
      </div>
    </header>
  )
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `cd web && npx vitest run src/components/admin/AdminTopbar.test.tsx`
Expected: PASS。

- [ ] **Step 5: typecheck + lint, then commit**

```bash
cd web && npx tsc -b && npm run lint
git add web/src/components/admin/AdminTopbar.tsx web/src/components/admin/AdminTopbar.test.tsx
git commit -m "feat: add AdminTopbar with global search"
```

---

### Task 8: AdminLayout + 路由改巢狀

組合殼層,並把 `App.tsx` 的 admin 路由改成巢狀 layout route。`/admin` 重導向 `/admin/library`。

**Files:**
- Create: `web/src/components/admin/AdminLayout.tsx`
- Test: `web/src/components/admin/AdminLayout.test.tsx`
- Modify: `web/src/App.tsx`

**Interfaces:**
- Consumes: `AdminSidebar`(Task 6)、`AdminTopbar`(Task 7)、`<Outlet/>`。
- Produces: `AdminLayout(): JSX.Element` — 固定左欄 78px + 右側 topbar + 可捲動主內容(`<Outlet/>`)。

- [ ] **Step 1: 寫 AdminLayout 失敗測試**

```tsx
import { describe, it, expect, vi } from 'vitest'
import { screen, render } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import AdminLayout from './AdminLayout'

vi.mock('../../contexts/AuthContext', () => ({
  useAuth: () => ({ user: { username: 'steven', role: 'admin' }, logout: vi.fn() }),
}))

describe('AdminLayout', () => {
  it('renders sidebar + topbar around the outlet content', () => {
    render(
      <MemoryRouter initialEntries={['/admin/library']}>
        <Routes>
          <Route element={<AdminLayout />}>
            <Route path="/admin/library" element={<div>庫內容</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('庫內容')).toBeInTheDocument()
    expect(screen.getByText('影片')).toBeInTheDocument() // sidebar 標籤
    expect(screen.getByText('管理後台 /')).toBeInTheDocument() // topbar 麵包屑
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/components/admin/AdminLayout.test.tsx`
Expected: FAIL。

- [ ] **Step 3: 實作 AdminLayout**

```tsx
import { Outlet } from 'react-router-dom'
import AdminSidebar from './AdminSidebar'
import AdminTopbar from './AdminTopbar'

export default function AdminLayout() {
  return (
    <div className="min-h-screen bg-bg text-cream">
      <AdminSidebar />
      <div className="pl-[78px] flex flex-col min-h-screen">
        <AdminTopbar />
        <main className="flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `cd web && npx vitest run src/components/admin/AdminLayout.test.tsx`
Expected: PASS。

- [ ] **Step 5: 改 App.tsx 路由**

把 import 區的 admin 頁 import 換成同時引入 `AdminLayout` 與新標籤頁(`TagManagePage` 在 Task 9 建立 —— 本步先建立巢狀結構,`/admin/tags` route 先指向既有頁的 placeholder?**不**:為避免引用未建立檔案,本步驟先不加 `/admin/tags`,Task 9 再加)。

將 `App.tsx` line 14-17 的 admin imports 改為:
```tsx
import AdminLayout from './components/admin/AdminLayout'
import VideoManagePage from './pages/admin/VideoManagePage'
import RecommendationManagePage from './pages/admin/RecommendationManagePage'
import UserManagePage from './pages/admin/UserManagePage'
import MediaSourcePage from './pages/admin/MediaSourcePage'
```

將 line 65-78 的 admin route 區塊改為:
```tsx
      {
        element: <ProtectedRoute />,
        children: [
          {
            element: <AdminRoute />,
            children: [
              {
                element: <AdminLayout />,
                children: [
                  { path: '/admin', element: <Navigate to="/admin/library" replace /> },
                  { path: '/admin/library', element: <VideoManagePage /> },
                  { path: '/admin/recommendations', element: <RecommendationManagePage /> },
                  { path: '/admin/users', element: <UserManagePage /> },
                  { path: '/admin/media-sources', element: <MediaSourcePage /> },
                ],
              },
            ],
          },
        ],
      },
```
（`Navigate` 已從 `react-router-dom` import,無需新增。）

- [ ] **Step 6: typecheck + lint + 全測試**

Run: `cd web && npx tsc -b && npm run lint && npx vitest run`
Expected: 全綠。（此時各 admin 頁仍渲染舊 `AdminHeader` —— 視覺上會有兩條 header,Task 10-13 reskin 時會移除各頁自己的 `AdminHeader`。功能不破。)

- [ ] **Step 7: Commit**

```bash
git add web/src/components/admin/AdminLayout.tsx web/src/components/admin/AdminLayout.test.tsx web/src/App.tsx
git commit -m "feat: add AdminLayout shell + nest admin routes under it"
```

---

### Task 9: 標籤管理頁(新)

`/admin/tags`:`listTags()` 依 category 分組成卡,`createTag` 新增。先抽純函式 `groupTagsByCategory`,再做頁面。

**Files:**
- Create: `web/src/lib/tagGroups.ts`
- Test: `web/src/lib/tagGroups.test.ts`
- Create: `web/src/pages/admin/TagManagePage.tsx`
- Test: `web/src/pages/admin/TagManagePage.test.tsx`
- Modify: `web/src/App.tsx`(加 `/admin/tags` route)

**Interfaces:**
- Consumes: `listTags`(`../../api/tags`)、`createTag`(`../../api/admin`)、`useToast`、type `TagWithCount`。
- Produces:
  - `interface TagCategoryGroup { category: string; label: string; tags: TagWithCount[] }`
  - `groupTagsByCategory(tags: TagWithCount[]): TagCategoryGroup[]` — 依 genre/studio/actor/custom 固定順序分組,丟掉空組與未知 category。
  - `TagManagePage(): JSX.Element`。

- [ ] **Step 1: 寫 tagGroups 失敗測試**

```ts
import { describe, it, expect } from 'vitest'
import { groupTagsByCategory } from './tagGroups'
import type { TagWithCount } from '../types'

const t = (id: number, name: string, category: string): TagWithCount =>
  ({ id, name, category, video_count: id })

describe('groupTagsByCategory', () => {
  it('groups by category in genre→studio→actor→custom order', () => {
    const groups = groupTagsByCategory([
      t(1, 'a', 'custom'), t(2, 'b', 'genre'), t(3, 'c', 'actor'),
    ])
    expect(groups.map((g) => g.category)).toEqual(['genre', 'actor', 'custom'])
    expect(groups.map((g) => g.label)).toEqual(['類型', '人物', '自訂'])
  })
  it('drops empty groups and unknown categories', () => {
    const groups = groupTagsByCategory([t(1, 'a', 'genre'), t(2, 'b', 'weird')])
    expect(groups).toHaveLength(1)
    expect(groups[0].tags.map((x) => x.name)).toEqual(['a'])
  })
})
```

- [ ] **Step 2: 跑測試確認失敗,然後實作 tagGroups**

Run: `cd web && npx vitest run src/lib/tagGroups.test.ts` → FAIL。

```ts
import type { TagWithCount } from '../types'

export interface TagCategoryGroup {
  category: string
  label: string
  tags: TagWithCount[]
}

const CATEGORY_ORDER: { category: string; label: string }[] = [
  { category: 'genre', label: '類型' },
  { category: 'studio', label: '工作室' },
  { category: 'actor', label: '人物' },
  { category: 'custom', label: '自訂' },
]

// 依設計固定順序分組；空組與未知 category 不顯示。
export function groupTagsByCategory(tags: TagWithCount[]): TagCategoryGroup[] {
  return CATEGORY_ORDER
    .map(({ category, label }) => ({ category, label, tags: tags.filter((x) => x.category === category) }))
    .filter((g) => g.tags.length > 0)
}
```
Run again → PASS。

- [ ] **Step 3: 寫 TagManagePage 失敗測試**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import TagManagePage from './TagManagePage'

vi.mock('../../api/tags', () => ({
  listTags: vi.fn(() => Promise.resolve([
    { id: 1, name: '動作', category: 'genre', video_count: 12 },
    { id: 2, name: 'A工作室', category: 'studio', video_count: 5 },
  ])),
}))
vi.mock('../../api/admin', () => ({ createTag: vi.fn() }))
vi.mock('../../contexts/ToastContext', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}))

describe('TagManagePage', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders category groups with their tags', async () => {
    renderWithRouter(<TagManagePage />, { route: '/admin/tags' })
    await waitFor(() => expect(screen.getByText('類型')).toBeInTheDocument())
    expect(screen.getByText('工作室')).toBeInTheDocument()
    expect(screen.getByText('動作')).toBeInTheDocument()
    expect(screen.getByText('A工作室')).toBeInTheDocument()
  })
})
```

- [ ] **Step 4: 跑測試確認失敗**

Run: `cd web && npx vitest run src/pages/admin/TagManagePage.test.tsx` → FAIL。

- [ ] **Step 5: 實作 TagManagePage**

頁面結構(用 token class,< 300 行):
- 標題列:`<h1 class="font-display font-bold text-xl tracking-tight">標籤與分類</h1>` + 副標「N 個標籤 · M 個分類」(mono 數字)+ 「新增標籤」`bg-accent text-accent-ink` 主鈕(開 modal)。
- 內容:`useEffect`(cleanup flag)抓 `listTags()` → `groupTagsByCategory` → 2 欄卡格(`grid md:grid-cols-2 gap-4`,`max-w-[1100px]`)。每卡 `bg-surface border border-border rounded-card p-5`:色點 + 分類 label + 該分類總數,下方該分類標籤 chips(`bg-surface-2 rounded-pill px-2.5 py-1 text-sm`,名稱 + `text-faint` 數量)。
- 新增 modal:輸入 name + select category(genre/studio/actor/custom),呼叫 `createTag(name, category)`,成功後 `useToast().success` + 重抓 `listTags`,失敗 `error`。遮罩 `bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px]`、內容 `stopPropagation`。
- 色點顏色:genre=accent、studio=data-blue、actor=data-purple、custom=muted(用對應 token class `bg-accent`/`bg-data-blue`/`bg-data-purple`/`bg-muted`)。

實作骨架(補完細節):
```tsx
import { useEffect, useState } from 'react'
import { listTags } from '../../api/tags'
import { createTag } from '../../api/admin'
import { useToast } from '../../contexts/ToastContext'
import { groupTagsByCategory } from '../../lib/tagGroups'
import type { TagWithCount } from '../../types'

const DOT: Record<string, string> = {
  genre: 'bg-accent', studio: 'bg-data-blue', actor: 'bg-data-purple', custom: 'bg-muted',
}

export default function TagManagePage() {
  const [tags, setTags] = useState<TagWithCount[]>([])
  const [showCreate, setShowCreate] = useState(false)
  const toast = useToast()

  function reload() {
    listTags().then(setTags).catch((err) => console.warn('failed to load tags', err))
  }
  useEffect(() => {
    let cancelled = false
    listTags().then((t) => { if (!cancelled) setTags(t) }).catch((err) => console.warn('failed to load tags', err))
    return () => { cancelled = true }
  }, [])

  const groups = groupTagsByCategory(tags)
  return (
    <div className="p-7 max-w-[1100px]">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="font-display font-bold text-xl tracking-tight text-cream">標籤與分類</h1>
          <p className="text-sm text-muted mt-0.5">
            <span className="font-mono">{tags.length}</span> 個標籤 · <span className="font-mono">{groups.length}</span> 個分類
          </p>
        </div>
        <button onClick={() => setShowCreate(true)}
          className="bg-accent text-accent-ink text-sm font-medium px-4 py-2 rounded-btn hover:brightness-110">新增標籤</button>
      </div>

      <div className="grid md:grid-cols-2 gap-4">
        {groups.map((g) => (
          <div key={g.category} className="bg-surface border border-border rounded-card p-5">
            <div className="flex items-center gap-2 mb-3">
              <span className={`w-2.5 h-2.5 rounded-full ${DOT[g.category] ?? 'bg-muted'}`} />
              <span className="font-display font-semibold text-cream">{g.label}</span>
              <span className="font-mono text-xs text-faint ml-auto">{g.tags.length}</span>
            </div>
            <div className="flex flex-wrap gap-2">
              {g.tags.map((tag) => (
                <span key={tag.id} className="bg-surface-2 rounded-pill px-2.5 py-1 text-sm text-cream">
                  {tag.name} <span className="font-mono text-faint">{tag.video_count}</span>
                </span>
              ))}
            </div>
          </div>
        ))}
      </div>

      {showCreate && (
        <CreateTagModal
          onClose={() => setShowCreate(false)}
          onCreated={() => { setShowCreate(false); reload(); toast.success('已新增標籤') }}
          onError={() => toast.error('新增失敗，請重試')}
        />
      )}
    </div>
  )
}

function CreateTagModal({ onClose, onCreated, onError }: { onClose: () => void; onCreated: () => void; onError: () => void }) {
  const [name, setName] = useState('')
  const [category, setCategory] = useState('genre')
  const [submitting, setSubmitting] = useState(false)
  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    try { await createTag(name.trim(), category); onCreated() }
    catch { onError() }
    finally { setSubmitting(false) }
  }
  return (
    <div className="fixed inset-0 flex items-center justify-center z-50 bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px]" onClick={onClose}>
      <div className="bg-surface rounded-lg p-6 w-full max-w-md border border-border" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-display font-semibold text-lg text-cream mb-4">新增標籤</h2>
        <form onSubmit={submit}>
          <label className="block text-sm text-muted mb-1">名稱</label>
          <input value={name} onChange={(e) => setName(e.target.value)} required disabled={submitting}
            className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-3" />
          <label className="block text-sm text-muted mb-1">分類</label>
          <select value={category} onChange={(e) => setCategory(e.target.value)} disabled={submitting}
            className="w-full bg-surface-2 text-cream text-sm rounded-btn px-3 py-2 outline-none focus:ring-2 focus:ring-accent mb-4">
            <option value="genre">類型</option>
            <option value="studio">工作室</option>
            <option value="actor">人物</option>
            <option value="custom">自訂</option>
          </select>
          <div className="flex justify-end gap-2">
            <button type="button" onClick={onClose} disabled={submitting} className="text-sm text-muted hover:text-cream px-3 py-1.5 rounded-btn">取消</button>
            <button type="submit" disabled={submitting || !name.trim()}
              className="bg-accent text-accent-ink text-sm px-4 py-1.5 rounded-btn disabled:opacity-50 hover:brightness-110">
              {submitting ? '建立中...' : '建立'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
```

- [ ] **Step 6: 加路由**

`App.tsx` import 區加 `import TagManagePage from './pages/admin/TagManagePage'`。在 AdminLayout children 的 `/admin/library` 之後加:
```tsx
                  { path: '/admin/tags', element: <TagManagePage /> },
```

- [ ] **Step 7: 跑測試 + typecheck + lint**

Run: `cd web && npx vitest run src/lib/tagGroups.test.ts src/pages/admin/TagManagePage.test.tsx && npx tsc -b && npm run lint`
Expected: 全綠。

- [ ] **Step 8: Commit**

```bash
git add web/src/lib/tagGroups.ts web/src/lib/tagGroups.test.ts web/src/pages/admin/TagManagePage.tsx web/src/pages/admin/TagManagePage.test.tsx web/src/App.tsx
git commit -m "feat: add admin tag management page"
```

---

### Task 10: Reskin MediaSourcePage

把媒體來源頁從灰色換成 token class,移除自帶 `AdminHeader`(改由 AdminLayout 提供),主內容加 `p-7`。行為/handler/modal 全不動。

**Files:**
- Modify: `web/src/pages/admin/MediaSourcePage.tsx`

**Interfaces:**
- Consumes: 既有 API 與 `ImportProgress`、`ErrorBanner`、`useToast`(全部不變)。

- [ ] **Step 1: 寫 reskin 守門測試(行為 + 不再有自帶 header)**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import MediaSourcePage from './MediaSourcePage'

vi.mock('../../api/admin', () => ({
  listMediaSources: vi.fn(() => Promise.resolve([
    { id: 's1', label: 'D槽', mount_path: '/mnt/host/D', enabled: true, video_count: 3, created_at: '', updated_at: '' },
  ])),
  getActiveImportJob: vi.fn(() => Promise.resolve(null)),
  createMediaSource: vi.fn(), updateMediaSource: vi.fn(), deleteMediaSource: vi.fn(), importVideos: vi.fn(),
}))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }) }))

describe('MediaSourcePage (reskin)', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders sources and uses design-token background (no embedded admin header)', async () => {
    const { container } = renderWithRouter(<MediaSourcePage />, { route: '/admin/media-sources' })
    await waitFor(() => expect(screen.getByText('D槽')).toBeInTheDocument())
    expect(screen.getByText('/mnt/host/D')).toBeInTheDocument()
    // 不再自帶搜尋框（那是 AdminTopbar 的職責）
    expect(screen.queryByPlaceholderText('搜尋影片...')).toBeNull()
    // 頁面根容器用 token，不再是 bg-gray-950
    expect(container.querySelector('.bg-gray-950')).toBeNull()
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/pages/admin/MediaSourcePage.test.tsx`
Expected: FAIL（目前仍含 `AdminHeader` + `bg-gray-950`）。

- [ ] **Step 3: 改檔**

1. 移除 `import AdminHeader from '../../components/AdminHeader'`。
2. 移除 JSX 裡 `<AdminHeader searchQuery="" onSearch={() => {}} />`。
3. 根容器 `<div className="min-h-screen bg-gray-950 flex flex-col">` → `<div className="p-7">`(刪掉內層 `<div className="flex-1 p-6">`,把其內容上移)。
4. 依「Token 對照表」逐一替換所有 gray/indigo/red/green class(含 `CreateSourceModal`/`EditSourceModal`/`ConfirmDialog` 三個子元件):卡片 `bg-surface border-border`、標題加 `font-display tracking-tight`、路徑加 `font-mono text-faint`、啟用 chip `bg-live/15 text-live`、停用 chip `bg-surface-2 text-faint`、主鈕 `bg-accent text-accent-ink`、危險鈕 `bg-fav text-cream`、遮罩 `bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px]`、input `bg-surface-2 focus:ring-accent`。
5. 來源卡左側加資料夾 icon(`<NavIcon name="folder" />`,需 `import NavIcon from '../../components/admin/NavIcon'`)。

- [ ] **Step 4: 跑測試確認通過 + 視覺自檢**

Run: `cd web && npx vitest run src/pages/admin/MediaSourcePage.test.tsx`
Expected: PASS。

- [ ] **Step 5: typecheck + lint + commit**

```bash
cd web && npx tsc -b && npm run lint
git add web/src/pages/admin/MediaSourcePage.tsx web/src/pages/admin/MediaSourcePage.test.tsx
git commit -m "refactor: reskin media source page to design system"
```

---

### Task 11: Reskin UserManagePage

此頁目前是 **inline style + `alert()`**,改成 token class 的資料表,並把 `alert()` 換成 `useToast`(對齊其他頁慣例)。行為(create/disable/enable/reset)不變。

**Files:**
- Modify: `web/src/pages/admin/UserManagePage.tsx`

**Interfaces:**
- Consumes: 既有 user API、`useToast`(新引入,取代 `alert`)。

- [ ] **Step 1: 寫守門測試**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import UserManagePage from './UserManagePage'

vi.mock('../../api/admin', () => ({
  listUsers: vi.fn(() => Promise.resolve([
    { id: 'u1', username: 'steven', role: 'admin', disabled_at: null, created_at: '2026-01-01T00:00:00Z', updated_at: '' },
    { id: 'u2', username: 'guest', role: 'viewer', disabled_at: null, created_at: '2026-01-02T00:00:00Z', updated_at: '' },
  ])),
  createUser: vi.fn(), deleteUser: vi.fn(), enableUser: vi.fn(), resetUserPassword: vi.fn(),
}))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }) }))

describe('UserManagePage (reskin)', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders the user table with role + status using tokens (no inline-style root)', async () => {
    const { container } = renderWithRouter(<UserManagePage />, { route: '/admin/users' })
    await waitFor(() => expect(screen.getByText('steven')).toBeInTheDocument())
    expect(screen.getByText('guest')).toBeInTheDocument()
    // 根容器不再用 inline style 的黑底
    const root = container.firstElementChild as HTMLElement
    expect(root.getAttribute('style')).toBeFalsy()
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/pages/admin/UserManagePage.test.tsx`
Expected: FAIL（目前 root 有 inline style）。

- [ ] **Step 3: 改檔**

1. 移除 `import AdminHeader` 與 `<AdminHeader .../>`。
2. 移除所有 `style={{...}}`,改 token class:
   - root → `<div className="p-7 max-w-[980px] mx-auto">`。
   - 標題 → `font-display font-bold text-xl tracking-tight text-cream`(文案改中文「使用者與權限」+ 副標「N 位 · Casbin RBAC」)。
   - 表格 → `w-full text-sm text-left`;表頭 `bg-surface-2 text-muted`,欄位中文化(使用者/角色/狀態/建立/操作);列 `border-b border-border hover:bg-surface-2`。
   - 角色 chip:admin `bg-accent/15 text-accent`、viewer `bg-surface-2 text-muted`。
   - 狀態:啟用 `text-live`(前置綠點)、停用 `text-faint`。
   - 頭像:首字母漸層圓(同 AdminTopbar 寫法,`w-7 h-7 rounded-full`)。
   - 建立日期 → `font-mono text-muted`。
   - 三個 modal 改 token(遮罩 `bg-[rgba(8,6,5,0.72)] backdrop-blur-[3px]`、卡 `bg-surface border-border`、主鈕 `bg-accent text-accent-ink`、危險 `bg-fav text-cream`、input `bg-surface-2 focus:ring-accent`)。
3. 把 4 處 `alert(...)` 換成 `const toast = useToast()` + `toast.error(...)` / `toast.success('已更新密碼')`;`import { useToast } from '../../contexts/ToastContext'`。

- [ ] **Step 4: 跑測試確認通過**

Run: `cd web && npx vitest run src/pages/admin/UserManagePage.test.tsx`
Expected: PASS。

- [ ] **Step 5: typecheck + lint + commit**

```bash
cd web && npx tsc -b && npm run lint
git add web/src/pages/admin/UserManagePage.tsx web/src/pages/admin/UserManagePage.test.tsx
git commit -m "refactor: reskin user management page to design system"
```

---

### Task 12: Reskin RecommendationManagePage

reskin + 縮圖改用 `PosterThumb`。沿用既有日期/排序/新增/刪除行為。把舊「影片管理」連結改指 `/admin/library`(內部連結更新)。

**Files:**
- Modify: `web/src/pages/admin/RecommendationManagePage.tsx`

- [ ] **Step 1: 寫守門測試**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithRouter } from '../../test/renderWithRouter'
import RecommendationManagePage from './RecommendationManagePage'

vi.mock('../../api/admin', () => ({
  listRecommendationsByDate: vi.fn(() => Promise.resolve([
    { id: 'r1', video_id: 'v1', title: '片A', thumbnail_url: undefined, duration_seconds: 600, resolution: '1080p', file_size_bytes: 1, sort_order: 1, is_fallback: false },
  ])),
  createRecommendation: vi.fn(), updateRecommendationSortOrder: vi.fn(), deleteRecommendation: vi.fn(),
}))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }) }))

describe('RecommendationManagePage (reskin)', () => {
  beforeEach(() => vi.clearAllMocks())
  it('lists recommendations for the date with token styling', async () => {
    const { container } = renderWithRouter(<RecommendationManagePage />, { route: '/admin/recommendations' })
    await waitFor(() => expect(screen.getByText('片A')).toBeInTheDocument())
    expect(container.querySelector('.bg-gray-950')).toBeNull()
  })
})
```

- [ ] **Step 2: 跑測試確認失敗 → 改檔 → 通過**

改檔重點:
1. 移除 `AdminHeader`。root → `<div className="p-7">`。
2. 頂部那條連結 `<Link to="/admin">影片管理</Link>` → `<Link to="/admin/library">影片庫</Link>`(內部連結更新)。
3. 縮圖那格的 `<div className="w-16 aspect-video ...">{thumbnail_url ? <img/> : <svg/>}</div>` → `<PosterThumb id={rec.video_id} src={rec.thumbnail_url} className="w-16" />`(`import PosterThumb from '../../components/admin/PosterThumb'`)。
4. 依 Token 對照表替換其餘 gray/indigo class;時長加 `font-mono`;表頭 `bg-surface-2`;遮罩 token 化。
5. 影片連結 `to={`/videos/${rec.video_id}`}` 保留(指使用者端播放頁,正確,不動)。

Run: `cd web && npx vitest run src/pages/admin/RecommendationManagePage.test.tsx` → PASS。

- [ ] **Step 3: typecheck + lint + commit**

```bash
cd web && npx tsc -b && npm run lint
git add web/src/pages/admin/RecommendationManagePage.tsx web/src/pages/admin/RecommendationManagePage.test.tsx
git commit -m "refactor: reskin recommendation page + use PosterThumb"
```

---

### Task 13: 影片庫排序/篩選/批次 純函式

VideoManagePage 改版前,先把新互動的 state 變換抽成純函式(可測),頁面再消費。

**Files:**
- Create: `web/src/lib/libraryParams.ts`
- Test: `web/src/lib/libraryParams.test.ts`

**Interfaces:**
- Produces:
  - `type LibrarySortBy = 'created_at' | 'title' | 'duration_seconds' | 'file_size_bytes'`
  - `type SortOrder = 'asc' | 'desc'`
  - `toggleSort(current: { by: LibrarySortBy; order: SortOrder }, clicked: LibrarySortBy): { by: LibrarySortBy; order: SortOrder }` — 點同欄切換升降,點新欄預設 `desc`。
  - `parseTagIds(raw: string): number[]` / `serializeTagIds(ids: number[]): string`(逗號分隔,過濾 NaN)。
  - `toggleTagId(ids: number[], id: number): number[]`。
  - `toggleSelected(selected: string[], id: string): string[]`。
  - `isAllSelected(selected: string[], pageIds: string[]): boolean`（pageIds 全在 selected 內;空頁回 false）。
  - `toggleSelectAll(selected: string[], pageIds: string[]): string[]`（全選 → 取消該頁全部;否則加入該頁全部,去重）。

- [ ] **Step 1: 寫失敗測試**

```ts
import { describe, it, expect } from 'vitest'
import {
  toggleSort, parseTagIds, serializeTagIds, toggleTagId,
  toggleSelected, isAllSelected, toggleSelectAll,
} from './libraryParams'

describe('toggleSort', () => {
  it('toggles order when same column clicked', () => {
    expect(toggleSort({ by: 'title', order: 'asc' }, 'title')).toEqual({ by: 'title', order: 'desc' })
    expect(toggleSort({ by: 'title', order: 'desc' }, 'title')).toEqual({ by: 'title', order: 'asc' })
  })
  it('defaults to desc when a new column is clicked', () => {
    expect(toggleSort({ by: 'title', order: 'asc' }, 'file_size_bytes')).toEqual({ by: 'file_size_bytes', order: 'desc' })
  })
})

describe('tag ids', () => {
  it('parses and serializes, filtering NaN', () => {
    expect(parseTagIds('1,2,3')).toEqual([1, 2, 3])
    expect(parseTagIds('')).toEqual([])
    expect(parseTagIds('1,x,3')).toEqual([1, 3])
    expect(serializeTagIds([1, 2])).toBe('1,2')
  })
  it('toggles membership', () => {
    expect(toggleTagId([1, 2], 2)).toEqual([1])
    expect(toggleTagId([1], 3)).toEqual([1, 3])
  })
})

describe('selection', () => {
  it('toggles a single id', () => {
    expect(toggleSelected(['a'], 'b')).toEqual(['a', 'b'])
    expect(toggleSelected(['a', 'b'], 'a')).toEqual(['b'])
  })
  it('isAllSelected only when every page id is selected', () => {
    expect(isAllSelected(['a', 'b'], ['a', 'b'])).toBe(true)
    expect(isAllSelected(['a'], ['a', 'b'])).toBe(false)
    expect(isAllSelected([], [])).toBe(false)
  })
  it('select-all toggles the current page set', () => {
    expect(toggleSelectAll([], ['a', 'b'])).toEqual(['a', 'b'])
    expect(toggleSelectAll(['a', 'b', 'c'], ['a', 'b'])).toEqual(['c'])
    expect(toggleSelectAll(['c'], ['a', 'b'])).toEqual(['c', 'a', 'b'])
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/lib/libraryParams.test.ts`
Expected: FAIL。

- [ ] **Step 3: 實作**

```ts
export type LibrarySortBy = 'created_at' | 'title' | 'duration_seconds' | 'file_size_bytes'
export type SortOrder = 'asc' | 'desc'

export function toggleSort(
  current: { by: LibrarySortBy; order: SortOrder },
  clicked: LibrarySortBy,
): { by: LibrarySortBy; order: SortOrder } {
  if (current.by === clicked) {
    return { by: clicked, order: current.order === 'asc' ? 'desc' : 'asc' }
  }
  return { by: clicked, order: 'desc' }
}

export function parseTagIds(raw: string): number[] {
  if (!raw) return []
  return raw.split(',').map((s) => Number(s)).filter((n) => Number.isFinite(n))
}

export function serializeTagIds(ids: number[]): string {
  return ids.join(',')
}

export function toggleTagId(ids: number[], id: number): number[] {
  return ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id]
}

export function toggleSelected(selected: string[], id: string): string[] {
  return selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]
}

export function isAllSelected(selected: string[], pageIds: string[]): boolean {
  return pageIds.length > 0 && pageIds.every((id) => selected.includes(id))
}

export function toggleSelectAll(selected: string[], pageIds: string[]): string[] {
  if (isAllSelected(selected, pageIds)) {
    return selected.filter((id) => !pageIds.includes(id))
  }
  const set = new Set(selected)
  for (const id of pageIds) set.add(id)
  return [...set]
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `cd web && npx vitest run src/lib/libraryParams.test.ts`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/libraryParams.ts web/src/lib/libraryParams.test.ts
git commit -m "feat: add library sort/filter/selection state helpers"
```

---

### Task 14: 影片庫頁改版(VideoManagePage)

把影片管理頁 reskin 成高密度資料表,接上 Task 13 的排序/標籤篩選/批次邏輯。沿用既有 import/backfill/edit/delete/tagInput 行為。`sort_by` 從 URL 讀,接到 `listVideos`。

**Files:**
- Modify: `web/src/pages/admin/VideoManagePage.tsx`
- Test: `web/src/pages/admin/VideoManagePage.test.tsx`

**Interfaces:**
- Consumes: `listVideos`(已支援 `sort_by`/`sort_order`/`q`/`tag_ids`)、`listTags`、Task 13 helpers、`PosterThumb`、既有 import/backfill/edit/delete handler。
- 批次:`批次刪除` = 對 `selected` 逐一 `deleteVideo` 後 reload;`批次加標籤` = 選 tag 後對 `selected` 逐一 `addVideoTag`。**不做**「移至來源」「views 排序」「設為精選星標」(本 slice 延後)。

- [ ] **Step 1: 寫守門測試(排序狀態 + 批次列出現)**

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithRouter } from '../../test/renderWithRouter'
import VideoManagePage from './VideoManagePage'

vi.mock('../../api/videos', () => ({
  listVideos: vi.fn(() => Promise.resolve({
    data: [
      { id: 'v1', title: '片A', description: '', thumbnail_url: undefined, duration_seconds: 600, file_size_bytes: 1024, resolution: '1080p', original_filename: 'a.mp4', tags: [], minio_object_key: '', thumbnail_key: '', preview_key: '', mime_type: '', created_at: '2026-01-01T00:00:00Z', updated_at: '' },
    ], total: 1, page: 1, page_size: 20,
  })),
}))
vi.mock('../../api/tags', () => ({ listTags: vi.fn(() => Promise.resolve([])) }))
vi.mock('../../api/admin', () => ({
  importVideos: vi.fn(), updateVideo: vi.fn(), deleteVideo: vi.fn(),
  listMediaSources: vi.fn(() => Promise.resolve([])),
  getActiveImportJob: vi.fn(() => Promise.resolve(null)),
  startBackfill: vi.fn(), getActiveBackfill: vi.fn(() => Promise.resolve(null)),
  addVideoTag: vi.fn(), removeVideoTag: vi.fn(),
}))
vi.mock('../../contexts/ToastContext', () => ({ useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }) }))

describe('VideoManagePage (library)', () => {
  beforeEach(() => vi.clearAllMocks())
  it('renders the video row and shows batch bar after selecting a row', async () => {
    renderWithRouter(<VideoManagePage />, { route: '/admin/library' })
    await waitFor(() => expect(screen.getByText('片A')).toBeInTheDocument())
    const checkbox = screen.getByLabelText('選取 片A')
    await userEvent.click(checkbox)
    expect(screen.getByText(/已選取/)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/pages/admin/VideoManagePage.test.tsx`
Expected: FAIL（目前無 checkbox / 批次列）。

- [ ] **Step 3: 改檔(分段)**

3a. 移除 `AdminHeader` 與其搜尋 props(搜尋改由 AdminTopbar 走 `?q=`;本頁仍從 `useSearchParams` 讀 `q`)。root → `<div className="p-7">`。移除頁內 `handleSearch`/`searchInput`/debounce(topbar 已負責)。
3b. 從 URL 讀排序:`const sortBy = (searchParams.get('sort_by') as LibrarySortBy) || 'created_at'`、`const sortOrder = (searchParams.get('sort_order') as SortOrder) || 'desc'`;`listVideos` 帶上 `sort_by: sortBy, sort_order: sortOrder`;fetch effect 依賴加 `sortBy, sortOrder`。
3c. 加 `const [selected, setSelected] = useState<string[]>([])`。tagIds 用 `parseTagIds(tagIdsStr)`。
3d. 工具列:片數 + 排序 chip(`最新/標題/時長/大小` → 對應 `created_at/title/duration_seconds/file_size_bytes`,點擊 `updateParams({ sort_by, sort_order })` 用 `toggleSort`)+ 升降序鈕 + 既有「補齊預覽」「匯入影片」鈕(token 化)。
3e. 標籤篩選列:`allTags` 橫向 chip(全部 + 各 tag),點擊用 `toggleTagId` → `updateParams({ tag_ids: serializeTagIds(next), page: '1' })`;選中 `bg-accent text-accent-ink`。
3f. 批次列(`selected.length > 0` 顯示,`bg-accent/10`):「已選取 N 部」+ `批次加標籤`(開既有/簡易 tag 選單 → 逐一 `addVideoTag`)+ `刪除`(逐一 `deleteVideo` 後 `setSelected([])` + reload)+「取消選取」。
3g. 表格:表頭 `bg-surface-2`;首欄全選 checkbox(`isAllSelected`/`toggleSelectAll`);可排序欄(影片/時長/大小/建立)點擊 header 呼叫 toggleSort;當前排序欄 `text-accent` + ↑/↓。
3h. 每列:選取 checkbox(`aria-label={`選取 ${video.title}`}`,`toggleSelected`)、`<PosterThumb id={video.id} src={video.thumbnail_url} className="w-[52px]" />`、標題(`Link to={`/videos/${video.id}`}`)+ 檔名(`font-mono text-faint`)、解析度 chip(4K `bg-accent/15 text-accent`、其餘 `bg-surface-2 text-muted`)、時長/大小(`font-mono text-muted`)、`TagInput`、建立日期(`font-mono`)、操作鈕組(複製路徑 → `navigator.clipboard.writeText(video.original_filename)` 後 icon 變綠勾 1.3s;編輯;刪除)。
3i. 三個既有 modal(import/edit/delete)token 化。

> 檔案可能逼近 300 行上限。若超過,把「工具列 + 標籤篩選 + 批次列」抽成同檔上方的子元件,或新增 `components/admin/LibraryToolbar.tsx`。保持每檔 < 300 行。

- [ ] **Step 4: 跑測試確認通過**

Run: `cd web && npx vitest run src/pages/admin/VideoManagePage.test.tsx`
Expected: PASS。

- [ ] **Step 5: typecheck + lint + commit**

```bash
cd web && npx tsc -b && npm run lint
git add web/src/pages/admin/VideoManagePage.tsx web/src/pages/admin/VideoManagePage.test.tsx
# 若有抽子元件一併 add
git commit -m "feat: redesign admin library page with sort/filter/batch"
```

---

### Task 15: 內部連結盤點 + 收尾驗證

掃出所有指向舊 admin 路徑的內部連結,確保都已更新,跑全套 gate。

**Files:**
- Audit: 全 `web/src`
- Possibly modify: 任何殘留 `to="/admin"`(非 redirect)、`to="/admin/recommendations"` 之外的舊連結。

- [ ] **Step 1: 盤點舊 admin 連結**

Run:
```bash
cd web && grep -rnE "to=[\"']/admin($|[\"'])" src ; grep -rn "AdminHeader" src
```
Expected:
- `to="/admin"` 只應出現在 `AdminSidebar`(品牌盾,指 `/admin/library` 實際上)與不該再有頁面直接連 `/admin` 當「影片管理」。若發現頁面用 `to="/admin"` 當影片庫連結,改成 `/admin/library`。
- `AdminHeader` 只應剩 `components/AdminHeader.tsx` 本身(未被任何 admin 頁 import)。確認 4 頁皆已移除 import。

修正任何殘留。

- [ ] **Step 2: 跑前端完整 gate**

Run: `cd web && npx tsc -b && npm run lint && npx vitest run`
Expected: 全綠(0 error / 0 warning / 所有測試 PASS)。

- [ ] **Step 3: 跑專案統一 gate**

Run: `cd /home/user/Vaultflix && task verify`
Expected: 綠(含 go vet/gofmt/go test 與前端 tsc/eslint/vitest)。

- [ ] **Step 4: 手動瀏覽器自檢(可選但建議)**

`task deploy` 或 `cd web && npm run dev` 後,以 admin 帳號逐頁點過:側邊欄切換、disabled 項 hover 顯示「即將推出」、topbar 搜尋導向影片庫並過濾、影片庫排序/標籤篩選/批次選取、標籤頁分組、各頁 CRUD。確認無 console error、視覺一致。

- [ ] **Step 5: 最終 commit(若 Step 1 有修正)**

```bash
git add -A
git commit -m "fix: update internal links to new admin routes"
```

---

## Self-Review

**Spec coverage(對照 spec 各段):**
- A. Routing 巢狀 layout + 7 nav + disabled 兩項 + `/admin`→library → Task 4(nav config)/6(sidebar)/8(layout+route)。✅
- B. 新元件 AdminLayout/Sidebar/Topbar/posterGradient → Task 3/5/6/7/8。✅
- C. token 對齊(surface-2/data-blue/data-purple) → Task 2。✅
- D. 四頁改版 + 標籤頁(只接現有 API;延後項明列) → Task 9(tags)/10(media)/11(users)/12(rec)/13+14(library)。✅
- E. 共用小元件(PosterThumb 等) → Task 5。✅
- 測試(RTL+jsdom) → Task 1,各 Task 附測試。✅
- 內部連結盤點(假設 5) → Task 15。✅

**Placeholder scan:** reskin 任務(10/11/12/14)以「Token 對照表 + 具體結構步驟 + 守門測試」描述,非 vague TBD;每個 Task 有可執行指令與預期輸出。✅

**Type consistency:** `LibrarySortBy`/`SortOrder` 跨 Task 13→14 一致;`AdminNavItem`/`AdminIconKey` 跨 Task 4→5→6 一致;`AdminIconKey` 的 7 個 key 與 `NavIcon` 的 `PATHS` key、`ADMIN_NAV` 的 `icon` 值一致(dashboard/video/star/tag/folder/users/chart)。✅

**已知尾差:** Task 8 後、reskin 各頁前,該頁同時有 AdminTopbar(layout)與自帶 AdminHeader,短暫雙 header;由 Task 10-14 各自移除 AdminHeader 消除。功能不破,屬過渡狀態,已在 Task 8 Step 6 註明。
</content>
</invoke>
