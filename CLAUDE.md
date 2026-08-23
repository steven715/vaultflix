# CLAUDE.md — Vaultflix 開發規範

## 專案概述

Vaultflix 是一個 Go + React 的個人影片管理與串流平台。後端為 Go API Server，前端為 React SPA，影片保留在本機磁碟，系統直接讀取串流；MinIO 僅存縮圖與預覽，metadata 存於 PostgreSQL。

---

## 語言與版本

- Go 1.25
- PostgreSQL 16
- React 19 + TypeScript
- Docker Compose V2

---

## Go 編碼規範

### 命名

- **Package**：全小寫單字，不用底線或混合大小寫。`handler`, `service`, `repository`, `model`, `config`, `middleware`
- **檔案名**：snake_case。`video_handler.go`, `auth_service.go`
- **Struct / Interface**：PascalCase。`VideoService`, `UserRepository`
- **Interface 命名**：行為導向，不加 `I` 前綴。用 `VideoRepository` 而非 `IVideoRepository`
- **變數 / 函式**：camelCase。`videoID`, `getUserByID`
- **常數**：PascalCase（exported）或 camelCase（unexported）。不用 `ALL_CAPS`
- **Acronyms**：保持全大寫或全小寫。`userID` 不是 `userId`，`httpClient` 不是 `HTTPClient`（例外：首字母縮寫在開頭且 unexported 時用全小寫）

### Error Handling

```go
// ✅ 正確：每個 error 都要處理，不吞掉
result, err := doSomething()
if err != nil {
    return fmt.Errorf("doSomething failed: %w", err)
}

// ✅ 正確：用 %w wrap error，保留 error chain
return fmt.Errorf("failed to get video %s: %w", videoID, err)

// ❌ 禁止：裸 return error 不加 context
return err

// ❌ 禁止：用 log 取代 return error（除非在最頂層 handler）
log.Println(err)
// 繼續執行...

// ❌ 禁止：忽略 error
result, _ := doSomething()
```

- Error message 用小寫開頭，不加句號
- 只在 handler 層做 log + HTTP response，service / repository 層只 wrap 和 return
- 使用 `errors.Is()` 和 `errors.As()` 判斷 error 類型，不用字串比對

### Struct 與 Function 設計

```go
// ✅ 正確：constructor 回傳 pointer
func NewVideoService(repo VideoRepository, minio MinIOClient) *VideoService {
    return &VideoService{repo: repo, minio: minio}
}

// ✅ 正確：依賴透過 constructor injection，不用全域變數
type VideoService struct {
    repo  VideoRepository
    minio MinIOClient
}

// ❌ 禁止：全域變數持有依賴
var db *pgx.Pool
```

- 每個 struct method 的 receiver 統一用 pointer receiver
- 一個檔案只放一個主要的 struct 及其 methods
- Function 參數超過 3 個時，使用 options struct

### Context 傳遞

```go
// ✅ 正確：context 作為第一個參數貫穿所有層
func (s *VideoService) GetByID(ctx context.Context, id string) (*Video, error)
func (r *VideoRepo) FindByID(ctx context.Context, id string) (*Video, error)

// ❌ 禁止：不傳 context
func (s *VideoService) GetByID(id string) (*Video, error)
```

### URL 消費者意識

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

### 執行期可調性原則

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

### Log 規範

- 使用 `log/slog`（Go 1.21+ 標準庫），不引入第三方 log library
- Log level 語意：
  - `slog.Debug`：開發除錯用，生產環境不輸出
  - `slog.Info`：正常業務事件（服務啟動、影片匯入完成、使用者登入）
  - `slog.Warn`：可恢復的異常（pre-signed URL 產生失敗、重試中）
  - `slog.Error`：不可恢復的錯誤（DB 連線斷開、MinIO 不可用）
- Log 必須帶結構化欄位：

