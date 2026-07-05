# 播放遙測端點 (Playback Telemetry) — 設計

- 狀態：Approved（brainstorming 定案，待寫 implementation plan）
- 日期：2026-07-06
- 場景：Feature
- 相關：ADR-0006（HUD stall 分類）、影片相容性 Phase 1（play_mode）、migration 015（watch_sessions）

## 目標

把播放器現有的 client 端量測從「即時顯示、關掉就沒」變成「可留存、可聚合」，
建立串流效能的 before/after baseline。要能回答例如：

- 外網 remux 片首播延遲 (time-to-first-frame) P50 = 幾秒？
- rebuffer ratio = 幾 %？
- 各 play_mode（direct / remux / transcode）差多少？

這是後續所有串流效能優化（ABR 等）的前提 —— 沒有量化就別談優化。本身很小、獨立有價值、零新基礎設施。

## 現況核實（設計前已核實的事實）

1. **量測 hook 目前沒掛進 PlayerPage。** `web/src/components/NetworkHud.tsx`、
   `web/src/hooks/usePlaybackStats.ts`、`web/src/utils/playbackStats.ts` 邏輯完整，但
   grep 全 `web/src` 沒有任何地方 import／render → 量測邏輯寫好了但**沒在跑**。要送遙測必須先掛上。
2. **既有 hook 只量即時快照，缺兩個 session 級指標。** `usePlaybackStats` 每 500ms setState 一次
   即時值（buffer headroom、throughput EWMA、TTFB、rebuffer **次數**、stall 分類），但**沒量**：
   - 首播延遲 (time-to-first-frame)
   - 累計 rebuffer 時長（只數次數，沒累加卡住的毫秒）→ 沒有它算不出 rebuffer ratio
3. **已存在 `watch_sessions` 表**（migration 015）：以 client `crypto.randomUUID()` 的 session_id
   為主鍵，heartbeat 持續 upsert 累計 `watched_seconds`。它與「遙測 session」天然同一個 session_id。

## 設計決策（brainstorming 定案）

| # | 問題 | 決定 | 理由 |
|---|------|------|------|
| 1 | Schema 形狀 | **獨立 `playback_telemetry` 表** | 遙測是「session 結束寫一次」的 immutable event；watch_sessions 是「持續 upsert 的執行中累計」。生命週期與寫入模式完全不同，混在同一列會讓 heartbeat 與遙測兩條寫入路徑互踩、語意混濁。 |
| 2 | 讀取端範圍 | **只落資料 + 一個極簡 admin 聚合端點** | 目標就是回答「P50 = 幾秒」；沒有讀取端 = 每次都要手寫 SQL，baseline 無法直接看。完整儀表板 UI 對單人自用 overkill。 |
| 3 | 網路情境來源 | **伺服器端由遠端 IP 判定** | client 自己不可靠地知道是不是外網；RFC1918 判定零客戶端配合。 |
| 4 | HUD 可見性 | **遙測常開，HUD overlay 藏在 toggle 後** | 量測 hook 必須掛才能送遙測；可見 overlay 只在 `?hud=1`／localStorage flag 時 render，預設 UX 不變，做 before/after 時可隨手打開目視驗證。 |
| 5 | 認證 scope | **viewer JWT（mirror heartbeat）** | 遙測是 JS 發的 POST（能帶 Authorization header），與 heartbeat 同性質；stream-token 是為裸 `<video src>` 媒體串流窄化的 scoped token，不適用。 |
| a | `fatal_error_family` 欄 | **保留** | 幾乎零成本，多一個「這場有沒有以致命錯誤收場」的失敗維度。 |
| b | 聚合分組粒度 | **按 `play_mode` 分組 + `scope` 當 filter 參數** | 第一版夠用；`?scope=external` 後看 remux 列即回答「外網 remux 首播 P50」。不做 `play_mode × scope` 交叉。 |
| c | Proxy IP 風險 | **先接受退化成 lan，留 TODO** | 這期不處理 trusted proxies；若 `c.ClientIP()` 穿不透 nginx，`network_scope` 會恆為內網 IP → 全 lan，屆時再修。 |

