# Admin 後台重設計 — Slice 1：殼層 + 4 頁改版 + 標籤頁

**日期**：2026-06-29
**場景**：Feature
**狀態**：設計已核可，待寫 implementation plan

## 背景

Vaultflix 使用者端已完成「私人放映室」深色電影感改版，但 admin 後台（`/admin/*`）仍是舊的灰/靛色 Tailwind 介面，且沒有總覽 Dashboard。Claude Design 交付了一份完整的 admin 後台高擬真設計稿（`design_handoff_vaultflix_admin/`：README + `Vaultflix Admin.dc.html` 原型 + `support.js`），要把 admin 升級成與使用者端一致的視覺系統，並補上 Plex/Jellyfin 風格的高密度操作台。

整份交付涵蓋 7 個畫面 + 1 詳細頁，規模過大，**拆成多個 spec→plan→實作循環**，每個獨立可交付、CI 綠才進下一個。本文件是 **Slice 1**。

### 設計系統現況（已就緒）

[web/src/index.css](../../../web/src/index.css) 的 Tailwind v4 `@theme` 已定義設計稿所需的絕大多數 token（`--color-accent #FFB23F`、`--color-bg #0D0B0A`、`--font-display: Bricolage Grotesque`、`--font-mono: Space Mono` 等），使用者端改版時已鋪好。Admin 頁目前用原始灰色 Tailwind（`bg-gray-900` 等），尚未套用。

## Slice 1 範圍

**邊界原則：凡是不需新後端的東西，這一刀做完。**

納入：
- **AdminLayout 殼層**（78px 圖示軌側邊欄 + 64px sticky header + 可捲動 `<Outlet/>`）
- **4 頁改版**：影片庫、每日精選、使用者、媒體來源（reskin，只接現有 API）
- **標籤管理頁**（全新，但後端 `listTags`/`createTag` 已齊）

明確**延後**到後續切片：
- 總覽 Dashboard（需 `GET /api/admin/stats`）
- 分析 Analytics（需彙總端點）
- admin 影片詳細頁 + 單片觀看統計（需 `GET /api/videos/:id/stats`）
- 轉檔佇列重試/取消（需逐項佇列端點）
- 批次「移至來源」（無端點）
- `sort_by=views` 排序（後端未支援）
- 從影片庫「設為精選」星標（語意未明，保守延後）

## 設計

### A. 架構 / Routing

把現有扁平的 `/admin/*` 路由改成**巢狀 layout route**：`AdminLayout` 當外層 element，各頁為 children，共用側邊欄 + header + `<Outlet/>`。

```
/admin                 → redirect /admin/library   （Dashboard 未做）
/admin/library         ← 取代 VideoManagePage
/admin/recommendations ← 取代 RecommendationManagePage
/admin/tags            ← 全新 標籤管理頁
/admin/media-sources   ← 取代 MediaSourcePage
/admin/users           ← 取代 UserManagePage
```

- 側邊欄 7 項（總覽 / 影片 / 精選 / 標籤 / 來源 / 帳號 / 分析），其中 `總覽`、`分析` 為 `enabled:false`：置灰、不可點、tooltip「即將推出」。
- `/admin` index redirect 到 `/admin/library`。品牌六角盾 icon 點擊同樣導向 `/admin/library`（Dashboard 完成前的暫時 landing；Slice 之後改回 `/admin`）。
- `AdminRoute` 守衛維持不變（`useAuth().isAdmin`）。
- `AdminHeader` 在 admin 區停用（由 `AdminLayout` 的 topbar 取代）；保留檔案以免影響其他引用，但 admin 路由不再使用。
- 使用者端頁面（Browse/Player 等）完全不受影響。

### B. 新元件（各自聚焦，符合 CLAUDE.md「每檔 < 300 行」）

