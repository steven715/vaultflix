# 自動標籤 Phase 1 — 番號/檔名 Scraper 設計

- **狀態**：已核可，待寫 implementation plan
- **日期**：2026-06-29
- **場景**：Feature（需求 → Spec → Plan）
- **分支**：`worktree-feat+auto-tagging-phase1`（worktree，base = `main`）
- **範圍**：ROADMAP 最高優先「自動標籤（AI 影片分析）」的 **Phase 1**。Phase 2（LLM）、Phase 3（CV）不在此 spec。

---

## 1. 目標與非目標

### 目標

從本機影片檔名抽出番號（如 `DASD-626`），依番號去可直抓的 JAV metadata 站抓資料，**以「建議」形式進 staging**，由使用者在 UI 接受/拒絕後才落到正式欄位。純本地運算 + HTML scrape，~$0、免 GPU。

### 非目標（Phase 1 明確排除）

- LLM 推論（Phase 2）、電腦視覺（Phase 3）
- sidecar 檔（`.nfo`/JSON）匯出 — 媒體碟 `:ro` 唯讀掛載，且 DB 即唯一真相
- FlareSolverr / headless browser 自動過 Cloudflare（僅保留 per-source cookie 注入鉤子）
- JavDB 自動過 Cloudflare（需要時由使用者貼 `cf_clearance` cookie）
- actress 頭像獨立瀏覽頁（Phase 1 僅存資料，不做專屬 UI）

---

## 2. 核心設計決策（已與使用者確認）

| 決策 | 選定 | 理由 |
|---|---|---|
| Enrich 時機 | **解耦成獨立 enrich pass** | 匯入維持純本地、快；scrape 慢/被 rate-limit/撞 Cloudflare，不可耦合進掃描迴圈 |
| 資料模型 | **Hybrid**：scalar 進 videos 欄位、genre→tags、actress→獨立表 | actress 有日文名/羅馬名/頭像，塞不進 `tags(name, category)`；genre 簡單，沿用 tags |
| 套用方式 | **Staging 建議表 + accept/reject** | 符合「以建議呈現讓我接受/拒絕」；失敗不污染既有資料 |
| Sidecar | **不做，DB 為唯一真相** | `:ro` 掛載無法寫媒體碟旁；單人系統 DB+MinIO 已是真相來源 |
| HTML 解析 | **引入 `github.com/PuerkitoBio/goquery`** | 業界標準、依賴 `x/net/html`、selector 易維護。唯一新增第三方 scrape 依賴 |
| Cloudflare | **先做可直抓來源 + per-source cookie/UA 覆寫** | Phase 1 優先 JavBus/JavLibrary；JavDB 留 cookie 注入鉤子，不引額外基礎設施 |

### 替使用者做的判斷（可推翻）

1. **匯入時就跑 avid 抽番號**（純 regex、本地、零成本），順便 seed `pending`/`no_code` 佇列，讓 enrichment worker 有東西撈，但 import 不碰網路。
2. **actress 頭像 keyed by actress**（跨片共用一張），非每片一份。
3. **accept 為整包套用 + 可選欄位覆寫**，不做 per-field 逐欄 accept（Phase 1 過度設計）。

---

## 3. 架構總覽

```
Import（本地, regex only, 不碰網路）
  └─ avid.ExtractCode(filename) → Video{ code, enrichment_status: pending | no_code }

EnrichmentService（per-video 觸發 / 批次 job；沿用 backfill async job 模式）
  ├─ 確認 code（必要時重抽）
  ├─ 依優先序逐 source：httpClient GET → goquery 解析 → EnrichedMetadata
  ├─ ScraperAggregator 依 per-field 優先序合併多來源
  ├─ 下載 cover + actress 頭像 → scratch temp → MinIO
  ├─ 寫 metadata_suggestions（raw JSONB, 一 source 一列）→ enrichment_status: suggested
  └─ WebSocket 通知 enrich_progress / enrich_complete / enrich_error

使用者在 UI 審核
  ├─ accept（可選 source / 編輯欄位）→ 套用 scalar 欄位 + genre→tags
  │    + actress→獨立表 + cover_key → enrichment_status: enriched, enriched_at
  └─ reject → 刪除/標記 suggestion；status 回 failed / none（可重試）
```