```go
// ✅ 正確
slog.Info("video imported",
    "video_id", video.ID,
    "duration", video.DurationSeconds,
    "size_bytes", video.FileSizeBytes,
)

// ❌ 禁止：拼接字串
slog.Info(fmt.Sprintf("video %s imported, duration: %d", video.ID, video.DurationSeconds))
```

---

## 分層架構規範

```
Handler（HTTP 層）
   ↓ 呼叫
Service（業務邏輯層）
   ↓ 呼叫
Repository（資料存取層）
```

### 各層職責邊界

| 層 | 做什麼 | 不做什麼 |
|---|--------|---------|
| Handler | 解析 HTTP request、呼叫 service、組裝 HTTP response、log error | 不寫 SQL、不直接操作 MinIO、不處理業務邏輯 |
| Service | 業務邏輯、跨 repository 協調、呼叫外部服務（MinIO） | 不碰 HTTP request/response、不寫 SQL |
| Repository | 執行 SQL 查詢、回傳 model struct | 不處理業務邏輯、不碰 HTTP、不呼叫其他 repository |

### Handler 回應格式

統一使用以下 JSON 結構：

```go
// 成功
type SuccessResponse struct {
    Data interface{} `json:"data"`
}

// 分頁
type PaginatedResponse struct {
    Data       interface{} `json:"data"`
    Total      int64       `json:"total"`
    Page       int         `json:"page"`
    PageSize   int         `json:"page_size"`
}

// 錯誤
type ErrorResponse struct {
    Error   string `json:"error"`
    Message string `json:"message"`
}
```

HTTP Status Code 使用規則：
- `200`：GET 成功、PUT 更新成功
- `201`：POST 建立成功
- `204`：DELETE 成功（無 body）
- `400`：request 格式錯誤、參數驗證失敗
- `401`：未認證（無 token 或 token 過期）
- `403`：已認證但無權限（Casbin 拒絕）
- `404`：資源不存在
- `500`：伺服器內部錯誤

---

## SQL 規範

- SQL 關鍵字全大寫：`SELECT`, `FROM`, `WHERE`, `INSERT INTO`
- Table / column 名全小寫 snake_case
- 使用 parameterized query（`$1`, `$2`），絕不拼接 SQL 字串
- 每個 query 寫成 const string 放在 repository 檔案頂部：

```go
const queryGetVideoByID = `
    SELECT id, title, description, minio_object_key, thumbnail_key,
           duration_seconds, resolution, file_size_bytes, mime_type,
           created_at, updated_at
    FROM videos
    WHERE id = $1
`
```

- Migration 檔案命名：`NNN_description.up.sql` / `NNN_description.down.sql`
- `down.sql` 必須完整可逆（能回滾到上一版本）

---

## 前端規範（React + TypeScript）

### 前後端 Response 契約

後端所有成功回應使用 `{ data: ... }` wrapper（`SuccessResponse`）。前端 API client 層是唯一處理這個契約的地方：

- axios response interceptor 統一解開 `SuccessResponse` wrapper，讓呼叫端直接拿到 `data` 內容
- 個別 API function 不應出現 `res.data.data` 的雙層解包 — 如果需要這樣寫，代表 interceptor 沒有正確處理
- 分頁回應（`PaginatedResponse`）因為頂層就包含 `total`、`page` 等欄位，不適用自動解包，需在 interceptor 中區分處理

```typescript
// ✅ 正確：interceptor 統一解包，呼叫端簡潔
// client.ts
client.interceptors.response.use((res) => {
  if (res.data?.data !== undefined) return { ...res, data: res.data.data }
  return res
})
// auth.ts
const res = await client.post<LoginResponse>('/auth/login', { username, password })
return res.data // 直接是 { token: "..." }

// ❌ 錯誤：每個呼叫端自己解包，容易遺漏
return res.data.data
```

### 自動重試與錯誤處理

- 任何自動重試邏輯必須設定**重試上限**，且重試次數用 `useRef` 追蹤，不用 `useState`（避免觸發重新渲染導致迴圈）
- 事件驅動的錯誤處理（如 `<video onError>`）不可直接觸發導致同一事件再次發生的狀態更新，必須有中斷條件