- `components/admin/AdminLayout.tsx` — 殼層：side rail + topbar + `<Outlet/>`，固定左欄 + 右側可捲動主欄。
- `components/admin/AdminSidebar.tsx` — 78px 圖示軌；nav config 為陣列 `{ key, label, icon, path, enabled }`；選中態 accent 字 + `rgba(255,178,63,0.13)` 底，hover `surface-2`；disabled 項置灰 + tooltip。icon 用 inline SVG（Heroicons 風格 stroke）。
- `components/admin/AdminTopbar.tsx` — 64px，`rgba(13,11,10,0.86)` + `blur(16px)`：左麵包屑「管理後台 /」+ 當前頁標題（由 route 推導）；中全域搜尋框（`max-w:420px`，focus border 變 accent，**輸入即 `navigate('/admin/library?q=…')`**）；右 API 狀態膠囊（綠點 + 「API 正常 · 8080」）+ 使用者膠囊（漸層頭像首字 + username + accent「admin」+ logout，沿用 `useAuth`）。
- `lib/posterGradient.ts` — 由 video id 雜湊穩定取 8 色盤之一的 135deg 線性漸層，作為缺縮圖時的 poster fallback；有 `thumbnail_url` 時優先用真圖。8 色盤見設計稿 README。

### C. Design token 對齊（`web/src/index.css` `@theme`）

**新增**：
- `--color-surface-2: #1A1512`（表頭 / 列 hover / 次按鈕底）
- `--color-data-blue: #43C6FF`
- `--color-data-purple: #B15CFF`

**沿用既有**：綠 = `--color-live #1FB588`、紅 = `--color-fav #FF5470`、accent = `--color-accent #FFB23F`。

**刻意不動**：`--color-surface` 維持現有 `#181410`（與 README `#141010` 差異極小，改動會牽動已上線的使用者端頁面，不值得）。

### D. 四頁改版 + 標籤頁（只接現有 API）

**影片庫**（取代 [web/src/pages/admin/VideoManagePage.tsx](../../../web/src/pages/admin/VideoManagePage.tsx)）
- reskin 成高密度資料表（表頭 `surface-2`、列垂直 padding 10px、hover `surface-2`、選取列 `rgba(255,178,63,0.06)`）。
- 工具列：片數 + 排序 chip（`最新/標題/時長/大小`）+ 升降序鈕 + 「匯入影片」accent 主鈕。
- 標籤篩選列：可橫滑 chip（全部 + 各標籤 + 數量），多選 toggle。
- 批次列（`selected.length>0` 才出現）：批次加標籤、批次刪除——**用現有 API 迴圈**（`addVideoTag` / `deleteVideo` 串接）+ 取消選取。
- 每列：核取方塊、縮圖（poster fallback）、標題 + 檔名、解析度 chip、時長/大小（mono）、標籤 chips（沿用 `TagInput`）、建立日期、操作鈕（複製路徑 / 編輯 / 刪除）。
- 排序：chip 或點表頭欄位（`created_at|title|duration_seconds|file_size_bytes`），再點切換升降；URL state 經 `useSearchParams`（`page/q/tag_ids/sort_by/sort_order`，沿用既有模式）。
- 分頁沿用既有 `Pagination`。
- **延後**：`views` 排序、批次移至來源、設為精選星標。

**每日精選**（取代 [web/src/pages/admin/RecommendationManagePage.tsx](../../../web/src/pages/admin/RecommendationManagePage.tsx)）
- reskin；日期選擇器 + 雙欄（今日清單含上/下移、移除 × ／ 加入影片）。沿用 `listRecommendationsByDate`/`createRecommendation`/`updateRecommendationSortOrder`/`deleteRecommendation` + `VideoPickerModal`。上下移 = 對調兩筆 `sort_order`。

**使用者**（取代 [web/src/pages/admin/UserManagePage.tsx](../../../web/src/pages/admin/UserManagePage.tsx)）
- reskin 成資料表（頭像 + 帳號 / 角色 chip / 狀態點 / 建立日期 / 操作：重設密碼、停用↔啟用）。沿用 `listUsers`/`createUser`/`deleteUser`/`enableUser`/`resetUserPassword`。停用對應 `disabled_at` 非 null。