分層嚴守 Handler → Service → Repository，跨層全用 interface（先定義再實作）。

---

## 4. 番號抽取（avid，純函式 package）

新 package `internal/scraper/avid`（零外部依賴）。移植 JavSP `avid.py` 正則階梯：

- **清洗** `Clean(filename string) string`：去解析度（`1080p`/`720p`/`4K`）、網站浮水印（`hhd800.com@`、`[www.xxx]`、`xxx.com@`）、`-C`/`-c`（中字後綴）、`cdN`/`partN`（分片）、首尾雜訊與括號標記。
- **抽取** `ExtractCode(filename string) (code string, ok bool)`，依序嘗試：
  1. **FC2**：`FC2-PPV-1234567` / `FC2-1234567` → 正規化為 `FC2-PPV-1234567`
  2. **素人數字前綴**：`259LUXU-1234`、`200GANA-1234`（label 以數字開頭）
  3. **標準**：`[A-Z]{2,10}-\d{2,5}`（如 `DASD-626`、`SSIS-001`）
- 回傳 `ok=false` 時呼叫端設 `enrichment_status=no_code`。
- **測試重心**：table-driven，涵蓋上述每階梯的正例與雜訊干擾案例（含 JavSP 已知邊界）。

> **Phase 1 階梯範圍**僅上述三類。其餘（heyzo、carib、一本道等無連字號特例）列為 Phase 1.x backlog，不在本次。

---

## 5. 資料模型

### Migration `013_add_metadata_enrichment`（up / down 皆完整可逆）

**`videos` 新增欄位**

| 欄位 | 型別 | 說明 |
|---|---|---|
| `code` | `VARCHAR(50)` | 番號，可為空 |
| `release_date` | `DATE` | 發行日 |
| `runtime_minutes` | `INT` | 片長（分鐘，來自 metadata，非 ffprobe） |
| `maker` | `VARCHAR(255)` | 片商 |
| `label` | `VARCHAR(255)` | 廠牌 |
| `series` | `VARCHAR(255)` | 系列 |
| `cover_key` | `VARCHAR(1000)` | MinIO 封面 object key |
| `enrichment_status` | `VARCHAR(20) DEFAULT 'none'` | 狀態機（見下） |
| `enriched_at` | `TIMESTAMPTZ` | 套用建議的時間 |

索引：`idx_videos_code (code)`、`idx_videos_enrichment_status (enrichment_status)`。

**`actresses`（新表）**

```sql
CREATE TABLE actresses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name_ja     VARCHAR(255) NOT NULL,
    name_romaji VARCHAR(255) DEFAULT '',
    avatar_key  VARCHAR(1000) DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name_ja)
);

CREATE TABLE video_actresses (
    video_id   UUID REFERENCES videos(id) ON DELETE CASCADE,
    actress_id UUID REFERENCES actresses(id) ON DELETE CASCADE,
    PRIMARY KEY (video_id, actress_id)
);
```

**`metadata_suggestions`（staging）**

```sql
CREATE TABLE metadata_suggestions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id   UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    source     VARCHAR(50) NOT NULL,   -- javbus / javlibrary / javdb
    code       VARCHAR(50) NOT NULL,
    payload    JSONB NOT NULL,         -- 整包 EnrichedMetadata
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status     VARCHAR(20) NOT NULL DEFAULT 'pending'
);
CREATE INDEX idx_metadata_suggestions_video ON metadata_suggestions(video_id);
```

**genre** → 沿用既有 `tags`（`category='genre'`），經 `video_tags` 關聯。

### `enrichment_status` 狀態機

```
none ──(import 抽到番號)──▶ pending ──(scrape 成功)──▶ suggested ──(accept)──▶ enriched
  │                            │
  │                            ├──(抽不到番號)─────────▶ no_code   （可重試）
  └──(import 抽不到番號)────────┘
                               └──(全來源失敗/被擋)─────▶ failed    （可重試）
```

---

## 6. 分層與 Interface 契約