```tsx
// ✅ 正確：ref 追蹤重試次數，超過上限停止
const retryRef = useRef(0)
function handleVideoError() {
  if (retryRef.current >= 1) return
  retryRef.current += 1
  refetchAndReload()
}

// ❌ 錯誤：無限重試 — error → refetch → setState → re-render → error → ...
function handleVideoError() {
  refetchAndReload() // 永遠重試，沒有上限
}
```

### useEffect 非同步操作

- `useEffect` 中執行 async 操作時，必須用 cleanup flag 防止 unmount 後的狀態更新
- 依賴陣列只放真正的觸發條件（如 route param `id`），不放 `useCallback` 包裝的函式引用

```tsx
// ✅ 正確：cleanup flag + 直接依賴 id
useEffect(() => {
  let cancelled = false
  const fetchData = async () => {
    const data = await getData(id)
    if (!cancelled) setData(data)
  }
  fetchData()
  return () => { cancelled = true }
}, [id])

// ❌ 錯誤：依賴 useCallback 函式引用，可能因 closure 不穩定導致重複執行
const fetchData = useCallback(async () => { ... }, [id])
useEffect(() => { fetchData() }, [fetchData])
```

### 假時鐘測試：前置條件要用 tick 逼出來，不要靠真實時間燒過去

`vi.useFakeTimers({ shouldAdvanceTime: true })` 讓假時鐘跟著真實時間前進，方便 awaited 的資料載入正常解析。代價是：**任何「靠這段等待剛好燒掉 N 毫秒」才成立的前置條件都是 flaky**，會隨機器快慢、CI 負載翻面 —— 同一份 tree 可以一次綠一次紅。

實際踩過的地雷：`usePlaybackStats` 是在 500ms 的 publish tick **裡面**才綁 `<video>` 的 listener（`streamPath` 比 `<video>` 早一個 render 變 non-null，effect 當下那次 `publish()` 看到的 `videoRef.current` 還是 null，而 effect 不會因為元素掛上而重跑）。測試若在任何 tick 跑之前就 `fireEvent.play()`，`onPlay` 根本還沒註冊 → `playStartRef` 是 null → ttff 算不出來 → unmount 的 telemetry beacon 被 `ttffMs == null && watchedMs <= 0` 這道 guard 擋掉 → 斷言整串垮。

規則：**要讓事件被聽到，先逼出綁定的那個 tick，再 fire 事件，再逼一次 tick 讓 hook 計算。**

```tsx
// ✅ 正確：每個 tick 都是明確逼出來的
await act(async () => { await vi.advanceTimersByTimeAsync(600) })  // tick 1：hook 綁上 listener
fireEvent.play(videoEl)                                            // 這下才聽得到
await act(async () => { await vi.advanceTimersByTimeAsync(600) })  // tick 2：hook 算 ttff

// ❌ 錯誤：假設 awaited 的載入「會順便」讓 interval 跑過一輪
await screen.findByText('T')
fireEvent.play(videoEl)        // 機器夠快 → 一個 tick 都還沒跑 → 事件掉了
await act(async () => { await vi.advanceTimersByTimeAsync(600) })
```

判斷標準：把這個測試放到一台快 10 倍的機器上，結論還成立嗎？只要答案取決於「那段 await 花了多久」，就是 flaky。

### 測試紅燈先分辨 flaky 再分辨壞掉

CI 紅不代表「這次改動弄壞了什麼」—— `task verify` 是整包 gate，任何既有或隨機的失敗都會在下一個推 code 的人頭上炸開。判斷順序：

1. 這次改動有沒有碰到相關檔案？（`git diff --stat <上次綠的 commit> HEAD -- <路徑>`）
2. 沒碰到的話，比對 **tree hash**：`git rev-parse <A>^{tree}` vs `<B>^{tree}`。同 tree 不同結果 = flaky，不是壞掉
3. 本機重跑多次取得失敗率，別用單次結果下結論

