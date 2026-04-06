# Phase 10 — 影片串流重構（http.ServeFile）設計文件

## 場景類型：Refactor

## 背景

Phase 9 完成後，新匯入的影片記錄本機路徑（`source_id` + `file_path`），但串流端點仍是 MinIO presigned URL 模式，導致新影片無法播放。本 Phase 新增 `/api/videos/:id/stream` endpoint，用 `http.ServeFile` 從本機路徑直接串流。

## 設計決策

| 決策 | 選擇 | 理由 |
|------|------|------|
| 舊 MinIO 影片向後相容 | 選項 A：307 redirect 到 presigned URL | 零停機、程式碼少、presign client 因縮圖仍保留 |
| `stream_url` 產生方式 | 後端回傳完整路徑 | 與現有 pattern 一致（縮圖同樣由後端產生）、前端不需區分新舊模式 |

## 改動範圍

### 1. Auth Middleware — 支援 query param token

**檔案**: `internal/middleware/auth.go`

修改 JWT auth middleware，在 `Authorization: Bearer` header 提取之後，加 fallback 讀 `?token=` query parameter。Header 優先。

邏輯：
1. 嘗試從 `Authorization` header 提取 Bearer token
2. 若為空，嘗試從 `c.Query("token")` 提取
3. 兩者都為空 → 401
4. 後續 JWT 驗證邏輯不變

**安全 trade-off**（加註解說明）：query param 中的 token 會出現在 server access log 和瀏覽器歷史紀錄。`<video src>` 無法自訂 header，這是業界常見做法（WebSocket、SSE、file download 同理）。自用場景可接受。

### 2. Stream Handler

**檔案**: `internal/handler/video_handler.go`（新增 `Stream` method）

`GET /api/videos/:id/stream`

流程：
1. 取得 video ID → 透過 video service 查詢
2. 判斷影片模式：
   - **新模式**（`source_id` + `file_path` 有值）：
     - 查 media source → 檢查 `source.Enabled`（停用 → 503）
     - `filepath.Join(source.MountPath, *video.FilePath)` → `filepath.Clean`
     - 路徑穿越防護：`strings.HasPrefix(cleanPath, AllowedMountPrefix)`（失敗 → 403）
     - `os.Stat` 檢查檔案存在（不存在 → 404 + 明確訊息）
     - 設定 `Content-Type` header（從 `video.MimeType`）
     - `http.ServeFile(c.Writer, c.Request, cleanPath)`
   - **舊模式**（`source_id` 為 NULL，`minio_object_key` 有值）：
     - 產生 presigned URL → `c.Redirect(307, presignedURL)`
   - **都沒有**：500 錯誤

`http.ServeFile` 原生支援：Range Request（seeking）、`Content-Length`、`If-Modified-Since`（304）。

**依賴**：Stream handler 需要存取 `MediaSourceService`。`VideoHandler` 需新增此依賴。

**AllowedMountPrefix**：目前定義在 `media_source_service.go`。需 export 為 `AllowedMountPrefix`（已是 exported），stream handler 直接引用 `service.AllowedMountPrefix`。

### 3. Video Service 改動

**檔案**: `internal/service/video_service.go`

`GetByID` 中 `stream_url` 產生邏輯改為：
- 所有影片統一回傳 `/api/videos/{id}/stream`（相對路徑）
- 移除 `GetByID` 中的 video presigned URL 產生（`minioSvc.GeneratePresignedURL` 呼叫）
- 移除 `expiry` 參數（不再需要）
- 縮圖 presigned URL 保持不變

`List` 中不回傳 `stream_url`（本來就沒有），縮圖 URL 不動。

### 4. Video Handler 清理

**檔案**: `internal/handler/video_handler.go`

`GetByID` method：
- 移除 `url_expiry_minutes` query parameter 解析
- 調整 `videoService.GetByID` 呼叫（移除 expiry 參數）

### 5. 前端改動

**檔案**: `web/src/pages/PlayerPage.tsx`

- 從 `useAuth()` context 取得 `token`
- `<video src>` 改為 `` `${video.stream_url}?token=${token}` ``
- token 為空時不設 src，避免送出無 token 的請求
- 移除與 presigned URL expiry 相關的邏輯（如果有）

### 6. Nginx 配置

**檔案**: `web/nginx.conf`

為 stream endpoint 新增專用 location block（放在通用 `/api/` 之前）：

```nginx
location ~ ^/api/videos/[^/]+/stream$ {
    proxy_pass http://vaultflix-api:8080;
    proxy_buffering off;
    client_max_body_size 0;
    proxy_set_header Range $http_range;
    proxy_set_header If-Range $http_if_range;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

選擇獨立 location 而非修改通用 `/api/`，避免對其他 API endpoint 造成非預期影響（如 `proxy_buffering off` 影響小型 JSON 回應的效能）。

### 7. Casbin Policy

**檔案**: `casbin/policy.csv`

新增：
```
p, viewer, /api/videos/:id/stream, GET
```

admin 已有 `/api/*, GET` wildcard，不需額外加。

### 8. 路由註冊

**檔案**: `cmd/server/main.go`

在 video 路由群組加：
```go
api.GET("/videos/:id/stream", videoHandler.Stream)
```

位於 JWTAuth + CasbinRBAC middleware 之後，與其他 video 路由一致。

### 9. MinIO Service 清理

- 保留 `GeneratePresignedURL` — 舊模式 redirect 和縮圖仍需要
- 保留 `presignClient` — 同上
- 保留 `GenerateThumbnailPresignedURL` — 不動

## 測試計畫

### Stream Handler 測試

Table-driven 測試：

| 場景 | 預期 |
|------|------|
| 正常串流（新模式） | 200 + 正確 Content-Type + body |
| Range Request | 206 Partial Content + Content-Range header |
| video ID 不存在 | 404 |
| DB 有記錄但檔案不在磁碟 | 404 + 明確訊息 |
| source 停用 | 503 |
| 舊模式影片（source_id NULL） | 307 redirect 到 presigned URL |
| file_path 含 `..` 路徑穿越 | 403 |

### Auth Token Query Param 測試

| 場景 | 預期 |
|------|------|
| Header 有效 token | 通過驗證（既有行為不破壞） |
| Query param 有效 token | 通過驗證 |
| Query param 無效 token | 401 |
| 無 header 也無 query param | 401 |
| Header + query param 同時存在 | Header 優先 |

## Trade-offs

| 決策 | 優點 | 缺點 |
|------|------|------|
| query param token | 解決 `<video src>` 無法帶 header 的限制 | token 出現在 server log / 瀏覽器歷史，自用可接受 |
| 舊模式 307 redirect | 零停機，舊影片立即可播 | 保留 MinIO presigned URL 路徑，多一個分支 |
| `http.ServeFile` | 原生 Range/304 支援，零額外程式碼 | Go process 直接串流，大量並發時 memory 較高（自用不成問題） |
| 獨立 nginx location | 不影響其他 API endpoint | 多一段配置需維護 |

## 不在範圍內

- 不清理 MinIO bucket 中的舊檔案
- 不刪除 `minio_object_key` 欄位
- 不處理 token refresh 對播放中影片的影響
- 不建立前端 Admin UI
