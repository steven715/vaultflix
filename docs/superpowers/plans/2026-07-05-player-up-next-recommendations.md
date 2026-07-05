# Player Up-Next 5 + Today Recommendations Sidebar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 播放頁右側欄「接著看」改為 5 部，並新增「今日推薦」5 部區塊，推薦每次進頁都 refetch。

**Architecture:** 純前端改動。新增 `RecommendationList` 垂直列表元件（吃 `RecommendationItem`，比照 `UpNextList` 樣式）。PlayerPage 調整接著看 slice 為 5、新增推薦 state 與依賴 `location.key` 的 refetch effect。接著看依賴維持 `[id]`。

**Tech Stack:** React 18 + TypeScript、react-router-dom、Vitest + Testing Library。

## Global Constraints

- 前端規範照 CLAUDE.md：`useEffect` async 一律 cleanup flag；重試/計數用 `useRef` 不用 `useState`；不用 `eslint-disable`（react-hooks@7 gate zero-warning）。
- 「每次進頁 refetch」用 `useLocation().key` 當 effect 依賴（推薦區塊）；deterministic 清單（接著看）依賴 `[id]`，不加 location.key。
- 元件命名 PascalCase、檔名與元件同名；`RecommendationItem` 型別無 `tags` 欄位。
- 後端 `SuccessResponse` 已由 axios interceptor 解包，`getTodayRecommendations` 直接回 `RecommendationItem[]`。
- 完成條件：`task test-fast` 綠（tsc / eslint zero-warning / vitest）；`task verify` 綠。

---

### Task 1: RecommendationList 元件 + 單元測試

**Files:**
- Create: `web/src/components/RecommendationList.tsx`
- Test: `web/src/components/RecommendationList.test.tsx`

**Interfaces:**
- Consumes: `RecommendationItem` from `web/src/types` (`video_id: string`, `title: string`, `thumbnail_url?: string`, `duration_seconds: number`, `resolution: string`); `PosterThumb` from `./PosterThumb`; `formatDuration` from `../utils/format`.
- Produces: `export default function RecommendationList({ items }: { items: RecommendationItem[] }): JSX.Element | null` — 供 PlayerPage 使用。每列 `<Link to={`/videos/${item.video_id}`}>`。

- [ ] **Step 1: Write the failing test**

`web/src/components/RecommendationList.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, it, expect } from 'vitest'
import RecommendationList from './RecommendationList'
import type { RecommendationItem } from '../types'

function makeItem(n: number): RecommendationItem {
  return {
    id: `r${n}`,
    video_id: `v${n}`,
    title: `Rec ${n}`,
    thumbnail_url: '',
    duration_seconds: 60,
    resolution: '1920x1080',
    file_size_bytes: 1,
    sort_order: n,
    is_fallback: false,
  }
}

function renderList(items: RecommendationItem[]) {
  return render(
    <MemoryRouter>
      <RecommendationList items={items} />
    </MemoryRouter>,
  )
}

describe('RecommendationList', () => {
  it('renders nothing when items is empty', () => {
    const { container } = renderList([])
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a row per item linking to /videos/:video_id', () => {
    renderList([makeItem(1), makeItem(2)])
    expect(screen.getByText('Rec 1')).toBeInTheDocument()
    expect(screen.getByText('Rec 2')).toBeInTheDocument()
    const link = screen.getByText('Rec 1').closest('a')
    expect(link).toHaveAttribute('href', '/videos/v1')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/RecommendationList.test.tsx`
Expected: FAIL — cannot resolve `./RecommendationList` (module not found).

- [ ] **Step 3: Write minimal implementation**

`web/src/components/RecommendationList.tsx`:

```tsx
import { Link } from 'react-router-dom'
import type { RecommendationItem } from '../types'
import { formatDuration } from '../utils/format'
import PosterThumb from './PosterThumb'

// RecommendationList is the player page's right-hand "今日推薦" column: a vertical
// list of recommended videos (same source as the home page). Mirrors UpNextList's
// row style; RecommendationItem has no tags, so the tag line is omitted.
export default function RecommendationList({ items }: { items: RecommendationItem[] }) {
  if (items.length === 0) return null
  return (
    <div className="space-y-1">
      {items.map((rec) => (
        <Link
          key={rec.id}
          to={`/videos/${rec.video_id}`}
          className="group flex gap-3 rounded-card p-2 transition-colors hover:bg-surface"
        >
          <PosterThumb
            id={rec.video_id}
            title={rec.title}
            thumbnailUrl={rec.thumbnail_url}
            showFallbackTitle={false}
            className="aspect-video w-[140px] shrink-0 rounded-lg"
          >
            <span className="absolute bottom-1 right-1 rounded bg-black/70 px-1 py-0.5 font-mono text-[10px] text-cream">
              {formatDuration(rec.duration_seconds)}
            </span>
          </PosterThumb>
          <div className="min-w-0 flex-1 py-0.5">
            <h4 className="line-clamp-2 text-sm font-medium leading-snug text-cream transition-colors group-hover:text-accent">
              {rec.title}
            </h4>
            <div className="mt-1 font-mono text-[11px] text-muted">
              <span className="text-accent">{rec.resolution}</span>
            </div>
          </div>
        </Link>
      ))}
    </div>
  )
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/RecommendationList.test.tsx`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/RecommendationList.tsx web/src/components/RecommendationList.test.tsx
git commit -m "feat: add RecommendationList vertical list component for player sidebar"
```

---

### Task 2: 接著看限 5 部（page_size 6, slice 5）

**Files:**
- Modify: `web/src/pages/PlayerPage.tsx:114-127` (the up-next effect)
- Test: `web/src/pages/PlayerPage.test.tsx`

**Interfaces:**
- Consumes: `listVideos` from `../api/videos` (already imported), `UpNextList` (already rendered).
- Produces: no new exports; behavioural change — up-next shows at most 5 videos excluding the current one, fetched with `page_size: 6`.

- [ ] **Step 1: Add the failing test**

在 `web/src/pages/PlayerPage.test.tsx` 的 `describe('PlayerPage play_mode', ...)` 內新增測試。先在檔案頂部確保有 mock（見 Task 3 會加 recommendations mock；本 Task 只依賴 videos mock，已存在）。新增：

```tsx
it('shows at most 5 up-next videos, excluding the current one', async () => {
  vi.mocked(videosApi.getVideo).mockResolvedValue({ ...base, play_mode: 'direct' } as never)
  const many = Array.from({ length: 6 }, (_, i) => ({
    id: i === 0 ? 'v1' : `u${i}`, // v1 is the current video → must be excluded
    title: `Up ${i}`,
    tags: [],
    resolution: '1920x1080',
    duration_seconds: 10,
    thumbnail_url: '',
  }))
  vi.mocked(videosApi.listVideos).mockResolvedValue({
    data: many,
    total: 6,
    page: 1,
    page_size: 6,
  } as never)

  renderPlayer()

  // Current video v1 excluded; the remaining 5 (u1..u5) all render.
  expect(await screen.findByText('Up 5')).toBeInTheDocument()
  expect(screen.getByText('Up 1')).toBeInTheDocument()
  expect(screen.queryByText('Up 0')).not.toBeInTheDocument() // v1 excluded
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/pages/PlayerPage.test.tsx -t "at most 5 up-next"`
Expected: FAIL — 目前 slice 為 8 且無 recommendations mock 可能報錯；重點是斷言關係不符或 render 例外。（若因 recommendations 未 mock 而 throw，先做 Task 3 的 mock 再回來；本步驟預期紅燈即可。）

- [ ] **Step 3: Change the up-next effect**

Modify `web/src/pages/PlayerPage.tsx:114-127`. Replace:

```tsx
  // Up-next column: a handful of other videos, excluding the current one.
  useEffect(() => {
    if (!id) return
    let cancelled = false
    listVideos({ page: 1, page_size: 12, sort_by: 'created_at', sort_order: 'desc' })
      .then((res) => {
        if (cancelled) return
        setUpNext(res.data.filter((v) => v.id !== id).slice(0, 8))
      })
      .catch((err) => console.warn('failed to load up-next', err))
    return () => {
      cancelled = true
    }
  }, [id])
```

with:

```tsx
  // Up-next column: 5 other videos, excluding the current one. Fetch 6 so that
  // even when the current video is among the newest 5, we still fill 5 rows.
  useEffect(() => {
    if (!id) return
    let cancelled = false
    listVideos({ page: 1, page_size: 6, sort_by: 'created_at', sort_order: 'desc' })
      .then((res) => {
        if (cancelled) return
        setUpNext(res.data.filter((v) => v.id !== id).slice(0, 5))
      })
      .catch((err) => console.warn('failed to load up-next', err))
    return () => {
      cancelled = true
    }
  }, [id])
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/pages/PlayerPage.test.tsx -t "at most 5 up-next"`
Expected: PASS. (若仍因 recommendations 未 mock 失敗，先完成 Task 3 再重跑；Task 2 與 Task 3 可合併 commit。)

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/PlayerPage.tsx web/src/pages/PlayerPage.test.tsx
git commit -m "feat: limit player up-next to 5 videos (fetch 6, exclude current)"
```

---

### Task 3: 推薦區塊 + location.key refetch

**Files:**
- Modify: `web/src/pages/PlayerPage.tsx` (imports, state, new effect, aside JSX)
- Test: `web/src/pages/PlayerPage.test.tsx`

**Interfaces:**
- Consumes: `getTodayRecommendations` from `../api/recommendations` (`(fallbackCount?: number) => Promise<RecommendationItem[]>`); `RecommendationList` from `../components/RecommendationList` (Task 1); `useLocation` from `react-router-dom`; `RecommendationItem` type.
- Produces: player right `<aside>` renders 接著看 (`UpNextList`) then 今日推薦 (`RecommendationList`); recommendations refetch on every `location.key` change.

- [ ] **Step 1: Add the failing tests + recommendations mock**

在 `web/src/pages/PlayerPage.test.tsx` 頂部 imports 後加入 recommendations mock：

```tsx
import * as recommendationsApi from '../api/recommendations'

vi.mock('../api/recommendations')
```

在 `beforeEach` 內加預設 mock（回 5 部推薦，且提供一個共用 factory）：

```tsx
  vi.mocked(recommendationsApi.getTodayRecommendations).mockResolvedValue([
    { id: 'r1', video_id: 'rv1', title: 'Rec One', thumbnail_url: '', duration_seconds: 60, resolution: '1920x1080', file_size_bytes: 1, sort_order: 1, is_fallback: false },
    { id: 'r2', video_id: 'rv2', title: 'Rec Two', thumbnail_url: '', duration_seconds: 60, resolution: '1920x1080', file_size_bytes: 1, sort_order: 2, is_fallback: false },
    { id: 'r3', video_id: 'rv3', title: 'Rec Three', thumbnail_url: '', duration_seconds: 60, resolution: '1920x1080', file_size_bytes: 1, sort_order: 3, is_fallback: false },
    { id: 'r4', video_id: 'rv4', title: 'Rec Four', thumbnail_url: '', duration_seconds: 60, resolution: '1920x1080', file_size_bytes: 1, sort_order: 4, is_fallback: false },
    { id: 'r5', video_id: 'rv5', title: 'Rec Five', thumbnail_url: '', duration_seconds: 60, resolution: '1920x1080', file_size_bytes: 1, sort_order: 5, is_fallback: false },
  ] as never)
```

新增兩個測試（放在 describe 內）：

```tsx
it('renders the 今日推薦 block with recommendation items', async () => {
  vi.mocked(videosApi.getVideo).mockResolvedValue({ ...base, play_mode: 'direct' } as never)
  renderPlayer()
  expect(await screen.findByText('今日推薦')).toBeInTheDocument()
  expect(await screen.findByText('Rec One')).toBeInTheDocument()
  expect(screen.getByText('Rec Five')).toBeInTheDocument()
})

it('refetches recommendations on navigation to another video (new location.key)', async () => {
  vi.mocked(videosApi.getVideo).mockResolvedValue({ ...base, play_mode: 'direct' } as never)

  // A tiny harness that lets the test navigate to a new :id at runtime.
  function Nav() {
    const navigate = useNavigate()
    return <button onClick={() => navigate('/watch/v2')}>go v2</button>
  }
  render(
    <MemoryRouter initialEntries={['/watch/v1']}>
      <Nav />
      <Routes>
        <Route path="/watch/:id" element={<PlayerPage />} />
      </Routes>
    </MemoryRouter>,
  )

  await screen.findByText('今日推薦')
  const before = vi.mocked(recommendationsApi.getTodayRecommendations).mock.calls.length
  await userEvent.click(screen.getByText('go v2'))
  await screen.findByText('今日推薦')
  expect(vi.mocked(recommendationsApi.getTodayRecommendations).mock.calls.length).toBeGreaterThan(before)
})
```

在檔案頂部 import 補上 `useNavigate`（若尚未 import）：

```tsx
import { MemoryRouter, Routes, Route, useNavigate } from 'react-router-dom'
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run src/pages/PlayerPage.test.tsx -t "今日推薦"`
Expected: FAIL — 「今日推薦」文字尚不存在（PlayerPage 未渲染推薦區塊）。

- [ ] **Step 3: Wire recommendations into PlayerPage**

在 `web/src/pages/PlayerPage.tsx`：

3a. import（第 2 行改為含 `useLocation`；並加入 api + 元件 + 型別）：

```tsx
import { useParams, Link, useNavigate, useLocation } from 'react-router-dom'
```
```tsx
import { getTodayRecommendations } from '../api/recommendations'
import RecommendationList from '../components/RecommendationList'
```
`RecommendationItem` 加到既有 types import：

```tsx
import type { VideoDetail, VideoWithTags, RecommendationItem } from '../types'
```

3b. 在元件內取得 location 並加 state（緊接 `const navigate = useNavigate()` 之後、`upNext` state 附近）：

```tsx
  const location = useLocation()
```
```tsx
  const [recommendations, setRecommendations] = useState<RecommendationItem[]>([])
```

3c. 在既有 up-next effect 之後新增推薦 effect：

```tsx
  // Today's recommendations (same source as the home page). Refetch on every
  // navigation — location.key changes even when navigating to the same :id, so
  // clicking another up-next item (or re-opening the same video) reloads these.
  useEffect(() => {
    let cancelled = false
    getTodayRecommendations(5)
      .then((items) => {
        if (!cancelled) setRecommendations(items.slice(0, 5))
      })
      .catch((err) => console.warn('failed to load recommendations', err))
    return () => {
      cancelled = true
    }
  }, [location.key])
```

3d. 更新右側 `<aside>`（`web/src/pages/PlayerPage.tsx:458-462`）：

```tsx
          {/* Up next + recommendations */}
          <aside className="w-full shrink-0 lg:w-[380px]">
            <h2 className="mb-3 font-display text-lg font-bold text-cream">接著看</h2>
            <UpNextList items={upNext} />
            {recommendations.length > 0 && (
              <div className="mt-8">
                <h2 className="mb-3 font-display text-lg font-bold text-cream">今日推薦</h2>
                <RecommendationList items={recommendations} />
              </div>
            )}
          </aside>
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run src/pages/PlayerPage.test.tsx`
Expected: PASS (all tests, incl. Task 2 up-next test and both recommendation tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/PlayerPage.tsx web/src/pages/PlayerPage.test.tsx
git commit -m "feat: add 今日推薦 block to player sidebar with location.key refetch"
```

---

### Task 4: Gate — typecheck + eslint + full vitest

**Files:** none (verification only)

**Interfaces:** none.

- [ ] **Step 1: Run the fast gate**

Run: `task test-fast`
Expected: 綠 — `go vet`/`gofmt`/`go test` 無關本次改動仍過；web `tsc` 無型別錯誤；`eslint` zero errors / zero warnings；`vitest` 全綠（含新測試）。

- [ ] **Step 2: Fix any red**

若 eslint 對新 effect 的 hooks 依賴報 warning，檢查依賴陣列（推薦 effect 應為 `[location.key]`，up-next 為 `[id]`），不得用 `eslint-disable`。若 tsc 報 `RecommendationItem` 欄位不符，對照 `web/src/types/index.ts:156`。修到綠。

- [ ] **Step 3: Commit any fixes**

```bash
git add -A
git commit -m "fix: resolve lint/typecheck issues in player recommendations"
```

（無修正則跳過。）

---

## Manual Verification (收工前)

不列為 task step，但完成條件要求：

1. `task deploy`（重 build 不可變 nginx image）。
2. 開 http://localhost:3000，進任一影片播放頁，確認：
   - 「接著看」顯示 5 部，不含當前影片。
   - 「今日推薦」顯示 5 部（與首頁同一份）。
   - 從「接著看」點進另一部（甚至再點同一部），推薦區塊有重新載入。

## Self-Review Notes

- Spec 需求 1（接著看 5）→ Task 2；需求 2（推薦區塊）→ Task 1 + Task 3；需求 3（location.key refetch）→ Task 3。測試四項分佈於 Task 1（RecommendationList 單元 2 項）、Task 2（接著看限 5）、Task 3（推薦渲染 + refetch）。
- 型別一致：`RecommendationList` props `{ items: RecommendationItem[] }` 在 Task 1 定義、Task 3 消費，一致。`getTodayRecommendations(5)` 帶 `fallback_count`，回 `RecommendationItem[]`，`.slice(0,5)`。
- 依賴陣列：推薦 `[location.key]`、接著看 `[id]`，符合 Global Constraints。