### 同路徑重新導航的 refetch

點擊指向「目前所在路徑」的連結（如已在首頁時再點 logo / 首頁）不會改變 URL 參數，靠 `[query]`、`[searchParams]` 之類的依賴**不會觸發 refetch**，畫面看起來「卡住不更新」。需要「每次導航都重抓」的資料（如首頁輪播推薦、續看清單），依賴 `useLocation().key` —— React Router 每次導航（即使目標與現況相同）都會 push 新 entry 並產生新的 `location.key`。

```tsx
// ✅ 正確：同路徑再點 logo / 首頁也會 refetch
const location = useLocation()
useEffect(() => {
  if (query) return
  let cancelled = false
  getTodayRecommendations().then((items) => !cancelled && setRecommendations(items))
  return () => { cancelled = true }
}, [query, location.key])

// ❌ 錯誤：已在首頁時點 logo，query 沒變 → effect 不跑 → 推薦永遠不更新
}, [query])
```

### 具名導航控制要導到目的地，不要用 `navigate(-1)`

當一個按鈕/連結的文案承諾了**具體目的地**（如「返回片庫」、「回首頁」），它就必須導向那個路由（`navigate('/')`），不能用 `navigate(-1)` / `history.back()`。`navigate(-1)` 是「上一頁」而非「片庫」—— 當 back stack 裡上一頁剛好是**同類型頁面**時（如播放頁 A → 從「接著看」點進播放頁 B），按「返回片庫」會回到 A 而不是片庫，與文案不符。

判斷標準：文案是否指名一個固定目的地？是 → `navigate('<目的地>')`；否（純粹語意是「上一步」的通用返回鍵）→ 才用 `navigate(-1)`。

```tsx
// ✅ 正確：文案說「返回片庫」就導到片庫
<button onClick={() => navigate('/')}>返回片庫</button>

// ❌ 錯誤：B → 返回片庫 會回到上一個播放頁 A，不是片庫
<button onClick={() => navigate(-1)}>返回片庫</button>
```

判斷標準：這份資料是否預期「回到此頁就刷新」？是 → 加 `location.key`；否（純由 URL 參數決定、deterministic 的如分頁列表）→ 不加，避免每次導航都多打一次 API。

---

## Docker 規範

- 使用 Docker Compose V2 語法（`services:` 頂層，無 `version:` 欄位）
- 所有服務使用 alpine-based image（除非有特殊需求）
- Volume 命名格式：`vaultflix-<service>-data`（如 `vaultflix-postgres-data`）
- 環境變數透過 `.env` 檔案注入，不寫死在 `docker-compose.yml` 中
- Health check 必須配置在每個服務上
- 對外暴露 port 的服務（如 MinIO），`.env` 中必須同時定義 internal endpoint（Docker hostname）和 public endpoint（host-accessible），命名慣例：`<SERVICE>_ENDPOINT` / `<SERVICE>_PUBLIC_ENDPOINT`

### 前端發版流程（不可變 nginx image）

前端 SPA 直接 build 進 `vaultflix-nginx` image：`nginx/Dockerfile` 是多階段 build，第一階段用 `node:20-alpine` 編譯 `web/`，第二階段把產物 `COPY` 進 nginx 的 `/usr/share/nginx/html`。**沒有 `web_dist` 共享 volume、沒有獨立的 `vaultflix-web` 容器** —— 前端是不可變產物，與 Go API image 對稱。

因為 build context 是 repo root（nginx Dockerfile 要讀 `web/` 與 `nginx/nginx.conf`），compose 的 nginx 服務用 `context: .` + `dockerfile: nginx/Dockerfile`，root 的 `.dockerignore` 已排除 `web/node_modules`、`web/dist`。

正確流程（image 換了 `up -d` 會自動 recreate，無需手動刪 volume）：

```bash
docker compose build vaultflix-nginx
docker compose up -d vaultflix-nginx
# 或一次到位：task deploy
```

