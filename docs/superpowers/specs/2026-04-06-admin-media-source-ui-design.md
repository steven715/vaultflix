# Phase 13 — Admin UI：Media Source 管理頁面

## 概述

建立 Admin 管理頁面，讓管理者在瀏覽器中管理 media source 並觸發匯入。整合 Phase 12 的匯入進度 UI，形成「選擇 source → 觸發匯入 → 看進度 → 看結果」的統一管理體驗。

## 設計決策

| 決策 | 選擇 | 理由 |
|------|------|------|
| 佈局 | 卡片式 | source 數量少，操作按鈕 + 匯入進度在卡片中整合更自然 |
| 路徑驗證 | 方案 B（提交時驗證） | 後端已回傳清楚錯誤訊息，不需額外 API |
| 匯入 UI 整合 | 從 VideoManagePage 提取為可復用元件 | Phase 12 的進度 UI 嵌在 modal 中，需重構為獨立元件 |

## 後端變更

### 1. MediaSource model 加入 VideoCount

```go
type MediaSource struct {
    // ... 既有欄位 ...
    VideoCount int `json:"video_count"`
}
```

`VideoCount` 是查詢時計算的欄位，不存在於 table。只在 `List` 時帶入。

### 2. List query 改為帶 JOIN + COUNT

```sql
SELECT ms.id, ms.label, ms.mount_path, ms.enabled, ms.created_at, ms.updated_at,
       COUNT(v.id) AS video_count
FROM media_sources ms
LEFT JOIN videos v ON v.source_id = ms.id
GROUP BY ms.id
ORDER BY ms.created_at ASC
```

### 3. 後端測試

- `TestListMediaSources_WithVideoCount` — 有影片 / 無影片的 source
- `TestListMediaSources_EmptyTable` — 空陣列不是 null

## 前端變更

### 1. 路由

- 路徑：`/admin/media-sources`
- 守衛：使用既有 `AdminRoute` 元件
- 在 `App.tsx` 的 admin 路由區塊加入

### 2. Header 導航

在現有 Header 的 admin 連結旁加入「媒體來源」入口。檢查現有 Header 中 admin 連結的位置，保持一致。

### 3. MediaSourcePage 頁面元件

**檔案位置**：`web/src/pages/admin/MediaSourcePage.tsx`

**頁面結構**：

```
┌──────────────────────────────────────────────────────────────┐
│  媒體來源管理                                    [+ 新增來源]  │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  📁 D槽影片                                          │    │
│  │  /mnt/host/D/Videos  ·  156 部影片  ·  ✅ 啟用        │    │
│  │  [編輯] [掃描匯入] [停用] [刪除]                       │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  📁 E槽動畫（停用時灰底降透明度）                       │    │
│  │  /mnt/host/E/Anime  ·  43 部影片  ·  ⏸ 已停用        │    │
│  │  [編輯] [掃描匯入] [啟用] [刪除]                       │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  匯入進度（觸發後顯示在對應 source card 下方）          │    │
│  │  ████████████░░░░  134 / 156  ·  importing.mp4       │    │
│  │  成功: 120  跳過: 12  失敗: 2                         │    │
│  └──────────────────────────────────────────────────────┘    │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

**狀態管理**：

- `sources`: MediaSource[] — 列表資料
- `loading` / `error` — 載入狀態，失敗時顯示錯誤 + 重試按鈕
- `importingSourceId`: string | null — 正在匯入的 source ID
- 空狀態：「尚未設定媒體來源，請點擊右上角新增」

### 4. 新增 Source Modal

- 欄位：label（必填）、mount_path（必填，placeholder `/mnt/host/`）
- 提交：`POST /api/media-sources`
- 錯誤在 modal 內顯示（路徑不存在、路徑重複、路徑不在 /mnt/host/ 下）
- 成功後關閉 modal + 重新載入列表

### 5. 編輯 Source Modal

- 只可修改 label
- mount_path 用 disabled input 顯示，標註「路徑不可修改」
- 提交：`PUT /api/media-sources/:id`

### 6. 啟用 / 停用

- `PUT /api/media-sources/:id` 切換 enabled
- 停用前確認：「停用後，此來源下的 N 部影片將無法播放。確定要停用嗎？」
- 停用的卡片用灰底 + 降透明度區分

### 7. 刪除

- video_count > 0 時警告：「此來源下有 N 部影片記錄，刪除後這些影片將保留在資料庫中但無法播放。確定要刪除嗎？」
- `DELETE /api/media-sources/:id`
- 成功後重新載入列表

### 8. 掃描匯入整合

- 點擊「掃描匯入」→ `POST /api/videos/import` with `{ source_id }`
- 收到 202 → 在該 source card 下方展開匯入進度元件
- WebSocket 推送進度（復用 Phase 12 邏輯）
- 匯入中所有「掃描匯入」按鈕停用
- 頁面 mount 時 `GET /api/import-jobs/active` 檢查進行中 job，自動恢復

### 9. 匯入進度元件提取

從 `VideoManagePage.tsx` 提取匯入進度邏輯為 `ImportProgress` 元件：

**檔案**：`web/src/components/admin/ImportProgress.tsx`

**Props**：
```typescript
interface ImportProgressProps {
  jobId: string
  onComplete?: () => void  // 完成後回調（重新載入列表）
}
```

**職責**：
- 監聽 WebSocket import_progress / import_complete / import_error 訊息
- 顯示進度條、成功/跳過/失敗計數、當前檔名
- 完成後顯示摘要

### 10. 前端 TypeScript 型別更新

```typescript
interface MediaSource {
  id: string
  label: string
  mount_path: string
  enabled: boolean
  video_count: number  // 新增
  created_at: string
  updated_at: string
}
```

### 11. API 函式

在 `web/src/api/admin.ts` 新增：
- `getMediaSources()` — GET /api/media-sources
- `createMediaSource(data)` — POST /api/media-sources
- `updateMediaSource(id, data)` — PUT /api/media-sources/:id
- `deleteMediaSource(id)` — DELETE /api/media-sources/:id

（匯入相關 API 已在 Phase 12 建立）

### 12. 錯誤處理

- 所有 API 呼叫失敗時在操作位置顯示錯誤訊息
- 列表載入失敗：顯示錯誤狀態 + 重試按鈕，不顯示空列表
- 網路錯誤與業務錯誤分開處理

## 收尾工作

### CLAUDE.md 更新
1. 專案概述：「影片存放在 MinIO」→「影片保留在本機磁碟，系統直接讀取串流；MinIO 僅存縮圖與預覽」
2. 新增 WebSocket 規範區段
3. 新增路徑安全規範區段
4. Docker 規範：新增磁碟層級掛載策略

### ROADMAP.md 更新
標記完成：非同步匯入、路徑驗證、進度回報、Presigned URL 續期

### README.md 更新
- 架構圖：presigned URL → API stream endpoint
- Quick Start：volume mount 改為磁碟層級掛載、匯入方式改為 Admin UI
- API Overview：新增 media source、import job、WebSocket 端點

## 不在範圍內

- 使用者管理頁面、系統監控 dashboard
- 新的前端 UI library
- 拖曳排序、匯入歷史記錄
