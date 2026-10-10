# CLAUDE.md — Vaultflix 開發規範

## 專案概述

Vaultflix 是一個 Go + React 的個人影片管理與串流平台。後端為 Go API Server，前端為 React SPA，影片保留在本機磁碟，系統直接讀取串流；MinIO 僅存縮圖與預覽，metadata 存於 PostgreSQL。

**場景前提（架構決策以此規模為準）**：Jellyfin 式個人媒體伺服器 —— 單一使用者為主、同時串流數個位數、區網優先、偶爾 ngrok 對外分享。不做 YouTube 式規模；串流走即時處理路線（保留原檔，播放時 remux/transcode）。詳見 `docs/adr/0009` 與 `docs/streaming.md`。

**領域用語以 [`CONTEXT.md`](CONTEXT.md) 為準。** 程式碼識別字、文件、commit、PR、測試名稱提到領域概念時用術語表的主詞條，不用 `_Avoid_` 列出的別名（程式碼中尚未改名的舊識別字如 `Actress` 例外，見 ROADMAP「程式碼對齊術語表」）。

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
- `409`：與資源當前狀態衝突（如重複執行中的作業、Media Source 已停用）
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

---

## 主題規範（動到該區域前必讀）

以下規則只在碰到特定區域時需要，不常駐於此檔。**觸發條件成立時先讀對應文件再動手。**

| 觸發條件 | 必讀 |
|---|---|
| 動 `web/src/`（React 元件、hooks、vitest 測試） | [`docs/conventions/frontend.md`](docs/conventions/frontend.md) —— useEffect、假時鐘測試、flaky 判斷、同路徑 refetch、具名導航 |
| 動 `internal/websocket/` 或 `web/src/hooks/useWebSocket.ts` | [`docs/conventions/websocket.md`](docs/conventions/websocket.md) —— Hub、訊息協議、重連、onclose 世代 |
| 動 `internal/streaming/`、Play Mode、任何呼叫 ffmpeg/ffprobe 的程式 | [`docs/streaming.md`](docs/streaming.md) —— 播放路徑、規模前提、§6 FFmpeg seek 與產物測試 |
| 動 `docker-compose*.yml`、`Dockerfile*`、`nginx/`、部署流程、影片掛載 | [`docs/conventions/docker.md`](docs/conventions/docker.md) —— 不可變 nginx image、磁碟掛載、compose 疊加陷阱 |
| 產生給瀏覽器的 URL、新增設定參數、處理使用者可控的檔案路徑 | [`docs/conventions/backend-design.md`](docs/conventions/backend-design.md) —— URL 消費者、執行期可調性、路徑安全 |
| 做出難以逆轉、有真實取捨的架構決策 | [`docs/adr/`](docs/adr/README.md) —— 先讀相關 ADR；新決策寫新 ADR |

修完 bug 後把教訓寫成**正向規則**，放進上表對應的主題文件（沒有合適的就新開一份並在此表加一列），不要直接堆進本檔。

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
- Service 層測試中，Repository 與外部服務（MinIO）透過 interface mock，不連真實 DB 或外部服務
- Repository 自己的 SQL 例外：以 `openTestPool` 對整合測試 stack 的真 Postgres 測試，未設 DB 時 skip，在 `task test-integration` 執行（ADR-0012）
- 命名：`Test<Function>_<Scenario>`，如 `TestGetVideoByID_NotFound`
- 每個端點或 service method 實作完成後立即補測試，不事後批次補寫
- 測試必須覆蓋：正常路徑、資源不存在（404）、權限不足（403）、參數驗證失敗（400）
- 每組測試寫完後執行 `go test` 確認通過，失敗時先修正再繼續

---

## Design by Contract

- 所有跨層依賴透過 interface 定義契約，不直接依賴 concrete type
- Interface 定義在使用端的 package 中（例如 service 依賴的 repository interface 定義在 repository package）
- Service struct 的欄位型別是 interface，不是 concrete struct
- 共用的 sentinel errors 定義在 `internal/model/errors.go`：`ErrNotFound`、`ErrAlreadyExists`、`ErrConflict`、`ErrMediaSourceDisabled`
- 每個 interface method 的 godoc 須標註錯誤回傳語意（找不到回 ErrNotFound，不允許回 nil error + nil result 的模糊狀態）
- 新的 service 或 repository 一律先定義 interface 再寫實作

---

## CI/CD 與單一入口

所有 build / test / deploy 透過 `Taskfile.yml` 的單一入口執行。agent 本機、開發者本機、CI 呼叫**同一個 target**，不存在「CI 那邊做法不一樣」。

前置工具（`task`、Go 1.25+、Node.js 20+、Docker、`gh`）的安裝方式見 README 的 Prerequisites。`task verify` 需要原生 Go 與 Node；整合測試需要 Docker。