改完前端若瀏覽器行為沒變，第一反應是 image 沒重 build（或 PWA/瀏覽器在吃舊快取，hard reload）—— 不再有 named volume 陷阱。

### 磁碟層級掛載策略

影片檔案保留在本機磁碟，透過 Docker volume mount 以唯讀模式掛載整個磁碟。

**磁碟配置是「因機器而異」的設定，不進被 git 追蹤的檔案。** 它住在 gitignore 的 `docker-compose.media.yml`（範本：`docker-compose.media.yml.example`），與 `.env` 同一類。tracked 的 `docker-compose.yml` / `docker-compose.prod.yml` 一行影片掛載都不該有。

```yaml
# docker-compose.media.yml — 用 YAML anchor 一次餵給 api 與 nginx
x-media-mounts: &media-mounts
  - D:/:/mnt/host/D:ro
  - E:/:/mnt/host/E:ro

services:
  vaultflix-api:
    volumes: *media-mounts
  vaultflix-nginx:
    volumes: *media-mounts
```

- 掛載點統一在 `/mnt/host/<磁碟代號>/` 下
- 使用 `:ro`（read-only）防止容器內程式修改原始檔案
- Media source 的 `mount_path` 必須在 `/mnt/host/` 前綴下
- api 與 nginx 必須拿到**完全相同**的掛載（nginx 少一個就無法做 X-Accel byte serving）。用 anchor 而不是抄兩份，讓兩者不可能 drift
- 新增磁碟只需在 `docker-compose.media.yml` 的 anchor 加一行 + 在 Admin UI 新增 media source
- `task up` / `task deploy` 會自動把這個檔案疊在最後；整合測試**刻意不疊**，改掛 `.ci/fixtures`，維持 host OS 無關

**compose 疊加順序有兩個陷阱**（改 compose 檔時務必記得）：

1. `docker-compose.media.yml` 必須是**最後**一個 `-f`。prod 對 api 的 `volumes:` 用了 `!override`，媒體掛載疊在它後面才會 merge 進去，疊在前面會被清掉
2. prod override 的 `build:` 必須明確寫 `dockerfile: Dockerfile`。compose 會 merge `build` map，base 指定了 `dockerfile: Dockerfile.dev`，沒有明確覆寫的話 prod 會拿 dev 的 toolchain image 去發版

---

## Chrome DevTools MCP（前端除錯）

Claude Code 透過 Chrome DevTools Protocol 連接瀏覽器進行前端除錯（截圖、DOM 檢查、Network 監控、Console 讀取等）。

### 前置需求

- **Google Chrome**：安裝於預設路徑 `C:\Program Files\Google\Chrome\Application\chrome.exe`
- **PowerShell 7+（pwsh）**：hook 使用 `shell: "powershell"`，需要 `pwsh` 指令可用。安裝方式：`winget install Microsoft.PowerShell`

### 運作方式

- Plugin 設定在 `.claude/settings.json` 的 `enabledPlugins` 中，clone 後自動啟用
- `PreToolUse` hook 會在 Claude 呼叫任何 chrome-devtools 工具前，自動檢查 port 9222 並啟動 Chrome debug 模式
- 使用獨立的 user-data-dir（`$env:USERPROFILE\.chrome-debug-profile`），不影響日常瀏覽器

### 注意事項

- Chrome 路徑非預設時，需在 `.claude/settings.local.json` 覆寫 hook command
- 若 port 9222 已被佔用（如另一個 Chrome debug instance），hook 會跳過啟動

---

## WebSocket 規範

### Hub Pattern

- WebSocket 連線管理集中在 `internal/websocket/` package
- `Hub` struct 透過 channel 序列化所有 client 註冊/移除/訊息推送，避免 map 並發存取
- 支援 per-user targeted message（`SendToUser`）和全域 broadcast
- 同一使用者可有多個連線（多分頁）

### 訊息協議

```go
type Message struct {
    Type    string      `json:"type"`
    Payload interface{} `json:"payload"`
}
```