## 資料流

```
usePlaybackStats（擴充 session 累加器）
    │ 播放全程用既有 waiting/playing/canplay listener + buffer 數學，
    │ 額外累加 TTFF、總 stall ms、watched ms、throughput 樣本（不另開量測系統）
    ▼
PlayerPage 於 session 結束（unmount / pagehide beacon）
    │ getSessionSummary() → payload + { session_id, video_id, play_mode }
    ▼
POST /api/playback/telemetry   （viewer JWT，mirror heartbeat）
    │ 後端由 c.ClientIP() 判定 network_scope（lan/external/unknown）
    ▼
playback_telemetry 表  （session_id UNIQUE → upsert 冪等）
    ▲
GET /api/admin/playback/telemetry?days=N&scope=  （admin / Casbin）
    └ percentile_cont 算 P50/P95，按 play_mode 分組回 baseline
```

## Section 1 — 資料庫（migration 017）

一 session 一列，session 結束時寫一次。`session_id` 與 `watch_sessions` 同一個 client UUID 可關聯，但**不設 FK**（短 session 可能連第一次 heartbeat 都還沒發，watch_sessions 列不存在，但仍想留下 TTFF / 立即放棄的遙測）。

```sql
-- 017_create_playback_telemetry.up.sql
CREATE TABLE playback_telemetry (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    video_id           UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    session_id         UUID NOT NULL,
    play_mode          TEXT NOT NULL,             -- direct | remux | transcode
    network_scope      TEXT NOT NULL,             -- lan | external | unknown（伺服器判定）
    ttff_ms            INT,                        -- 首播延遲；null = 從未播出第一幀
    watched_ms         INT  NOT NULL DEFAULT 0,   -- 實際播放時長（rebuffer ratio 分母）
    rebuffer_count     INT  NOT NULL DEFAULT 0,
    rebuffer_ms        INT  NOT NULL DEFAULT 0,   -- 累計卡頓時長（排除初次緩衝與 seek）
    avg_downlink_mbps  NUMERIC(8,2),              -- null = 無樣本
    fatal_error_family TEXT,                       -- null | starved | codec
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_playback_telemetry_session   ON playback_telemetry(session_id);
CREATE INDEX        idx_playback_telemetry_created   ON playback_telemetry(created_at DESC);
CREATE INDEX        idx_playback_telemetry_play_mode ON playback_telemetry(play_mode, created_at DESC);
```

`down.sql`：`DROP TABLE IF EXISTS playback_telemetry;`（完整可逆）。

設計要點：

- **存原始分量**（`watched_ms` + `rebuffer_ms`）而非預先算好的 ratio → 聚合時
  `SUM(rebuffer_ms)/NULLIF(SUM(watched_ms+rebuffer_ms),0)`，避免除零與精度落地。
- `session_id UNIQUE` → beacon + pagehide 雙送時 `ON CONFLICT (session_id) DO UPDATE`（last write wins）冪等。
- 欄位驗證在 service 層（sentinel error），不用 DB CHECK constraint，比照專案既有慣例。

## Section 2 — 後端分層

嚴格 Handler → Service → Repository，interface 定在 repository package（同 `WatchSessionRepository`/`AnalyticsRepository` 慣例）。

**model**（`internal/model/`）
- `PlaybackTelemetryInput`：ingest 用，含 `UserID, VideoID, SessionID, PlayMode string`、量測數值、`RemoteIP string`（供 service 判定 scope）。
- `TelemetryQuery`（`Days int, Scope string`）、`TelemetrySummary`（`RangeDays int, Scope string, ByPlayMode []PlayModeStats`）、`PlayModeStats`（`PlayMode string, Sessions int64, TTFFP50Ms, TTFFP95Ms, RebufferRatio, AvgMbps *float64`——nullable 用 pointer）。