> 全部「先定義 interface 再寫實作」。interface 定義在使用端 package。每個 method 的 godoc 標註錯誤語意。

### model（`internal/model`）

- `EnrichedMetadata`：canonical 抓取結果（code, title, releaseDate, runtimeMinutes, maker, label, series, actresses[], genres[], coverURL）。
- `Actress`、`MetadataSuggestion`。
- 新 sentinel errors 進 `internal/model/errors.go`：`ErrCodeNotFound`、`ErrScrapeBlocked`、`ErrSourceUnavailable`。

### repository（`internal/repository`）

- **`VideoRepository` 擴充**：
  - `UpdateMetadata(ctx, id string, m model.VideoMetadataUpdate) error` — 套 scalar 欄位
  - `SetEnrichmentStatus(ctx, id, status string) error`
  - `ListByEnrichmentStatus(ctx, status string) ([]model.Video, error)`
- **`ActressRepository`（新）**：`Upsert(ctx, *model.Actress) error`（依 `name_ja`）、`AddVideoActress(ctx, videoID, actressID string) error`、`GetByVideoID(ctx, videoID string) ([]model.Actress, error)`
- **`SuggestionRepository`（新）**：`Create`、`GetByVideoID`、`GetByID`、`Delete`
- **`TagRepository` 擴充**：`GetOrCreateByName(ctx, name, category string) (*model.Tag, error)`

### service（`internal/service`）

- **`MetadataScraper` interface**：`ScrapeByCode(ctx context.Context, code string) (*model.EnrichedMetadata, error)`
  - 實作：`JavBusScraper`、`JavLibraryScraper`（Phase 1 直抓）；`JavDBScraper`（結構備好，走 cookie 注入，預設停用）
  - 錯誤語意：找不到番號頁 → `ErrCodeNotFound`；Cloudflare/JS challenge → `ErrScrapeBlocked`；連線失敗 → `ErrSourceUnavailable`
- **`ScraperAggregator`**：依**可設定 per-field 優先序**合併多來源結果（參考 Javinizer-go per-field source priority）。
- **`EnrichmentService`**（orchestrator）：依賴 `[]MetadataScraper`/aggregator、`MinIOClient`、`VideoRepository`、`ActressRepository`、`SuggestionRepository`、`TagRepository`、`websocket.Notifier`。提供：
  - `EnrichVideo(ctx, videoID string, opts EnrichOptions) error` — 單片
  - 批次 job（沿用 `backfill_service.go` 的狀態/取消/進度/單一執行中限制）
  - `AcceptSuggestion(ctx, videoID, suggestionID string, override SuggestionOverride) error` — 套用
  - `RejectSuggestion(ctx, videoID, suggestionID string) error`
- **`httpClient` wrapper**（`internal/service` 或 `internal/scraper`）：per-host rate limit（ticker）、指數退避重試、per-source cookie/UA 注入。

### handler（`internal/handler`）

`EnrichmentHandler`：

| Method + Path | 用途 | 成功碼 |
|---|---|---|
| `POST /api/videos/:id/enrich` | 觸發單片 enrich | 202 |
| `POST /api/enrich-jobs` | 批次（依 filter，如所有 `pending`） | 202 |
| `GET /api/enrich-jobs/active` | 查進行中的批次 job | 200 |
| `GET /api/videos/:id/suggestions` | 看該片建議 | 200 |
| `POST /api/videos/:id/suggestions/:sid/accept` | 套用（body 可選欄位覆寫/挑來源） | 200 |
| `DELETE /api/videos/:id/suggestions/:sid` | reject | 204 |

回應沿用 `SuccessResponse` / `ErrorResponse`；HTTP 碼遵循 CLAUDE.md（400/401/403/404/500）。

### WebSocket（`internal/websocket`）

新增 message type：`enrich_progress`、`enrich_complete`、`enrich_error`（對齊既有 import/backfill 慣例）。

---

## 7. MinIO 影像儲存