已定義的 type：
- `import_progress` — 逐檔匯入進度
- `import_complete` — 匯入完成（含最終 ImportJob）
- `import_error` — 匯入致命錯誤
- `notification` — 通用通知
- `ping` — 心跳

### Notifier Interface

跨層依賴透過 `Notifier` interface 解耦，定義在 `internal/websocket/hub.go`：

```go
type Notifier interface {
    SendToUser(userID string, msg *Message)
    Broadcast(msg *Message)
}
```

Service 層（如 `ImportService`）依賴此 interface，不直接依賴 `Hub` struct。

### 前端重連策略

- 使用 exponential backoff：初始 1s，每次 ×2，上限 30s
- 重連次數上限 20 次，超過停止重連
- 用 `useRef` 追蹤重連次數，避免 re-render 導致計數重置
- 心跳間隔 50s，保持連線活躍

### `onclose` 的副作用要綁 socket 世代，不要用跨世代共用布林

`onclose` 是非同步事件：它觸發時，當初那條 socket 可能早已不是「目前這條」。因此 `onclose` 裡**所有**動作（reset `isConnected`、`cleanup()` 清計時器、`scheduleReconnect()`）都必須先比對 socket 實例身分，不能用單一個跨世代共用的布林 flag 來判斷「這次關閉是不是刻意的」。

反例：用一個共用的 `intentionalCloseRef` 布林。token 快速變更（`tok1 → tok2`，兩者皆 truthy）時，順序是「effect cleanup 設 flag=true 並關掉舊 socket → effect body 設 flag=false 並 `connect()` 建新 socket → 稍後舊 socket 的 `onclose` 才非同步觸發」。舊 socket 的 `onclose` 讀到的 flag 已被新世代覆寫成 false，於是它誤判成非預期斷線：不只多開一條重連，還會用共用的 `cleanup()` 清掉**新** socket 剛裝好的 heartbeat、把 live 連線的 `isConnected` 壓回 false。

正解是用 `wsRef.current` 與閉包捕獲的 `ws` 做三向判斷（每個「刻意關閉」路徑都已把 `wsRef.current` 設成 null 或換成新 socket，所以實例身分足以分辨世代）：

```tsx
ws.onclose = () => {
  // wsRef.current === ws    → 這條 live socket 真的掉了 → reset + 重連
  // wsRef.current === null  → 刻意拆除且無後繼（logout / unmount）→ reset，不重連
  // wsRef.current === 其他   → 已被新世代取代（token 變更）→ 新 socket 自己管狀態，這裡什麼都別做
  if (wsRef.current !== null && wsRef.current !== ws) return
  setIsConnected(false)
  cleanup()
  if (wsRef.current === ws) scheduleReconnect()
}
```

注意 `setIsConnected` 只能放在非同步的 `onclose` 裡，不能搬到 `disconnect()`：`disconnect()` 會被 effect 同步呼叫，`react-hooks` 的 `set-state-in-effect` 規則會擋（同步在 effect 內 setState 觸發 cascading render）。真正的網路斷線（`wsRef.current === ws`）仍照常走 backoff 重連。

---

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

### Sentinel Errors

- `model.ErrPathNotAllowed` — 路徑不在允許前綴內，或包含非法組件
- `model.ErrPathNotExist` — 路徑不存在於檔案系統

---

## 檔案與目錄規則

- 每個 Go 檔案不超過 300 行。超過時拆分
- 每個 function 不超過 50 行。超過時提取子函式
- Import 分三組，空行分隔：標準庫 → 第三方 → 專案內部

```go
import (
    "context"
    "fmt"

    "github.com/gin-gonic/gin"
    "github.com/jackc/pgx/v5"

    "github.com/steven/vaultflix/internal/model"
    "github.com/steven/vaultflix/internal/service"
)
```

---

## 測試規範