**repository**（`internal/repository/playback_telemetry_repo.go`）
- `PlaybackTelemetryRepository` interface：
  - `Insert(ctx, in model.PlaybackTelemetryInput) error` — upsert；FK 違反（pg code 23503）回 `model.ErrNotFound`；godoc 標註錯誤語意。
  - `Aggregate(ctx, q model.TelemetryQuery) ([]model.PlayModeStats, error)`。
- const query 置檔頂、parameterized（`$1` days、`$2` scope）。

**service**（`internal/service/playback_telemetry_service.go`）
- `PlaybackTelemetryService{ repo repository.PlaybackTelemetryRepository }`
- `Record(ctx, in) error`：驗證 `PlayMode ∈ {direct,remux,transcode}` 與 ids 非空（否則 `ErrInvalidInput`）、clamp 負值、`in.NetworkScope = classifyNetworkScope(in.RemoteIP)`、`repo.Insert`。error 用 `%w` wrap。
- `Summary(ctx, q) (*model.TelemetrySummary, error)`：mirror `AnalyticsService.Summary` 形狀，clamp days/limit。
- `classifyNetworkScope(ip string) string`（純函式，同檔）：loopback（127/8、`::1`）、RFC1918（10/8、172.16–31、192.168/16）、ULA（`fc00::/7`）、link-local → `lan`；可解析的公開位址 → `external`；空／無法解析 → `unknown`。table-driven 測試。

**handler**（`internal/handler/playback_telemetry_handler.go`）
- `Record(c)`：`user_id` 取自 JWT context、`RemoteIP = c.ClientIP()`、bind 其餘 → `service.Record`；成功 `201`／`204`，`ErrInvalidInput`→400、`ErrNotFound`→404、其餘→500 + `slog.Error`。
- `Summary(c)`：clamp `days`/`scope` query → `service.Summary` → `SuccessResponse{Data}`（200）。比照 `analytics_handler` 的 `clampParam`。

**route**（`cmd/server/main.go`，protected `api` group 內）
- `api.POST("/playback/telemetry", playbackTelemetryHandler.Record)`
- `api.GET("/admin/playback/telemetry", playbackTelemetryHandler.Summary)`
- 確認 Casbin admin policy 涵蓋新 `/admin/*` 路徑（同 `/admin/analytics` 既有做法）。
- main.go 依序 new repo → service → handler，注入 pool。

**聚合查詢**（回答「外網 remux 片首播 P50」＝ `?scope=external` 後看 remux 列）：
```sql
SELECT play_mode,
       COUNT(*)                                              AS sessions,
       percentile_cont(0.5)  WITHIN GROUP (ORDER BY ttff_ms) AS ttff_p50_ms,
       percentile_cont(0.95) WITHIN GROUP (ORDER BY ttff_ms) AS ttff_p95_ms,
       SUM(rebuffer_ms)::float / NULLIF(SUM(watched_ms + rebuffer_ms), 0) AS rebuffer_ratio,
       AVG(avg_downlink_mbps)                                AS avg_mbps
FROM playback_telemetry
WHERE created_at >= NOW() - ($1 || ' days')::interval
  AND ($2 = '' OR network_scope = $2)
GROUP BY play_mode;
```

## Section 3 — 前端

**擴充 `usePlaybackStats.ts`**（復用既有 listener，不新增第二套量測 —— 回答「HUD state 怎麼餵遙測避免重複計算」）
- 在既有 ref 累加器旁加：
  - `ttffRef`：首次 `playing` 且 `currentTime>0` 時 = `now − loadStart`，只記一次。
  - `stallStartRef` / `rebufferMsRef`：`waiting`（genuine：`currentTime>0 && !seeking`，同既有 `rebufferRef` 計數條件）記起點，`playing`/`canplay` 累加時長。
  - `watchedMsRef`：publish tick 中 `phase==='playing'` 累加牆鐘。
  - throughput running avg（sum/count）供落地代表值。
  - `fatalFamilyRef`：classifyPhase 落 codec/network fatal 時記 family。