**媒體來源**（取代 [web/src/pages/admin/MediaSourcePage.tsx](../../../web/src/pages/admin/MediaSourcePage.tsx)）
- reskin 成來源卡列（資料夾 icon + 名稱 + 啟用/停用 chip + 路徑 mono + 影片數 + 掃描匯入 / 編輯）。沿用 `listMediaSources`/`createMediaSource`/`updateMediaSource`/`deleteMediaSource`/`importVideos` + `ImportProgress`/`BackfillProgress`。
- **延後**：轉檔佇列重試/取消（無端點）。

**標籤管理頁（新）** `/admin/tags`
- `listTags()` 回 `TagWithCount[]`，依 `category`（genre/studio/actor/custom）分組成卡片（色點 + 分類名 + 總數 + 該分類標籤 chips 含數量）。
- 「新增標籤」= `createTag(name, category)`。
- 用 `useToast` 提示成功/失敗。

### E. 共用小元件

為避免各頁重複，抽出：
- poster 縮圖元件（真圖優先、缺圖走 `posterGradient` fallback）
- 解析度 chip（4K = accent 半透、其餘灰）
- 狀態 chip / 狀態點

實際抽取顆粒度於 plan 階段定；原則是「兩頁以上用到才抽」，避免過早抽象。

## 資料流

- **不新增任何 API 呼叫**，全部沿用 `web/src/api/` 既有 function。
- axios client interceptor（解 `{data}` envelope）不動。
- 影片庫 URL state 經 `useSearchParams`；topbar 搜尋更新 `?q=` 並導向 `/admin/library`。

## 錯誤 / 載入 / 空狀態

沿用既有 `ErrorBanner`、骨架/「載入中」、各列表空狀態文案與 `useToast`。

## 測試（純前端，免整合測試）

依 CLAUDE.md 測試規範，vitest 覆蓋：
- `AdminLayout`/`AdminSidebar`：渲染 7 nav、disabled 項不可點、active 高亮正確。
- `AdminTopbar`：搜尋輸入導向 `/admin/library?q=…`。
- `posterGradient`：同 id 決定性回同一漸層、不同 id 分佈於 8 色盤。
- 影片庫：排序切換、標籤篩選 toggle、批次選取/批次操作觸發正確 API。
- 標籤頁：依 category 正確分組。

完成條件 = `task test-fast`（`go vet` 無關、前端 `tsc` + `eslint` zero error/warning + `vitest`）綠。不碰 import/影片掃描/串流，免 `task test-integration`。Stop hook 的 `task verify` 須綠。

## 風險 / Trade-off

- **巢狀 route 改寫**牽動 [web/src/App.tsx](../../../web/src/App.tsx) 的 admin 路由註冊與既有書籤/連結路徑（如 `VideoManagePage` 原路徑）。緩解：plan 階段盤點所有指向舊 admin 路徑的內部連結，一併更新；保留 redirect。
- **側邊欄 disabled 項**可能被誤讀為壞掉。緩解：明確 tooltip「即將推出」+ 置灰樣式。
- **共用小元件過早抽象**風險。緩解：「兩頁以上用到才抽」。
- Slice 邊界讓視覺短期不完全一致（Dashboard/Analytics 仍缺），但每頁本身一致，且這是可交付的最小地基，符合小步交付原則。

## 假設

1. `--color-surface` 維持 `#181410` 不改為 README 的 `#141010`（差異微小、避免動到使用者端）。
2. 品牌盾 icon 與 `/admin` index 在 Dashboard 完成前暫導向 `/admin/library`。
3. 「設為精選」星標雖後端可行（`createRecommendation`），仍延後，待 Dashboard/精選整合時一併處理語意。
4. `AdminHeader.tsx` 保留檔案、僅在 admin 路由停用，不刪除。
5. 內部既有指向舊 admin 路徑的連結會在 plan 階段盤點並更新。