- 測試檔案與被測檔案同目錄：`video_service.go` → `video_service_test.go`
- Table-driven tests 為主
- Mock struct 手寫，放在 `internal/mock/` 目錄，不引入第三方 mock 框架
- Repository 與外部服務（MinIO）透過 interface mock，不連真實 DB 或外部服務
- 命名：`Test<Function>_<Scenario>`，如 `TestGetVideoByID_NotFound`
- 每個端點或 service method 實作完成後立即補測試，不事後批次補寫
- 測試必須覆蓋：正常路徑、資源不存在（404）、權限不足（403）、參數驗證失敗（400）
- 每組測試寫完後執行 `go test` 確認通過，失敗時先修正再繼續

---

## Design by Contract

- 所有跨層依賴透過 interface 定義契約，不直接依賴 concrete type
- Interface 定義在使用端的 package 中（例如 service 依賴的 repository interface 定義在 repository package）
- Service struct 的欄位型別是 interface，不是 concrete struct
- 共用的 sentinel errors 定義在 `internal/model/errors.go`：`ErrNotFound`、`ErrAlreadyExists`、`ErrConflict`
- 每個 interface method 的 godoc 須標註錯誤回傳語意（找不到回 ErrNotFound，不允許回 nil error + nil result 的模糊狀態）
- 新的 service 或 repository 一律先定義 interface 再寫實作

---

## CI/CD 與單一入口

所有 build / test / deploy 透過 `Taskfile.yml` 的單一入口執行。agent 本機、開發者本機、CI 呼叫**同一個 target**，不存在「CI 那邊做法不一樣」。

**前置工具（host 需安裝）**：

