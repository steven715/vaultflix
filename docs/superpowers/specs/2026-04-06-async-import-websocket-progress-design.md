# Phase 12 — 非同步匯入 + WebSocket 進度推送

## 場景類型：Refactor

## 背景

`POST /api/videos/import` 目前同步阻塞 — HTTP request 等所有影片處理完才回傳。大量影片（18GB / 4m40s）會導致 HTTP timeout 或前端無回饋。

本 Phase 改為非同步：API 立即回傳 job ID，背景 goroutine 執行匯入，透過 WebSocket 推送逐檔進度。

## 設計決策

- **不用 job queue / Redis**：單用戶場景，goroutine + `sync.Map` 管理 in-memory job 狀態即可
- **前端位置**：留在 `VideoManagePage`，替換現有同步匯入 modal
- **移除 `ws-test`**：匯入進度已充分驗證 WebSocket
- **Casbin**：現有 admin 萬用規則已涵蓋新路由
- **`processFile` 重構**：回傳 status result struct 取代 skip-error 模式

## 後端

### Import Job Model

```go
type ImportJob struct {
    ID          string        `json:"id"`
    SourceID    string        `json:"source_id"`
    SourceLabel string        `json:"source_label"`
    Status      string        `json:"status"`      // "running", "completed", "failed"
    Total       int           `json:"total"`
    Processed   int           `json:"processed"`
    Imported    int           `json:"imported"`
    Skipped     int           `json:"skipped"`
    Failed      int           `json:"failed"`
    Errors      []ImportError `json:"errors"`
    StartedAt   time.Time     `json:"started_at"`
    FinishedAt  *time.Time    `json:"finished_at,omitempty"`
}

type ImportError struct {
    FileName string `json:"file_name"`
    Error    string `json:"error"`
}

type ImportProgress struct {
    JobID    string `json:"job_id"`
    FileName string `json:"file_name"`
    Current  int    `json:"current"`
    Total    int    `json:"total"`
    Status   string `json:"status"`  // "processing", "success", "skipped", "error"
    Error    string `json:"error,omitempty"`
}
```

### Import Service 改造

- 新增 `Notifier` 依賴 + `sync.Map`（activeJobs）+ `sync.Mutex`（importMu）
- `StartAsync(ctx, source, userID)` → 檢查 mutex、建立 job、啟動 goroutine、立即回傳
- `runImport(ctx, job, source, userID)` → 核心迴圈 + WebSocket 推送
- `processOneFile(ctx, job, source, file)` → 從現有 `processFile` 重構，回傳 result struct
- `GetJob(jobID)` / `GetActiveJob()` → 查詢方法
- 背景 goroutine 使用 `context.Background()`，不用 request context
- 舊的同步 `Run` 方法移除

### Handler 改造

| 端點 | 行為 |
|------|------|
| `POST /api/videos/import` | 改回 202 Accepted + job |
| `GET /api/import-jobs/active` | 查詢進行中 job |
| `GET /api/import-jobs/:id` | 查詢特定 job |

路由順序：`/active` 在 `/:id` 之前。

### 清理

- 移除 `POST /api/admin/ws-test` 端點及相關 handler 方法

## 前端

### 匯入 UI 改造（VideoManagePage）

四種狀態：`idle` → `importing` → `completed` / `failed`

- **idle**：media source 下拉選單 + 開始匯入按鈕
- **importing**：進度條 + 當前檔名 + 成功/跳過/失敗計數，按鈕停用
- **completed**：完成摘要，失敗項可展開
- **failed**：錯誤訊息 + 明細

### API 更新

- `importVideos(sourceID)` 改為送 `{ source_id }` 並期待 202
- 新增 `getActiveImportJob()` 和 `getImportJob(id)`
- 新增 `listMediaSources()` API function

### WebSocket 監聽

監聽 `import_progress`、`import_complete`、`import_error` 訊息，用 `lastMessage` + `useEffect` 更新 UI 狀態。

### 狀態恢復

頁面 mount 時呼叫 `GET /api/import-jobs/active`，有 running job 就恢復進度模式。

### TypeScript 型別

新增 `ImportJob`、`ImportError`、`ImportProgress`、`MediaSource` 型別。

## 測試

### Service 測試

- `TestStartAsync_ReturnsImmediately`
- `TestStartAsync_Conflict`
- `TestRunImport_ProgressMessages`
- `TestRunImport_SkippedAndFailed`
- `TestRunImport_EmptyDirectory`
- `TestRunImport_ScanError`
- `TestGetJob_Found` / `NotFound`
- `TestGetActiveJob_Running` / `None`

使用 `assert.Eventually` polling 等待背景 goroutine 完成。

### Handler 測試

- `TestImportHandler_Async_Success` → 202
- `TestImportHandler_Async_Conflict` → 409
- `TestGetImportJobHandler_Found` / `NotFound`
- `TestGetActiveImportJobHandler_NoJob` → `{ data: null }`

## 不在範圍內

- 完整 Admin dashboard（Phase 13）
- 匯入取消功能
- MinIO bucket 舊檔清理
- Job 歷史持久化