- object key：`covers/{videoID}.jpg`、`actresses/{actressID}.jpg`（頭像跨片共用）。
- 流程：scrape 取得圖片 URL → 下載到 scratch temp 檔 → 上傳 → 清 temp（對齊既有 `UploadThumbnail(objectKey, filePath)` 風格）。
- `MinIOClient` interface 新增 `UploadCover` / `UploadActressAvatar` + 對應 presign method。
- **Trade-off**：沿用 per-type 方法保持與現有命名一致，代價是 interface 變寬；替代是單一泛型 `UploadImage`，較簡潔但偏離現有慣例。**選前者（一致性優先）**。

---

## 8. Config（infra 走 env、業務走 request）

| 類別 | 參數 | 位置 |
|---|---|---|
| infra（需重啟） | `ENRICH_HTTP_TIMEOUT`、`ENRICH_USER_AGENT` | env，啟動載入 |
| 業務（執行期可調） | source 優先序、per-host rate、per-source cookie（`cf_clearance`） | API request 參數，env 僅作 fallback 預設 |

符合 CLAUDE.md「改這值需要重啟服務嗎？」判準。

---

## 9. 錯誤處理 / Fallback

- 抽不到番號 → `enrichment_status=no_code`。
- 全來源 scrape 失敗 → `failed`。
- Cloudflare 擋 → `ErrScrapeBlocked`，UI 提示貼 `cf_clearance` cookie。
- **任何失敗都不阻擋匯入、不污染既有資料**，只進「待審/可重試」狀態。
- error 一律 `%w` wrap 帶 context；只在 handler 層 log + HTTP response；service/repository 只 wrap + return。
- slog 結構化欄位（`video_id`、`code`、`source`、`status`）。

---

## 10. 與既有匯入流程的整合點

[internal/service/import_service.go](../../../internal/service/import_service.go) `processOneFile`：在 `probeMetadata` 之後、`videoRepo.Create` 之前，呼叫 `avid.ExtractCode(filename)`：

- 抽到 → `video.Code = code`、`enrichment_status = pending`
- 抽不到 → `enrichment_status = no_code`

**僅此一處改動**碰 import；scrape 完全在獨立 enrichment pass，不進 import 迴圈。

---

## 11. 新依賴

僅 `github.com/PuerkitoBio/goquery`（傳遞依賴 `golang.org/x/net`）。其餘全標準庫。

---

## 12. 測試策略

- **avid 抽取**：table-driven（**重心**），每階梯正例 + 雜訊干擾。
- **各 scraper**：`testdata/*.html` 離線 fixture → 斷言解析出的 `EnrichedMetadata`，table-driven，**不連網路**。
- **ScraperAggregator**：per-field 優先序合併，table-driven。
- **EnrichmentService**：mock 全 interface，測 happy / `no_code` / `blocked` / partial（部分來源失敗）。
- **handler**：accept/reject 的 200/400/404、trigger 202。
- **httpClient**：rate limiter 單元測試。
- 因觸及 import/掃描，收尾跑 `task test-integration`（CLAUDE.md done-condition）。

---

## 13. 併行開發注意（與 admin 分支）

- 本工作在 worktree `feat+auto-tagging-phase1`（base `main`），admin 重設計在主 checkout 的 `feat/admin-redesign-shell`，互不干擾。
- **Migration 編號**：本 feature 佔用 `013`；admin 分支若需 migration 請改用 `014`，避免撞號使 golang-migrate 爆掉。
- **共用檔案**（`cmd/server/main.go` 路由 wiring、`config.go`）兩邊都可能改 → 先 merge 的後，另一條 rebase main 再解 conflict。

---

## 14. 驗收清單（Phase 1 完成定義）

- [ ] `avid.ExtractCode` 通過 table-driven 測試（三階梯 + 清洗）
- [ ] migration 013 up/down 可逆，`task` 套用無誤
- [ ] JavBus + JavLibrary scraper 以離線 fixture 通過解析測試
- [ ] EnrichmentService 單片 + 批次（含取消/進度/WS）可運作，mock 測試綠
- [ ] cover/頭像存入 MinIO 並可 presign
- [ ] enrich 端點 + suggestions accept/reject 端點完整，含 400/404 測試
- [ ] import 整合點：抽番號並 seed `pending`/`no_code`
- [ ] `task verify` 綠 + `task test-integration` 綠
