# Plan ② — X-Accel-Redirect 卸載串流 byte 路徑

> 場景：**Refactor（後端 + infra，不改行為）**。建議另開對話實作。
> done = `task verify` 綠 + `task test-integration` 綠 + 實機串流正常。
> 改到串流路徑，依 CLAUDE.md 必須跑 integration。

## 目標

影片 bytes 不再穿過 Go 程序：Go 只做「驗 token + 路徑安全」，然後回一個 `X-Accel-Redirect` 標頭，**由 nginx 直接從磁碟讀檔送出（原生 Range）**。授權不變，Go 不再被搬位元組佔住。

## 背景現況

- 目前 `Stream()`（`internal/handler/video_handler.go:226`）末段是 `c.Header("Content-Type", ...)` + `http.ServeFile(...)`，即每個 GB 都穿過 Go 程序。
- 影片掛載是**個別子目錄**：`/mnt/d/AdultsV → /mnt/host/D/AdultsV`、`/mnt/g/下載 → /mnt/host/G/下載`，路徑**含中文與空白**。
- **只有 API 容器掛了影片 volume，nginx 沒有。**
- **dev 模式 `npm run dev` 是 vite proxy 直連 :8080 Go，不經 nginx。**

## 設計決策：用 env 開關（dev/prod 雙模式）

`npm run dev` 直連 Go（無 nginx），X-Accel 標頭到瀏覽器會壞掉。所以加一個 infra 參數 `VIDEO_XACCEL_PREFIX`：

- **空（預設）** → Go 走原本的 `http.ServeFile`（dev、單元測試、不經 nginx 的場景全部不變）。
- **設成 `/internal-video/`（prod）** → Go 改回 `X-Accel-Redirect`。

符合 CLAUDE.md「基礎設施/部署拓樸參數放 env」的例外（它不是業務參數，是「我背後有沒有 accel-capable proxy」的拓樸事實）。

## 變更清單

| 檔案 | 動作 |
|---|---|
| `internal/config/config.go` | 加 `VideoXAccelPrefix string` = `getEnv("VIDEO_XACCEL_PREFIX", "")` |
| `internal/handler/video_handler.go` | `Stream()` 末段：prefix 非空 → 發 X-Accel 標頭；否則維持 ServeFile。需 inject prefix（改 `NewVideoHandler` 或從 config） |
| `internal/handler/video_handler_test.go` | 加 `TestStream_XAccelRedirect`：設 prefix，斷言標頭值（含中文/空白的 URL-encode）、body 空 |
| `nginx/nginx.conf` | 加 `internal` location `/internal-video/` + 媒體副檔名 `types` 區塊 |
| `docker-compose.yml` & `docker-compose.prod.yml` | `vaultflix-nginx` 加上與 API **相同**的 `/mnt/host/...:ro` volume |
| `.env` 範例 / 部署文件 | prod 設 `VIDEO_XACCEL_PREFIX=/internal-video/` |

## 關鍵實作細節

### handler 末段（取代現在的 `c.Header(Content-Type)+ServeFile`）

```go
// cleanPath 已過路徑安全檢查（維持不動）
if h.xaccelPrefix != "" {
    rel := strings.TrimPrefix(cleanPath, model.AllowedMountPrefix) // 去掉 /mnt/host/
    if rel == cleanPath {                                          // 防禦：不在前綴下 → 退回直送
        slog.Warn("xaccel path not under mount prefix, falling back", "path", cleanPath)
    } else {
        c.Header("X-Accel-Redirect", h.xaccelPrefix+encodePathSegments(rel))
        c.Status(http.StatusOK)   // 空 body，nginx 接手
        return
    }
}
c.Header("Content-Type", video.MimeType)
http.ServeFile(c.Writer, c.Request, cleanPath)
```

### URL-encode（中文/空白檔名的正確性關鍵）— 逐段 escape、保留斜線

```go
func encodePathSegments(p string) string {
    parts := strings.Split(p, "/")
    for i, s := range parts {
        parts[i] = url.PathEscape(s)   // 空白→%20、中文→%E2%80%A6
    }
    return strings.Join(parts, "/")
}
```

> nginx 收到 `X-Accel-Redirect` 會自行 unescape 再查檔。`url.PathEscape` 把空白編成 `%20`（非 `+`），正合 nginx 預期。

### nginx 新增 internal location（安全核心是 `internal;`）

```nginx
location /internal-video/ {
    internal;                 # ← 只能被 X-Accel-Redirect 內部觸發，外部直連一律 404
    alias /mnt/host/;         # /internal-video/D/AdultsV/x.mp4 → /mnt/host/D/AdultsV/x.mp4
    types {                   # app 只匯入這 5 種，閉集合 → Content-Type 由副檔名決定
        video/mp4 mp4; video/x-matroska mkv; video/x-msvideo avi;
        video/x-ms-wmv wmv; video/quicktime mov;
    }
    default_type application/octet-stream;
    sendfile on;
}
```

### compose 兩份都要給 nginx 同樣的 mount（否則 nginx 看不到檔）

```yaml
vaultflix-nginx:
  volumes:
    - /mnt/d/AdultsV:/mnt/host/D/AdultsV:ro
    - /mnt/g/下載:/mnt/host/G/下載:ro
```

> 可考慮用 YAML anchor 共用 API/nginx 的 volume 區塊，避免兩處 drift。

## 資料流（改後）

```
iPhone → ngrok → nginx(/api/videos/:id/stream)
                   → Go：JWTAuth(吃 stream token) + 路徑安全 → 回 X-Accel-Redirect 標頭(無 body)
                   → nginx 攔截標頭 → internal /internal-video/ → 直接讀磁碟 + 原生 Range → 送出
```

授權仍在 Go（中介層不變），bytes 不再經過 Go。

## 風險 / Trade-off

- **多一個 mount 消費者**：nginx 與 API 都要掛同一批磁碟 → 新增磁碟時兩處都要改（原本只改 API）。耦合上升，但與「磁碟層級掛載策略」相容。緩解：compose 用 YAML anchor 共用 volume 區塊。
- **`internal;` 漏設 = 安全洞**：若忘了 `internal;`，任何人可直接 GET `/internal-video/...` 繞過 stream token。**最高風險點**，驗收要專門測「外部直連該 location 必須 404」。
- **Content-Type 改由 nginx 決定**：DB 的 `video.MimeType` 在磁碟路徑下不再生效（legacy MinIO 路徑不受影響）。因匯入只允許 5 種副檔名，types 區塊涵蓋即可；未來加格式要同步補 types。
- **dev/prod 行為分歧**：env 未設時走 ServeFile、設了走 accel → 兩條路徑都要測，否則「dev 正常但 prod 壞」或反之。單元測試覆蓋 accel 分支、integration 覆蓋實際 nginx 直送。
- **本案不解決 ngrok 瓶頸**：這是「Go 不被佔住」的架構優化，對「串流變快」幫助有限（瓶頸是 ngrok 頻寬）。若目標是 mobile 串流體驗，換 Cloudflare Tunnel 的優先序更高 —— 先對齊預期，別做完發現「沒變快」。

## 驗收

- `task verify` 綠（新單元測試含中文檔名的 encode 斷言）。
- `task test-integration` 綠；額外手動驗：prod compose 起來後
  1. 正常播放（經 nginx accel）
  2. 外部直連 `/internal-video/任意路徑` 回 404
  3. Range/拖曳進度正常（nginx 原生 Range）
- 觀測：串流期間 Go 的 CPU/連線不再隨影片大小上升。

## 估時

0.5–1 天（大半在 compose mount 與實機 Range 驗證）。