### 入口指令清單

| 指令 | 用途 | 跑在哪 |
|---|---|---|
| `task verify` | 快層 gate（= `test-fast`），Stop hook 自動跑 | 原生 |
| `task test-fast` | `go vet` + `gofmt` 檢查 + `go test ./...` + web `tsc`(typecheck) + `eslint` + `vitest` | 原生 |
| `task lint` | `go vet` + 前端 `eslint`（手動 lint-only 便捷指令；eslint 也已含在 `test-fast` gate 內） | 原生 |
| `task test-integration` | 乾淨全棧 + fixture 跑 `scripts/test_all.sh`（up -d api → run --rm test-runner） | Docker |
| `task test-full` | `test-fast` + `test-integration` | Docker |
| `task up` / `task down` / `task logs` | build 並起/停/看唯一的執行 stack（改 code 後重跑 `task up`；自動疊 `docker-compose.media.yml`） | Docker |
| `task reset-admin-password` | 把 admin 密碼重設為 `.env` 的 `ADMIN_DEFAULT_PASSWORD` | Docker |

### 各場景 done-condition

- **Bug Fix / Feature / Refactor done** = `task verify` 綠 + 相關範圍的 `task test-integration`（或 `task test-full`）綠 + PR 的 CI 綠。
- 純前端改動：至少 `task test-fast`（含 vitest）綠。
- 改到 import / 影片掃描 / 串流：要跑 `task test-integration`。
- Stop hook 會在收工前強制 `task verify`；別繞過它，紅燈就修到綠。

### 執行 stack 與部署（ADR-0011）

- 只有一個執行 stack：`docker-compose.yml`（API 為編譯後的 binary，nginx 在前、X-Accel 開啟）。本機 `task up` 即部署，不推 registry。
- 整合測試用 `docker-compose.test.yml` 以 `!override` 覆寫 base（隔離的 compose project、拋棄式 volume、fixture 掛載），不複製 infra 定義。需 Docker Compose v2.24+。

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

## Chrome DevTools MCP（瀏覽器除錯）

repo 根目錄的 `.mcp.json` 註冊了 `chrome-devtools-mcp`（版本固定，升級時改 `.mcp.json`）。首次開啟專案時 Claude Code 會詢問是否啟用。

- 它會自行以獨立 profile 啟動 Chrome，不影響日常瀏覽器；不需要手動開 debug port 或 hook
- 前置：Google Chrome 與 Node.js（`npx`）已安裝
- 用途：重現前端 bug、驗證 UI 改動（截圖、DOM、Network、Console、performance trace、網路節流）
- 串流問題（首播延遲、外網卡頓）優先用 Network 面板 + 節流重現，再對照 Playback Telemetry
- 限制：只有 Chromium。Safari 專屬問題（如原生 HLS）無法用它重現

---

## 開發流程

- 一個 PR 只做一個場景（Bug Fix / Feature / Refactor 擇一）。同一個對話可以處理多件事，但要拆成不同 branch / PR，不混在同一個 diff
- Feature 設計用 `/grill-with-docs`（術語寫進 `CONTEXT.md`、決策寫 ADR）；Bug 用 `/diagnosing-bugs`；實作優先 `/tdd`
- 發 PR 前跑 `/code-review`，即使改動看起來很簡單
- Done-condition 見上方「CI/CD 與單一入口」

---

## Claude Code 工作指引

- 每建立或修改一個檔案後，簡短說明做了什麼以及為什麼
- 遇到計畫文件中不明確的地方，先用你的判斷做決定，完成後統一列出所有假設
- 嚴格遵守分層架構，不跨層呼叫
- 如果某個步驟需要做架構決策（例如選 Gin 還是 Echo），說明你的選擇理由
- 當我提出架構修改或設計要求時，如果你認為原本的設計更合理、我的修改在此專案脈絡下屬於過度設計、或存在我可能沒考慮到的副作用（如安全風險、維護成本），請直接說出來並給出理由，不要無條件照做
- 每次架構決策除了說明選擇理由，也要列出該決策的潛在缺點或 trade-off（例如：增加了複雜度、多了安全考量面、對目前專案規模是否過度設計）

---

## Agent skills

Matt Pocock skills（`mattpocock-skills` plugin）的每 repo 設定。

### Issue tracker

Issue 與 spec 放在 GitHub Issues（`steven715/vaultflix`），透過 `gh` CLI 操作。See `docs/agents/issue-tracker.md`.

### Triage labels

使用預設五個角色：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。See `docs/agents/triage-labels.md`.

### Domain docs

Single-context：root 的 `CONTEXT.md`（術語表）+ `docs/adr/`。See `docs/agents/domain.md`.