- `go-task`（`task` 指令）：build/test/deploy 單一入口。Windows `winget install Task.Task` 或 `scoop install task`；Linux/WSL `sh -c "$(curl -ssL https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin`。確認 `task` 在 PATH 上（winget 會把 shim 放到 `%LOCALAPPDATA%\Microsoft\WinGet\Links`）。
- `task verify` 還需要 **Go 1.25+**（`go vet`/`gofmt`/`go test`）與 **Node.js 20+**（`tsc`/`vitest`）原生安裝；整合測試需要 Docker。
- `gh`（GitHub CLI）：push 分支與開 PR 用，先 `gh auth login`。Windows `winget install GitHub.cli`；Linux/WSL 見 [cli.github.com](https://cli.github.com)。

> Linux/WSL 安裝到 `~/.local/bin`、`~/.local/go/bin` 的工具記得確認在 PATH 上。

### 入口指令清單

| 指令 | 用途 | 跑在哪 |
|---|---|---|
| `task verify` | 快層 gate（= `test-fast`），Stop hook 自動跑 | 原生 |
| `task test-fast` | `go vet` + `gofmt` 檢查 + `go test ./...` + web `tsc`(typecheck) + `eslint` + `vitest` | 原生 |
| `task lint` | `go vet` + 前端 `eslint`（手動 lint-only 便捷指令；eslint 也已含在 `test-fast` gate 內） | 原生 |
| `task test-integration` | 乾淨全棧 + fixture 跑 `scripts/test_all.sh`（up -d api → run --rm test-runner） | Docker |
| `task test-full` | `test-fast` + `test-integration` | Docker |
| `task build` / `task build:api` | build SHA-tagged image | Docker |
| `task push:api` | 推 API image 到 GHCR | Docker |
| `task deploy` | 本機部署（prod compose，build 不可變 nginx image） | Docker |
| `task up` / `task down` / `task logs` | 起/停/看 dev stack（自動疊 `docker-compose.media.yml`） | Docker |
| `task reset-admin-password` | 把 admin 密碼重設為 `.env` 的 `ADMIN_DEFAULT_PASSWORD` | Docker |

### 各場景 done-condition

對接「對話場景紀律」表，三種場景的 done 條件一致：

- **Bug Fix / Feature / Refactor done** = `task verify` 綠 + 相關範圍的 `task test-integration`（或 `task test-full`）綠 + PR 的 CI 綠。
- 純前端改動：至少 `task test-fast`（含 vitest）綠。
- 改到 import / 影片掃描 / 串流：要跑 `task test-integration`。
- Stop hook 會在收工前強制 `task verify`；別繞過它，紅燈就修到綠。

### lint 現況（重要）

前端既有 ESLint 債（`react-hooks@7` 的 react-compiler 規則，11 個 error）已清完。`npm run lint` 現已納入 `task test-fast`，因此 `task verify` 與 Stop hook 都會跑前端 eslint（zero errors / zero warnings），CI 也透過 `verify` job blocking。原本 CI 那個 non-blocking 的獨立 `lint` job 已移除（與 `verify` 重複）。

往後前端 lint 紅燈一律當 gate 處理，比照 typecheck / vitest，紅就修到綠，不再累積債。

### 不可變產物與部署

- Go API build 成 SHA-tagged image，CI 推到 `ghcr.io/steven715/vaultflix-api`。同一個產物 promote，不為不同環境 rebuild。
- 部署是手動 gate：本機 `task deploy`。`local`/`test`/整合測試一律自動，無需介入。
- prod / test 用 `docker-compose.prod.yml` / `docker-compose.test.yml` 以 `!override`/`!reset` 覆寫 base，不複製 infra 定義（避免 drift）。需 Docker Compose v2.24+。

---

## Git Commit 規範

```
<type>: <description>

type 可選值：
  feat     新功能
  fix      修 bug
  refactor 重構（不改變行為）
  docs     文件
  chore    建置、設定、依賴
  test     測試
```

範例：
- `feat: add video import service with ffprobe metadata extraction`
- `chore: setup docker-compose with postgres and minio`
- `fix: handle nil pointer in watch history update`

---

## 禁止事項

- ❌ 不使用 `init()` function（除了 driver registration 等不可避免的場景）
- ❌ 不使用全域可變狀態（global mutable state）
- ❌ 不使用 `panic` 做流程控制（只用於真正不可恢復的程式錯誤）
- ❌ 不引入未在計畫文件中列出的第三方依賴（需先討論）
- ❌ 不在 handler 層直接寫 SQL
- ❌ 不在 service / repository 層操作 `*gin.Context`
- ❌ 不在程式碼中寫死密碼、API key、secret

---

## 對話場景紀律

每個對話聚焦一個場景，不混用：

| 場景 | 目的 | 入口 |
|------|------|------|
| **Bug Fix** | 重現 → 定位 → 修復 → 驗證 → 提煉規範到 CLAUDE.md | 使用者回報 bug 或瀏覽器測試發現問題 |
| **Feature** | 需求 → 設計（Spec）→ 計畫（Plan）→ 實作 → 驗證 | 使用者提出新功能 |
| **Refactor** | 現狀分析 → 方案 → 實作 → 驗證 | 使用者要求重構或 code review 指出結構問題 |

- 對話開始時確認場景類型，全程在該場景內工作
- 如果使用者在對話中途切換場景（例如修 bug 途中開始做新功能），**主動提醒**：「這看起來是另一個場景，建議開新對話處理，這樣 context 更乾淨、review 也更精準」
- 三種場景的後段流程共用：驗證 → Code Review → PR → Merge
- 完整的開發流程步驟見 `/dev-workflow` skill

---

## Claude Code 工作指引

- 每建立或修改一個檔案後，簡短說明做了什麼以及為什麼
- 遇到計畫文件中不明確的地方，先用你的判斷做決定，完成後統一列出所有假設
- 嚴格遵守分層架構，不跨層呼叫
- 每完成一個 Phase，列出驗收清單的通過狀態
- 如果某個步驟需要做架構決策（例如選 Gin 還是 Echo），說明你的選擇理由
- 當我提出架構修改或設計要求時，如果你認為原本的設計更合理、我的修改在此專案脈絡下屬於過度設計、或存在我可能沒考慮到的副作用（如安全風險、維護成本），請直接說出來並給出理由，不要無條件照做
- 每次架構決策除了說明選擇理由，也要列出該決策的潛在缺點或 trade-off（例如：增加了複雜度、多了安全考量面、對目前專案規模是否過度設計）
