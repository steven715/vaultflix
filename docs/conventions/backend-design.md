# 後端設計原則

> 產生給瀏覽器的 URL、新增設定參數、處理使用者可控的檔案路徑前先讀。

## URL 消費者意識

產生 URL 時必須考慮**誰會消費這個 URL**：

- Server-to-server 通訊（API → MinIO）使用 Docker 內部 hostname（如 `minio:9000`）
- 前端/瀏覽器消費的 URL（presigned URL、thumbnail URL）必須使用 public-facing endpoint（如 `localhost:9000`）
- Config 中明確區分這兩種 endpoint，命名反映用途：`MINIO_ENDPOINT`（內部）vs `MINIO_PUBLIC_ENDPOINT`（外部）
- 產生 presigned URL 時，使用以 public endpoint 初始化的獨立 client，確保簽名與 hostname 一致

```go
// ✅ 正確：兩個 client 各司其職
internalClient, _ := minio.New(cfg.MinIOEndpoint, opts)       // 上傳、刪除
presignClient, _ := minio.New(cfg.MinIOPublicEndpoint, opts)   // 產生 presigned URL

// ❌ 錯誤：用內部 client 產生 URL 再替換 host（簽名會不匹配）
url := internalClient.PresignedGetObject(...)
url.Host = publicEndpoint // 簽名基於 minio:9000，瀏覽器送 localhost:9000 → 驗證失敗
```

## 執行期可調性原則

業務行為參數（如掃描路徑、數量限制、過期時間）應優先從 API 請求參數傳入，環境變數只作為 fallback 預設值。

**判斷標準**：「改這個值需要重啟服務嗎？」如果不應該，就不該只存在於環境變數。基礎設施連線資訊（DB DSN、MinIO endpoint）例外，因為連線本身需要重建。

```go
// ✅ 正確：業務參數從 request 傳入，執行期可調
type importRequest struct {
    SourceDir string `json:"source_dir" binding:"required"`
}
func (h *VideoHandler) Import(c *gin.Context) {
    var req importRequest
    // ...
    result, err := h.importService.Run(ctx, req.SourceDir)
}

// ❌ 錯誤：業務參數寫死在環境變數，改路徑要重啟服務
type ImportService struct {
    sourceDir string // 從 env var 讀入，啟動後不可變
}
```

- 基礎設施參數（DB、MinIO、JWT secret）→ 環境變數，啟動時載入
- 業務行為參數（匯入路徑、分頁大小、URL 有效期）→ API 請求參數，執行期可調

## 路徑安全規範

### 基本原則

所有使用者可控的檔案路徑必須經過驗證，防止路徑穿越攻擊。

### 標準 Pattern

```go
// 1. 定義允許的前綴
const AllowedMountPrefix = "/mnt/host/"

// 2. Clean + prefix 檢查
cleaned := filepath.Clean(path)
if !strings.HasPrefix(cleaned, strings.TrimSuffix(prefix, string(filepath.Separator))) {
    return model.ErrPathNotAllowed
}

// 3. 拒絕 Clean 後與原始路徑不一致的輸入（含 .., //, 結尾斜線等）
if cleaned != path {
    return model.ErrPathNotAllowed
}

// 4. 驗證路徑存在且為目錄
info, err := os.Stat(cleaned)
```

### Video 檔案路徑一律經 `MediaSourceService.ResolveFile`

把 Video 的 `(source_id, file_path)` 變成磁碟路徑，只能呼叫 `ResolveFile`，不可自己 `filepath.Join(source.MountPath, ...)`。它一次做完：查 Media Source、拒絕停用的 source、檢查路徑落在該 source 的 mount 內**且**在注入的 mount prefix 內、`os.Stat` 確認存在。

```go
// ✅ 正確：所有規則在 seam 後面
abs, err := s.files.ResolveFile(ctx, *v.SourceID, *v.FilePath)

// ❌ 錯誤：手接路徑，漏掉 enabled / traversal / 存在檢查
abs := filepath.Join(source.MountPath, *v.FilePath)
```

依賴它的 service 使用 `mediaFileResolver` interface；測試用 `mock.ResolveUnder` / `mock.ResolveFailing`。Backfill 遇到 `ErrMediaSourceDisabled` 視為**略過**（不算失敗）。

### Sentinel Errors

- `model.ErrPathNotAllowed` — 路徑不在允許前綴內，或包含非法組件
- `model.ErrPathNotExist` — 路徑不存在於檔案系統
- `model.ErrMediaSourceDisabled` — Video 所屬的 Media Source 已停用

## 多表寫入的原子性

一個使用者動作要寫多張表、且「寫一半」會留下錯誤狀態時（例如接受 Metadata Suggestion：Video Metadata、Performer、genre Tag、刪除 Suggestion），整組寫入放進**一個 repository 方法、一個交易**（`pool.Begin` → 全部成功才 `Commit`，任何錯誤 `Rollback`），不要在 service 裡逐一呼叫多個 repository。

- 交易內沿用同 package 既有的 query 常數，不另寫一份 SQL
- 「把已存在的關聯再連一次」是合法操作時，關聯表用 `ON CONFLICT DO NOTHING`，不要回 `ErrConflict` 讓整個動作失敗
- 以 `openTestPool`（ADR-0012）對真 Postgres 測：成功路徑 + 中途失敗後**什麼都沒寫入**