- 回傳改為 `{ stats, getSessionSummary }`；`getSessionSummary()` 是穩定的 ref 讀取函式，回
  `{ ttffMs, watchedMs, rebufferCount, rebufferMs, avgDownlinkMbps, fatalErrorFamily }`。
- 純累加邏輯抽成 DOM-free 函式放 `playbackStats.ts` 以便 vitest（如 stall 時長 reducer）。

**`PlayerPage.tsx` 接線**
- 掛 hook：`avgBitrateBps = file_size_bytes*8/duration_seconds`；`streamPath` 依 play_mode 取
  direct 的 `stream_url` path 或 `/api/videos/:id/hls`。
- `{hudVisible && <NetworkHud stats={stats} />}` 置於既有 relative video 容器內；`hudVisible` 讀
  `?hud=1`（`useSearchParams`）或 `localStorage('vaultflix-hud')`。
- 在既有 unmount cleanup（`flushHeartbeat(true)` 旁）呼叫 `sendTelemetry()`：beacon keepalive、
  `sentRef` 防重、只在 `ttffMs != null || watchedMs > 0` 才送（純開啟沒播不送）。session_id 沿用
  `sessionIdRef`（已 per video open 重置）；累加器由 hook 內 `streamPath`-keyed effect 重置。
- 遵守 CLAUDE.md：ref 計數不用 state、cleanup 不 setState、beacon 用於 page-leave。

**`api/telemetry.ts`**
- `postPlaybackTelemetry(payload)` + beacon 變體（mirror `watchSession.ts` heartbeat 在 PlayerPage 的 beacon 寫法）。
- payload：`{ session_id, video_id, play_mode, ttff_ms, watched_ms, rebuffer_count, rebuffer_ms, avg_downlink_mbps, fatal_error_family }`。

## Section 4 — 測試 / 驗收

**Go table-driven**（mock 手寫進 `internal/mock/`，不連真 DB）
- `service.Record`：正常 upsert；`play_mode` 非法 → `ErrInvalidInput`（400）；負值 clamp；FK 缺 video → `ErrNotFound`（404）。
- `classifyNetworkScope`：loopback、10./172.16–31./192.168.、`fc00::`、link-local → lan；公開 IPv4/IPv6 → external；空／亂碼 → unknown。
- `service.Summary`：聚合形狀（含空結果）。
- `handler`：正常（201/204、200）、400、500。

**vitest**
- 擴充的純累加函式（TTFF 計算、stall 時長 reducer）；既有 `playbackStats` 測試不回歸。

**驗收**
- `task verify` 綠（go vet / gofmt / go test / tsc / eslint / vitest）。
- 因新增 migration + 觸及串流路徑 → 跑 `task test-integration`（POST 落庫、GET 聚合）綠。
- PR CI 綠。

## 已知風險 / TODO

- **Proxy IP（決策 c）**：走 Docker/nginx 時 `c.ClientIP()` 需 gin trusted proxies 設定 + nginx 傳
  `X-Forwarded-For` 才讀得到真實來源；本期不處理，`network_scope` 可能退化成恆 `lan`（nginx 內網 IP）。
  留 TODO，之後再設 trusted proxies。這是 CLAUDE.md「URL 消費者意識」的類比——判定依據要對應真正的來源。

## 非目標（YAGNI）

- 前端聚合儀表板 UI（決策 2）。
- `play_mode × network_scope` 交叉聚合（決策 b）。
- 逐 stall 事件的細粒度時間序列（只落 session summary）。
- ABR / 真轉碼（本 feature 只是其前置量化 enabler）。
