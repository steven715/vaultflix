# 自動標籤 Phase 1 — 番號/檔名 Scraper Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 從本機影片檔名抽番號，依番號從可直抓的 JAV metadata 站抓資料，以「建議」形式進 staging，由使用者 accept/reject 後落到正式欄位。

**Architecture:** 匯入維持純本地（只跑 regex 抽番號並 seed 佇列）；scrape 在解耦的 EnrichmentService（per-video + 批次 job，沿用 backfill 模式）裡進行。抓到的結果寫 `metadata_suggestions` staging 表，accept 時才套到 videos 欄位 / actresses 表 / genre tags。嚴守 Handler→Service→Repository 分層，跨層全用 interface。

**Tech Stack:** Go 1.22+、Gin、pgx/v5、PostgreSQL 16、MinIO、`github.com/PuerkitoBio/goquery`（新依賴）、`log/slog`、golang-migrate。

**Spec:** [docs/superpowers/specs/2026-06-29-auto-tagging-phase1-design.md](../specs/2026-06-29-auto-tagging-phase1-design.md)

## Global Constraints

- Go 1.22+；新第三方依賴**僅** `github.com/PuerkitoBio/goquery`（其餘標準庫）。
- 嚴守分層 Handler→Service→Repository；跨層用 interface，先定 interface 再寫實作；interface 定義在使用端 package。
- error 一律 `fmt.Errorf("...: %w", err)` wrap 帶 context；只在 handler 層 log + HTTP response；service/repository 只 wrap+return。Error message 小寫開頭不加句號。
- `context.Context` 為每個跨層 method 第一參數。
- slog 結構化欄位（`video_id`、`code`、`source`、`status`），不拼字串。
- SQL：關鍵字大寫、table/column snake_case、parameterized query、query 寫成檔頂 const。
- Mock 手寫放 `internal/mock/`，不引第三方 mock 框架。測試命名 `Test<Function>_<Scenario>`，table-driven 為主。
- 業務參數（source 優先序、rate、cookie）走 request、env 僅 fallback；infra（timeout、UA）走 env。
- migration 命名 `013_add_metadata_enrichment.up.sql`/`.down.sql`，down 完整可逆。**本 feature 佔用編號 013。**
- 每個檔案 ≤300 行、每個 function ≤50 行；import 分三組。
- 每完成一個 task 跑該層測試；收尾 `task verify` + `task test-integration` 須綠。

---

## File Structure

**新建**
- `internal/scraper/avid/avid.go` — 番號清洗 + 抽取（純函式）
- `internal/scraper/avid/avid_test.go`
- `internal/scraper/httpclient.go` — rate-limit + retry + cookie/UA 注入的 HTTP wrapper
- `internal/scraper/httpclient_test.go`
- `internal/scraper/scraper.go` — `MetadataScraper` interface + 共用 helper
- `internal/scraper/javbus.go` / `javbus_test.go` / `testdata/javbus_*.html`
- `internal/scraper/javlibrary.go` / `javlibrary_test.go` / `testdata/javlibrary_*.html`
- `internal/scraper/javdb.go`（結構備好、預設停用，不在驗收測試）
- `internal/scraper/aggregator.go` / `aggregator_test.go` — per-field 優先序合併
- `internal/service/enrichment_service.go` / `enrichment_service_test.go`
- `internal/handler/enrichment_handler.go` / `enrichment_handler_test.go`
- `internal/repository/actress_repo.go`、`internal/repository/suggestion_repo.go`
- `internal/mock/scraper.go`、`internal/mock/actress_repo.go`、`internal/mock/suggestion_repo.go`（擴充既有 mock 檔）
- `migrations/013_add_metadata_enrichment.up.sql` / `.down.sql`

**修改**
- `internal/model/video.go`（新欄位）、`internal/model/errors.go`（sentinel）、新增 `internal/model/enrichment.go`（EnrichedMetadata / Actress / MetadataSuggestion / VideoMetadataUpdate / SuggestionOverride）
- `internal/repository/video_repo.go`（interface + impl 擴充）、`internal/repository/tag_repo.go`（`GetOrCreateByName`）
- `internal/service/minio_service.go`（cover/avatar upload + presign）
- `internal/websocket/message.go`（enrich message types）
- `internal/service/import_service.go`（整合點：抽番號 seed status）
- `cmd/server/main.go`、`internal/config/config.go`（wiring + config）
- `internal/mock/*`（既有 mock 補新 method）
- `go.mod` / `go.sum`（goquery）

---

## Task 1: avid 番號清洗 + 抽取（純函式）

**Files:**
- Create: `internal/scraper/avid/avid.go`
- Test: `internal/scraper/avid/avid_test.go`

**Interfaces:**
- Produces: `func Clean(filename string) string`、`func ExtractCode(filename string) (code string, ok bool)`

- [ ] **Step 1: Write the failing test**

```go
package avid

import "testing"

func TestExtractCode_Standard(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"plain", "DASD-626.mp4", "DASD-626", true},
		{"lowercase", "ssis-001.mkv", "SSIS-001", true},
		{"underscore", "MIDE_888.mp4", "MIDE-888", true},
		{"with resolution", "SSIS-123-1080p.mp4", "SSIS-123", true},
		{"website watermark", "hhd800.com@DASD-700.mp4", "DASD-700", true},
		{"chinese sub suffix", "STARS-256-C.mp4", "STARS-256", true},
		{"multi-disc", "ABP-999-cd2.mp4", "ABP-999", true},
		{"fc2 ppv", "FC2-PPV-1234567.mp4", "FC2-PPV-1234567", true},
		{"fc2 short", "FC2-1234567.mp4", "FC2-PPV-1234567", true},
		{"amateur numeric prefix luxu", "259LUXU-1234.mp4", "259LUXU-1234", true},
		{"amateur numeric prefix gana", "200gana-1888.mp4", "200GANA-1888", true},
		{"no code", "家庭聚會影片.mp4", "", false},
		{"random digits only", "20240101_backup.mp4", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ExtractCode(c.in)
			if ok != c.ok || got != c.want {
				t.Fatalf("ExtractCode(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestClean_StripsNoise(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hhd800.com@SSIS-001-1080p-C.mp4", "SSIS-001"},
		{"[www.example.com]ABP-123.mkv", "ABP-123"},
		{"MIDE-888-cd1.mp4", "MIDE-888"},
	}
	for _, c := range cases {
		if got := Clean(c.in); got != c.want {
			t.Errorf("Clean(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./internal/scraper/avid/ -run TestExtractCode -v`
Expected: FAIL (`undefined: ExtractCode`).

- [ ] **Step 3: Implement**

```go
package avid

import (
	"regexp"
	"strings"
)

var (
	// 去除網站浮水印前綴：domain@ 或 [domain] 或 (domain)
	reWatermark = regexp.MustCompile(`(?i)(^[a-z0-9.\-]+\.(?:com|net|org|cc|app|me|tv|xyz)@)|(\[[^\]]*\])|(\([^)]*\.(?:com|net|org|cc|app|me|tv|xyz)[^)]*\))`)
	// 去除解析度 / 畫質
	reResolution = regexp.MustCompile(`(?i)[-_ ]?(\d{3,4}p|2160p|4k|fhd|hd|\d{3,4}x\d{3,4})`)
	// 去除分片 cdN / partN
	reDisc = regexp.MustCompile(`(?i)[-_ ]?(cd\d+|part\d+|disc\d+)`)
	// 去除中字 / 無碼破解等後綴
	reSuffixTag = regexp.MustCompile(`(?i)[-_ ](c|ch|u|uc|uncensored|leak|hack)$`)

	// 抽取階梯（順序重要）
	reFC2      = regexp.MustCompile(`(?i)FC2[-_ ]*(?:PPV[-_ ]*)?(\d{5,7})`)
	reAmateur  = regexp.MustCompile(`(?i)(\d{3}[A-Z]{2,6})[-_ ]?(\d{2,5})`)
	reStandard = regexp.MustCompile(`(?i)([A-Z]{2,10})[-_ ]?(\d{2,5})`)
)

// Clean 去掉副檔名與常見雜訊（浮水印、解析度、分片、中字後綴），回傳清洗後字串。
func Clean(filename string) string {
	s := filename
	if i := strings.LastIndex(s, "."); i > 0 {
		s = s[:i]
	}
	s = reWatermark.ReplaceAllString(s, "")
	s = reResolution.ReplaceAllString(s, "")
	s = reDisc.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = reSuffixTag.ReplaceAllString(s, "")
	return strings.Trim(s, " -_")
}

// ExtractCode 從檔名抽出正規化番號。抽不到回 ("", false)。
// 階梯：FC2 → 素人數字前綴 → 標準 [A-Z]{2,10}-\d{2,5}。
func ExtractCode(filename string) (string, bool) {
	s := Clean(filename)
	if m := reFC2.FindStringSubmatch(s); m != nil {
		return "FC2-PPV-" + m[1], true
	}
	if m := reAmateur.FindStringSubmatch(s); m != nil {
		return strings.ToUpper(m[1]) + "-" + m[2], true
	}
	if m := reStandard.FindStringSubmatch(s); m != nil {
		return strings.ToUpper(m[1]) + "-" + m[2], true
	}
	return "", false
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/scraper/avid/ -v`
Expected: PASS. 若某 case 失敗，調整對應 regex（例如 `reStandard` 誤吞 `reAmateur` 的數字前綴 → 確認 amateur 在前）。

- [ ] **Step 5: Commit**

```bash
git add internal/scraper/avid/
git commit -m "feat: add avid番號 cleaning and extraction (pure functions)"
```

---

## Task 2: Migration 013 + model 型別 + sentinel errors

**Files:**
- Create: `migrations/013_add_metadata_enrichment.up.sql`, `migrations/013_add_metadata_enrichment.down.sql`, `internal/model/enrichment.go`
- Modify: `internal/model/video.go`, `internal/model/errors.go`

**Interfaces:**
- Produces: `model.Video` 新欄位；`model.EnrichedMetadata`、`model.ActressMeta`、`model.Actress`、`model.MetadataSuggestion`、`model.VideoMetadataUpdate`、`model.SuggestionOverride`；errors `model.ErrCodeNotFound`、`model.ErrScrapeBlocked`、`model.ErrSourceUnavailable`。

- [ ] **Step 1: Write migration up**

`migrations/013_add_metadata_enrichment.up.sql`:

```sql
ALTER TABLE videos ADD COLUMN code VARCHAR(50);
ALTER TABLE videos ADD COLUMN release_date DATE;
ALTER TABLE videos ADD COLUMN runtime_minutes INT;
ALTER TABLE videos ADD COLUMN maker VARCHAR(255);
ALTER TABLE videos ADD COLUMN label VARCHAR(255);
ALTER TABLE videos ADD COLUMN series VARCHAR(255);
ALTER TABLE videos ADD COLUMN cover_key VARCHAR(1000) DEFAULT '';
ALTER TABLE videos ADD COLUMN enrichment_status VARCHAR(20) NOT NULL DEFAULT 'none';
ALTER TABLE videos ADD COLUMN enriched_at TIMESTAMPTZ;

CREATE INDEX idx_videos_code ON videos(code);
CREATE INDEX idx_videos_enrichment_status ON videos(enrichment_status);

CREATE TABLE actresses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name_ja     VARCHAR(255) NOT NULL,
    name_romaji VARCHAR(255) NOT NULL DEFAULT '',
    avatar_key  VARCHAR(1000) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (name_ja)
);

CREATE TABLE video_actresses (
    video_id   UUID REFERENCES videos(id) ON DELETE CASCADE,
    actress_id UUID REFERENCES actresses(id) ON DELETE CASCADE,
    PRIMARY KEY (video_id, actress_id)
);

CREATE TABLE metadata_suggestions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    video_id   UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    source     VARCHAR(50) NOT NULL,
    code       VARCHAR(50) NOT NULL,
    payload    JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status     VARCHAR(20) NOT NULL DEFAULT 'pending'
);
CREATE INDEX idx_metadata_suggestions_video ON metadata_suggestions(video_id);
```

- [ ] **Step 2: Write migration down**

`migrations/013_add_metadata_enrichment.down.sql`:

```sql
DROP TABLE IF EXISTS metadata_suggestions;
DROP TABLE IF EXISTS video_actresses;
DROP TABLE IF EXISTS actresses;

DROP INDEX IF EXISTS idx_videos_enrichment_status;
DROP INDEX IF EXISTS idx_videos_code;

ALTER TABLE videos DROP COLUMN IF EXISTS enriched_at;
ALTER TABLE videos DROP COLUMN IF EXISTS enrichment_status;
ALTER TABLE videos DROP COLUMN IF EXISTS cover_key;
ALTER TABLE videos DROP COLUMN IF EXISTS series;
ALTER TABLE videos DROP COLUMN IF EXISTS label;
ALTER TABLE videos DROP COLUMN IF EXISTS maker;
ALTER TABLE videos DROP COLUMN IF EXISTS runtime_minutes;
ALTER TABLE videos DROP COLUMN IF EXISTS release_date;
ALTER TABLE videos DROP COLUMN IF EXISTS code;
```

- [ ] **Step 3: Add model types**

`internal/model/enrichment.go`:

```go
package model

import "time"

// EnrichedMetadata 是單一來源 scrape 出的 canonical 結果（也是 suggestion payload 的形狀）。
type EnrichedMetadata struct {
	Code           string        `json:"code"`
	Title          string        `json:"title"`
	ReleaseDate    *time.Time    `json:"release_date,omitempty"`
	RuntimeMinutes int           `json:"runtime_minutes"`
	Maker          string        `json:"maker"`
	Label          string        `json:"label"`
	Series         string        `json:"series"`
	Genres         []string      `json:"genres"`
	Actresses      []ActressMeta `json:"actresses"`
	CoverURL       string        `json:"cover_url"`
}

// ActressMeta 是 scrape 出的女優資料（尚未落表）。
type ActressMeta struct {
	NameJa     string `json:"name_ja"`
	NameRomaji string `json:"name_romaji"`
	AvatarURL  string `json:"avatar_url"`
}

// Actress 對應 actresses 表。
type Actress struct {
	ID         string    `json:"id"`
	NameJa     string    `json:"name_ja"`
	NameRomaji string    `json:"name_romaji"`
	AvatarKey  string    `json:"avatar_key"`
	CreatedAt  time.Time `json:"created_at"`
}

// MetadataSuggestion 對應 metadata_suggestions 表。
type MetadataSuggestion struct {
	ID        string           `json:"id"`
	VideoID   string           `json:"video_id"`
	Source    string           `json:"source"`
	Code      string           `json:"code"`
	Payload   EnrichedMetadata `json:"payload"`
	FetchedAt time.Time        `json:"fetched_at"`
	Status    string           `json:"status"`
}

// VideoMetadataUpdate 是 accept 後要套到 videos 的 scalar 欄位。
type VideoMetadataUpdate struct {
	Code           string
	Title          string
	ReleaseDate    *time.Time
	RuntimeMinutes int
	Maker          string
	Label          string
	Series         string
	CoverKey       string
}

// SuggestionOverride 是 accept 時使用者對欄位的覆寫（空欄沿用 suggestion 原值）。
type SuggestionOverride struct {
	Title  *string  `json:"title,omitempty"`
	Genres []string `json:"genres,omitempty"`
}

// Enrichment 狀態常數。
const (
	EnrichmentNone      = "none"
	EnrichmentPending   = "pending"
	EnrichmentSuggested = "suggested"
	EnrichmentEnriched  = "enriched"
	EnrichmentFailed    = "failed"
	EnrichmentNoCode    = "no_code"
)
```

- [ ] **Step 4: Extend Video model**

在 `internal/model/video.go` 的 `Video` struct 末尾（`FilePath` 後）加：

```go
	Code             string     `json:"code,omitempty"`
	ReleaseDate      *time.Time `json:"release_date,omitempty"`
	RuntimeMinutes   int        `json:"runtime_minutes,omitempty"`
	Maker            string     `json:"maker,omitempty"`
	Label            string     `json:"label,omitempty"`
	Series           string     `json:"series,omitempty"`
	CoverKey         string     `json:"cover_key,omitempty"`
	EnrichmentStatus string     `json:"enrichment_status"`
	EnrichedAt       *time.Time `json:"enriched_at,omitempty"`
```

- [ ] **Step 5: Add sentinel errors**

在 `internal/model/errors.go` 加：

```go
// ErrCodeNotFound 表示來源站找不到該番號的頁面。
var ErrCodeNotFound = errors.New("code not found at source")

// ErrScrapeBlocked 表示被 Cloudflare / JS challenge 擋下。
var ErrScrapeBlocked = errors.New("scrape blocked by challenge")

// ErrSourceUnavailable 表示來源站連線失敗 / 非預期狀態。
var ErrSourceUnavailable = errors.New("scrape source unavailable")
```

（若 `errors` 尚未 import 則補上。）

- [ ] **Step 6: Verify build + migration round-trip**

Run: `go build ./...` → exit 0。
Run: `task test-integration`（會套 migration；若僅想驗 migration，於乾淨 DB 跑 up 再跑 down 應無錯）。
Expected: build 綠；migration up/down 無誤。

- [ ] **Step 7: Commit**

```bash
git add migrations/013_* internal/model/
git commit -m "feat: add enrichment schema (migration 013) and model types"
```

---

## Task 3: Repository 層（Video 擴充 + Actress + Suggestion + Tag helper + mocks）

**Files:**
- Create: `internal/repository/actress_repo.go`, `internal/repository/suggestion_repo.go`, `internal/mock/actress_repo.go`, `internal/mock/suggestion_repo.go`
- Modify: `internal/repository/video_repo.go`, `internal/repository/tag_repo.go`, `internal/mock/video_repo.go`, `internal/mock/tag_repo.go`

**Interfaces:**
- Consumes: `model.*`（Task 2）。
- Produces:
  - `VideoRepository` 新增 `UpdateMetadata(ctx, id string, m model.VideoMetadataUpdate) error`、`SetEnrichmentStatus(ctx, id, status string) error`、`ListByEnrichmentStatus(ctx, status string) ([]model.Video, error)`
  - `ActressRepository`：`Upsert(ctx, *model.Actress) error`（依 name_ja，回填 ID）、`AddVideoActress(ctx, videoID, actressID string) error`、`GetByVideoID(ctx, videoID string) ([]model.Actress, error)`
  - `SuggestionRepository`：`Create(ctx, *model.MetadataSuggestion) error`、`GetByVideoID(ctx, videoID string) ([]model.MetadataSuggestion, error)`、`GetByID(ctx, id string) (*model.MetadataSuggestion, error)`、`Delete(ctx, id string) error`
  - `TagRepository` 新增 `GetOrCreateByName(ctx, name, category string) (*model.Tag, error)`

> 本 task 的 repo 實作以 build + integration test 驗證（CLAUDE.md：repo 不連真實 DB 做單元測試）。mock 在 Task 9/10/13 的 service/handler 單元測試使用。

- [ ] **Step 1: Define ActressRepository interface + impl**

`internal/repository/actress_repo.go`（query const 放檔頂；`Upsert` 用 `INSERT ... ON CONFLICT (name_ja) DO UPDATE SET name_romaji=EXCLUDED.name_romaji, avatar_key=EXCLUDED.avatar_key RETURNING id`；其餘照 existing repo 風格用 `pgxpool.Pool`）。完整實作參照 `internal/repository/tag_repo.go` 既有 pattern：建構子 `NewActressRepository(db *pgxpool.Pool) *actressRepo`，interface 定義在此檔頂。

```go
const queryUpsertActress = `
    INSERT INTO actresses (name_ja, name_romaji, avatar_key)
    VALUES ($1, $2, $3)
    ON CONFLICT (name_ja) DO UPDATE
        SET name_romaji = EXCLUDED.name_romaji,
            avatar_key  = EXCLUDED.avatar_key
    RETURNING id, created_at
`
const queryAddVideoActress = `
    INSERT INTO video_actresses (video_id, actress_id)
    VALUES ($1, $2)
    ON CONFLICT DO NOTHING
`
const queryActressByVideo = `
    SELECT a.id, a.name_ja, a.name_romaji, a.avatar_key, a.created_at
    FROM actresses a
    JOIN video_actresses va ON va.actress_id = a.id
    WHERE va.video_id = $1
    ORDER BY a.name_ja
`
```

- [ ] **Step 2: Define SuggestionRepository interface + impl**

`internal/repository/suggestion_repo.go`（`payload` 欄位用 `pgx` 直接寫 `model.EnrichedMetadata` 需先 `json.Marshal`；讀回時 `json.Unmarshal` 到 `Payload`）。

```go
const queryCreateSuggestion = `
    INSERT INTO metadata_suggestions (video_id, source, code, payload, status)
    VALUES ($1, $2, $3, $4, $5)
    RETURNING id, fetched_at
`
const querySuggestionsByVideo = `
    SELECT id, video_id, source, code, payload, fetched_at, status
    FROM metadata_suggestions
    WHERE video_id = $1
    ORDER BY fetched_at DESC
`
const querySuggestionByID = `
    SELECT id, video_id, source, code, payload, fetched_at, status
    FROM metadata_suggestions
    WHERE id = $1
`
const queryDeleteSuggestion = `DELETE FROM metadata_suggestions WHERE id = $1`
```

`Create` 內：`payloadJSON, err := json.Marshal(s.Payload)`；找不到（`GetByID`）回 `model.ErrNotFound`。

- [ ] **Step 3: Extend VideoRepository**

在 `internal/repository/video_repo.go` interface 加三個 method，並實作。query const：

```go
const queryUpdateVideoMetadata = `
    UPDATE videos
    SET code = $2, title = $3, release_date = $4, runtime_minutes = $5,
        maker = $6, label = $7, series = $8, cover_key = $9,
        enrichment_status = 'enriched', enriched_at = NOW(), updated_at = NOW()
    WHERE id = $1
`
const querySetEnrichmentStatus = `
    UPDATE videos SET enrichment_status = $2, updated_at = NOW() WHERE id = $1
`
const queryVideosByEnrichmentStatus = `
    SELECT id, title, enrichment_status, code, original_filename, source_id, file_path
    FROM videos WHERE enrichment_status = $1 ORDER BY created_at
`
```

> `ListByEnrichmentStatus` 只需 enrich worker 用到的欄位即可（id/code/original_filename 等），避免改動既有完整 row scan helper。

- [ ] **Step 4: Add TagRepository.GetOrCreateByName**

`internal/repository/tag_repo.go`：

```go
const queryGetOrCreateTag = `
    INSERT INTO tags (name, category) VALUES ($1, $2)
    ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
    RETURNING id, name, category
`
```

`GetOrCreateByName` 執行此 query 回 `*model.Tag`。

- [ ] **Step 5: Add mocks**

`internal/mock/actress_repo.go`、`internal/mock/suggestion_repo.go` 新建；`internal/mock/video_repo.go`、`internal/mock/tag_repo.go` 補新 method。手寫 mock 用 func 欄位可覆寫的風格（對齊既有 mock）：

```go
package mock

import (
	"context"

	"github.com/steven/vaultflix/internal/model"
)

type ActressRepo struct {
	UpsertFn          func(ctx context.Context, a *model.Actress) error
	AddVideoActressFn func(ctx context.Context, videoID, actressID string) error
	GetByVideoIDFn    func(ctx context.Context, videoID string) ([]model.Actress, error)
}

func (m *ActressRepo) Upsert(ctx context.Context, a *model.Actress) error {
	return m.UpsertFn(ctx, a)
}
func (m *ActressRepo) AddVideoActress(ctx context.Context, videoID, actressID string) error {
	return m.AddVideoActressFn(ctx, videoID, actressID)
}
func (m *ActressRepo) GetByVideoID(ctx context.Context, videoID string) ([]model.Actress, error) {
	return m.GetByVideoIDFn(ctx, videoID)
}
```

`SuggestionRepo`、以及 `VideoRepo`/`TagRepo` 既有 mock 的新 method 比照（依既有 mock 檔風格）。

- [ ] **Step 6: Verify build**

Run: `go build ./...` → exit 0。
Run: `task test-integration`（驗 repo SQL）。

- [ ] **Step 7: Commit**

```bash
git add internal/repository/ internal/mock/
git commit -m "feat: add actress/suggestion repos and video/tag repo extensions"
```

---

## Task 4: httpClient wrapper（rate limit + retry + cookie/UA 注入）

**Files:**
- Create: `internal/scraper/httpclient.go`, `internal/scraper/httpclient_test.go`

**Interfaces:**
- Produces: `type ClientOptions struct { Timeout time.Duration; UserAgent string; MinInterval time.Duration; MaxRetries int; Cookies map[string]string }`；`func NewClient(opts ClientOptions) *Client`；`func (c *Client) Get(ctx context.Context, url string) ([]byte, error)`（429/503 → 退避重試；耗盡仍失敗回 `model.ErrSourceUnavailable`）。

- [ ] **Step 1: Write failing test**

```go
package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientGet_RetriesOn503ThenSucceeds(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	c := NewClient(ClientOptions{Timeout: 2 * time.Second, MaxRetries: 2, MinInterval: time.Millisecond})
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(body) != "OK" || atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("got body=%q hits=%d", body, hits)
	}
}

func TestClientGet_SendsUserAgentAndCookies(t *testing.T) {
	var gotUA, gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		if ck, err := r.Cookie("cf_clearance"); err == nil {
			gotCookie = ck.Value
		}
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	c := NewClient(ClientOptions{Timeout: time.Second, UserAgent: "Vaultflix/1.0", Cookies: map[string]string{"cf_clearance": "abc"}})
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if gotUA != "Vaultflix/1.0" || gotCookie != "abc" {
		t.Fatalf("ua=%q cookie=%q", gotUA, gotCookie)
	}
}
```

- [ ] **Step 2: Run, verify fail** — `go test ./internal/scraper/ -run TestClientGet -v` → FAIL（undefined）。

- [ ] **Step 3: Implement**

```go
package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

type ClientOptions struct {
	Timeout     time.Duration
	UserAgent   string
	MinInterval time.Duration
	MaxRetries  int
	Cookies     map[string]string
}

type Client struct {
	hc       *http.Client
	opts     ClientOptions
	lastReq  time.Time
}

func NewClient(opts ClientOptions) *Client {
	if opts.Timeout == 0 {
		opts.Timeout = 15 * time.Second
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "Vaultflix/1.0"
	}
	return &Client{hc: &http.Client{Timeout: opts.Timeout}, opts: opts}
}

// Get 抓取 url，含速率間隔與退避重試。429/503 重試；耗盡回 ErrSourceUnavailable。
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastStatus int
	for attempt := 0; attempt <= c.opts.MaxRetries; attempt++ {
		if c.opts.MinInterval > 0 {
			if wait := time.Until(c.lastReq.Add(c.opts.MinInterval)); wait > 0 {
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		}
		c.lastReq = time.Now()

		body, status, err := c.do(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("http get %s: %w", url, err)
		}
		lastStatus = status
		switch {
		case status == http.StatusOK:
			return body, nil
		case status == http.StatusForbidden:
			return nil, fmt.Errorf("status 403 for %s: %w", url, model.ErrScrapeBlocked)
		case status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable:
			backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		default:
			return nil, fmt.Errorf("status %d for %s: %w", status, url, model.ErrSourceUnavailable)
		}
	}
	return nil, fmt.Errorf("exhausted retries (last status %d) for %s: %w", lastStatus, url, model.ErrSourceUnavailable)
}

func (c *Client) do(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	for k, v := range c.opts.Cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}
```

- [ ] **Step 4: Run tests, verify pass** — `go test ./internal/scraper/ -run TestClientGet -v` → PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/scraper/httpclient.go internal/scraper/httpclient_test.go
git commit -m "feat: add rate-limited retrying http client for scrapers"
```

---

## Task 5: MetadataScraper interface + JavBus scraper（goquery, fixture TDD）

**Files:**
- Create: `internal/scraper/scraper.go`, `internal/scraper/javbus.go`, `internal/scraper/javbus_test.go`, `internal/scraper/testdata/javbus_dasd626.html`
- Modify: `go.mod`

**Interfaces:**
- Consumes: `Client`（Task 4）、`avid`、`model.EnrichedMetadata`。
- Produces: `type MetadataScraper interface { Source() string; ScrapeByCode(ctx context.Context, code string) (*model.EnrichedMetadata, error) }`；`func NewJavBusScraper(c *Client, baseURL string) *JavBusScraper`；`func parseJavBus(html []byte, code string) (*model.EnrichedMetadata, error)`（純函式，供測試）。

> **Selector 真確性**：以下 selector 依 JavBus 已知頁面結構（`.container .info` 區塊、`.movie .bigImage img`、`.star-name a`、`.genre a`）撰寫，**實作者必須先抓一份真實頁面存成 fixture 再對照微調**（Step 1）。

- [ ] **Step 1: Capture fixture + write failing test**

先抓一份真頁存檔（一次性，存進 repo 當離線 fixture）：

```bash
curl -A "Mozilla/5.0" "https://www.javbus.com/DASD-626" -o internal/scraper/testdata/javbus_dasd626.html
```

`internal/scraper/javbus_test.go`：

```go
package scraper

import (
	"os"
	"testing"
)

func TestParseJavBus_DASD626(t *testing.T) {
	html, err := os.ReadFile("testdata/javbus_dasd626.html")
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseJavBus(html, "DASD-626")
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "DASD-626" {
		t.Errorf("code = %q", got.Code)
	}
	if got.Title == "" {
		t.Error("title empty")
	}
	if got.Maker == "" {
		t.Error("maker empty")
	}
	if len(got.Actresses) == 0 {
		t.Error("expected at least one actress")
	}
	if len(got.Genres) == 0 {
		t.Error("expected genres")
	}
	if got.CoverURL == "" {
		t.Error("cover url empty")
	}
}
```

- [ ] **Step 2: Run, verify fail** — `go test ./internal/scraper/ -run TestParseJavBus -v` → FAIL（undefined）。

- [ ] **Step 3: Add goquery + implement scraper interface**

```bash
go get github.com/PuerkitoBio/goquery
```

`internal/scraper/scraper.go`:

```go
package scraper

import (
	"context"

	"github.com/steven/vaultflix/internal/model"
)

// MetadataScraper 對單一來源站依番號抓 metadata。
// ScrapeByCode 找不到番號頁回 model.ErrCodeNotFound；被擋回 model.ErrScrapeBlocked；
// 連線/解析失敗回 model.ErrSourceUnavailable。
type MetadataScraper interface {
	Source() string
	ScrapeByCode(ctx context.Context, code string) (*model.EnrichedMetadata, error)
}
```

`internal/scraper/javbus.go`（`ScrapeByCode` 組 URL → `Client.Get` → `parseJavBus`；`parseJavBus` 用 goquery 解析）:

```go
package scraper

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/steven/vaultflix/internal/model"
)

type JavBusScraper struct {
	client  *Client
	baseURL string
}

func NewJavBusScraper(c *Client, baseURL string) *JavBusScraper {
	if baseURL == "" {
		baseURL = "https://www.javbus.com"
	}
	return &JavBusScraper{client: c, baseURL: baseURL}
}

func (s *JavBusScraper) Source() string { return "javbus" }

func (s *JavBusScraper) ScrapeByCode(ctx context.Context, code string) (*model.EnrichedMetadata, error) {
	url := fmt.Sprintf("%s/%s", strings.TrimRight(s.baseURL, "/"), code)
	body, err := s.client.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("javbus get %s: %w", code, err)
	}
	return parseJavBus(body, code)
}

// parseJavBus 解析 JavBus 影片頁。Selector 依已知結構，需以 fixture 驗證。
func parseJavBus(html []byte, code string) (*model.EnrichedMetadata, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse javbus html: %w", model.ErrSourceUnavailable)
	}
	m := &model.EnrichedMetadata{Code: code}
	m.Title = strings.TrimSpace(doc.Find("h3").First().Text())
	if m.Title == "" {
		return nil, fmt.Errorf("javbus empty title for %s: %w", code, model.ErrCodeNotFound)
	}
	doc.Find(".movie .info p").Each(func(_ int, p *goquery.Selection) {
		header := strings.TrimSpace(p.Find("span.header").First().Text())
		switch {
		case strings.Contains(header, "製作商"), strings.Contains(header, "Studio"):
			m.Maker = cleanLabel(p.Text(), header)
		case strings.Contains(header, "發行商"), strings.Contains(header, "Label"):
			m.Label = cleanLabel(p.Text(), header)
		case strings.Contains(header, "系列"), strings.Contains(header, "Series"):
			m.Series = cleanLabel(p.Text(), header)
		case strings.Contains(header, "長度"), strings.Contains(header, "Length"):
			m.RuntimeMinutes = parseMinutes(p.Text())
		}
	})
	doc.Find(".genre a").Each(func(_ int, a *goquery.Selection) {
		if g := strings.TrimSpace(a.Text()); g != "" {
			m.Genres = append(m.Genres, g)
		}
	})
	doc.Find(".star-name a").Each(func(_ int, a *goquery.Selection) {
		name := strings.TrimSpace(a.Text())
		if name == "" {
			return
		}
		avatar, _ := a.PardsAvatar() // placeholder; see note
		m.Actresses = append(m.Actresses, model.ActressMeta{NameJa: name, AvatarURL: avatar})
	})
	if src, ok := doc.Find(".bigImage img").Attr("src"); ok {
		m.CoverURL = absoluteURL(src)
	}
	return m, nil
}

func cleanLabel(full, header string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(full), header))
}

func parseMinutes(s string) int {
	var digits strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	n, _ := strconv.Atoi(digits.String())
	return n
}

func absoluteURL(src string) string {
	if strings.HasPrefix(src, "//") {
		return "https:" + src
	}
	return src
}
```

> **實作備註**：上方 `a.PardsAvatar()` 是佔位 — JavBus 女優頭像在 `.star a img` 的 `src`，actress 名在同一 anchor。實作者依 fixture 把「名 + 頭像」用 `.star li a`（內含 `img` 與文字）迭代取出：`name = a.Find("img").AttrOr("title", "")`、`avatar = a.Find("img").AttrOr("src", "")`。請依實際 DOM 修正，移除佔位呼叫。

- [ ] **Step 4: Run test, adjust selectors against fixture, verify pass**

Run: `go test ./internal/scraper/ -run TestParseJavBus -v`
依 fixture 實際 DOM 調整 selector 直到 PASS（title/maker/actress/genre/cover 皆非空）。

- [ ] **Step 5: Commit**

```bash
git add internal/scraper/scraper.go internal/scraper/javbus.go internal/scraper/javbus_test.go internal/scraper/testdata/ go.mod go.sum
git commit -m "feat: add MetadataScraper interface and JavBus scraper"
```

---

## Task 6: JavLibrary scraper（fixture TDD）

**Files:**
- Create: `internal/scraper/javlibrary.go`, `internal/scraper/javlibrary_test.go`, `internal/scraper/testdata/javlibrary_dasd626.html`

**Interfaces:**
- Produces: `func NewJavLibraryScraper(c *Client, baseURL string) *JavLibraryScraper`（實作 `MetadataScraper`，`Source()="javlibrary"`）；`func parseJavLibrary(html []byte, code string) (*model.EnrichedMetadata, error)`。

- [ ] **Step 1: Capture fixture + failing test**（結構同 Task 5 Step 1，檔名 `javlibrary_dasd626.html`，測試函式 `TestParseJavLibrary_DASD626`，斷言 code/title/genres/actresses 非空）。
- [ ] **Step 2: Run, verify fail.**
- [ ] **Step 3: Implement**（JavLibrary 詳情頁欄位在 `#video_info` 的 `.item` 區塊：`#video_maker .maker`、`#video_label .label`、`#video_genres .genre a`、`#video_cast .cast .star a`、`#video_length .text`(分鐘)、`#video_jacket img` src 封面）。`parseJavLibrary` 比照 `parseJavBus` 風格，找不到主標題回 `model.ErrCodeNotFound`。
- [ ] **Step 4: Run test, adjust selectors, verify pass.**
- [ ] **Step 5: Commit**

```bash
git add internal/scraper/javlibrary.go internal/scraper/javlibrary_test.go internal/scraper/testdata/javlibrary_dasd626.html
git commit -m "feat: add JavLibrary scraper"
```

> JavDB scraper（`internal/scraper/javdb.go`）：本 Phase 僅建立 struct + `ScrapeByCode` 走相同 Client（帶 cookie），**預設不註冊進 aggregator**、不納驗收測試（Cloudflare）。可在本 task 後追加一個極簡 commit 放佔位實作，或留待 Phase 1.x。

---

## Task 7: ScraperAggregator（per-field 優先序合併）

**Files:**
- Create: `internal/scraper/aggregator.go`, `internal/scraper/aggregator_test.go`

**Interfaces:**
- Produces: `func MergeByPriority(results []SourceResult) *model.EnrichedMetadata`；`type SourceResult struct { Source string; Data *model.EnrichedMetadata }`。輸入已按來源優先序排好（index 0 最高）。每個 scalar 欄位取「第一個非空」的來源值；list 欄位（genres/actresses）取「第一個非空 list」的來源（不混合，避免重複/衝突）。

- [ ] **Step 1: Write failing test**

```go
package scraper

import (
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func TestMergeByPriority_FirstNonEmptyWins(t *testing.T) {
	results := []SourceResult{
		{Source: "javbus", Data: &model.EnrichedMetadata{Code: "DASD-626", Title: "", Maker: "Das!", Genres: nil}},
		{Source: "javlibrary", Data: &model.EnrichedMetadata{Code: "DASD-626", Title: "後輩", Maker: "X", Genres: []string{"巨乳"}}},
	}
	got := MergeByPriority(results)
	if got.Title != "後輩" {
		t.Errorf("title = %q, want 後輩 (fallback to javlibrary)", got.Title)
	}
	if got.Maker != "Das!" {
		t.Errorf("maker = %q, want Das! (javbus wins)", got.Maker)
	}
	if len(got.Genres) != 1 || got.Genres[0] != "巨乳" {
		t.Errorf("genres = %v, want [巨乳]", got.Genres)
	}
}

func TestMergeByPriority_Empty(t *testing.T) {
	if got := MergeByPriority(nil); got != nil {
		t.Errorf("want nil for empty input, got %v", got)
	}
}
```

- [ ] **Step 2: Run, verify fail.**
- [ ] **Step 3: Implement**

```go
package scraper

import "github.com/steven/vaultflix/internal/model"

type SourceResult struct {
	Source string
	Data   *model.EnrichedMetadata
}

// MergeByPriority 依輸入順序（index 0 最高）逐欄位取第一個非空值。input 空回 nil。
func MergeByPriority(results []SourceResult) *model.EnrichedMetadata {
	if len(results) == 0 {
		return nil
	}
	out := &model.EnrichedMetadata{}
	for _, r := range results {
		d := r.Data
		if d == nil {
			continue
		}
		if out.Code == "" {
			out.Code = d.Code
		}
		if out.Title == "" {
			out.Title = d.Title
		}
		if out.ReleaseDate == nil {
			out.ReleaseDate = d.ReleaseDate
		}
		if out.RuntimeMinutes == 0 {
			out.RuntimeMinutes = d.RuntimeMinutes
		}
		if out.Maker == "" {
			out.Maker = d.Maker
		}
		if out.Label == "" {
			out.Label = d.Label
		}
		if out.Series == "" {
			out.Series = d.Series
		}
		if out.CoverURL == "" {
			out.CoverURL = d.CoverURL
		}
		if len(out.Genres) == 0 {
			out.Genres = d.Genres
		}
		if len(out.Actresses) == 0 {
			out.Actresses = d.Actresses
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests, verify pass.**
- [ ] **Step 5: Commit**

```bash
git add internal/scraper/aggregator.go internal/scraper/aggregator_test.go
git commit -m "feat: add per-field priority metadata aggregator"
```

---

## Task 8: MinIO cover/avatar upload + presign

**Files:**
- Modify: `internal/service/minio_service.go`, `internal/mock/minio.go`（既有 mock）

**Interfaces:**
- Produces: `MinIOClient` 新增 `UploadCover(ctx, objectKey, filePath string) error`、`UploadActressAvatar(ctx, objectKey, filePath string) error`、`GenerateCoverPresignedURL(ctx, objectKey string, expiry time.Duration) (string, error)`、`GenerateActressAvatarPresignedURL(ctx, objectKey string, expiry time.Duration) (string, error)`。

- [ ] **Step 1: Extend interface + impl**

在 `MinIOClient` interface 加四個 method。實作比照既有 `UploadThumbnail`（`PutObject` 帶 `ContentType: "image/jpeg"`，存進既有 image/thumbnail bucket，object key 以 `covers/`、`actresses/` 前綴區分）；presign 比照 `GenerateThumbnailPresignedURL`（走 presignClient + URLCache）。

- [ ] **Step 2: Update mock**

`internal/mock/minio.go` 補四個 func 欄位 + method（風格同既有）。

- [ ] **Step 3: Verify build** — `go build ./...` → exit 0。

- [ ] **Step 4: Commit**

```bash
git add internal/service/minio_service.go internal/mock/minio.go
git commit -m "feat: add MinIO cover/actress-avatar upload and presign"
```

---

## Task 9: EnrichmentService — EnrichVideo（single）+ WS message types

**Files:**
- Create: `internal/service/enrichment_service.go`, `internal/service/enrichment_service_test.go`
- Modify: `internal/websocket/message.go`

**Interfaces:**
- Consumes: `[]scraper.MetadataScraper`、`scraper.MergeByPriority`、`MinIOClient`、`VideoRepository`、`ActressRepository`、`SuggestionRepository`、`TagRepository`、`websocket.Notifier`、`avid`。
- Produces: `func NewEnrichmentService(...) *EnrichmentService`；`func (s *EnrichmentService) EnrichVideo(ctx context.Context, videoID, userID string) error`。enrich 成功寫 suggestions（每來源一列）+ 下載 cover/avatar 到 MinIO + 設 `enrichment_status=suggested` + WS 通知。抽不到番號設 `no_code` 回 `model.ErrCodeNotFound`；全來源失敗設 `failed`。

- [ ] **Step 1: Add WS message types**

`internal/websocket/message.go` 加：

```go
	TypeEnrichProgress = "enrich_progress"
	TypeEnrichComplete = "enrich_complete"
	TypeEnrichError    = "enrich_error"
```

- [ ] **Step 2: Write failing test（happy + no_code）**

```go
package service

import (
	"context"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/scraper"
)

func TestEnrichVideo_NoCode(t *testing.T) {
	videoRepo := &mock.VideoRepo{
		GetByIDFn: func(_ context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: id, OriginalFilename: "family_trip.mp4"}, nil
		},
		SetEnrichmentStatusFn: func(_ context.Context, id, status string) error {
			if status != model.EnrichmentNoCode {
				t.Errorf("status = %q, want no_code", status)
			}
			return nil
		},
	}
	svc := NewEnrichmentService(nil, videoRepo, &mock.ActressRepo{}, &mock.SuggestionRepo{}, &mock.TagRepo{}, &mock.MinIO{}, &mock.Notifier{})
	err := svc.EnrichVideo(context.Background(), "v1", "u1")
	if err == nil {
		t.Fatal("want ErrCodeNotFound, got nil")
	}
}

func TestEnrichVideo_WritesSuggestion(t *testing.T) {
	var created *model.MetadataSuggestion
	videoRepo := &mock.VideoRepo{
		GetByIDFn:             func(_ context.Context, id string) (*model.Video, error) { return &model.Video{ID: id, OriginalFilename: "DASD-626.mp4"}, nil },
		SetEnrichmentStatusFn: func(_ context.Context, id, status string) error { return nil },
	}
	sugRepo := &mock.SuggestionRepo{CreateFn: func(_ context.Context, s *model.MetadataSuggestion) error { created = s; return nil }}
	fakeScraper := &mock.Scraper{
		SourceVal: "javbus",
		ScrapeFn: func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
			return &model.EnrichedMetadata{Code: code, Title: "T", Maker: "M"}, nil
		},
	}
	svc := NewEnrichmentService([]scraper.MetadataScraper{fakeScraper}, videoRepo, &mock.ActressRepo{}, sugRepo, &mock.TagRepo{}, &mock.MinIO{}, &mock.Notifier{})
	if err := svc.EnrichVideo(context.Background(), "v1", "u1"); err != nil {
		t.Fatal(err)
	}
	if created == nil || created.Source != "javbus" || created.Code != "DASD-626" {
		t.Fatalf("suggestion not created correctly: %+v", created)
	}
}
```

（`mock.Scraper` 在本 task 一併新增 `internal/mock/scraper.go`：`SourceVal string`、`ScrapeFn func(ctx, code) (*model.EnrichedMetadata, error)`。`mock.Notifier` 若不存在則新增 no-op。）

- [ ] **Step 3: Run, verify fail.**

- [ ] **Step 4: Implement EnrichmentService.EnrichVideo**

要點（拆成 ≤50 行的私有 helper）：
1. `video, err := s.videoRepo.GetByID(ctx, videoID)`，wrap err。
2. `code, ok := avid.ExtractCode(video.OriginalFilename)`；`!ok` → `SetEnrichmentStatus(no_code)` + WS enrich_error + 回 `model.ErrCodeNotFound`。
3. 逐 scraper `ScrapeByCode`：成功收進 `[]scraper.SourceResult` 並 `suggestionRepo.Create`（payload=該來源結果）；個別來源失敗只記 slog.Warn 不中止。
4. 全部失敗（無任何 SourceResult）→ `SetEnrichmentStatus(failed)` + WS enrich_error + 回 wrap 後 err。
5. 下載 merged（或各來源）cover/avatar：`MergeByPriority` 取主 cover → 下載到 scratch temp（`os.CreateTemp`）→ `minio.UploadCover` → 設 video.CoverKey（暫存於 suggestion payload 或在 accept 時才上傳——**Phase 1 在 enrich 時上傳並把 key 寫進 suggestion payload 的 CoverURL 改為 MinIO key**）。
6. `SetEnrichmentStatus(suggested)` + WS enrich_complete。

> **影像上傳時機抉擇**：在 enrich 時上傳 cover/avatar（reject 後成孤兒，可後續清理 job 處理）較簡單；替代是 accept 時才上傳（避免孤兒，但 accept handler 變重）。**Phase 1 選 enrich 時上傳**，孤兒清理列 backlog。實作時把 `EnrichedMetadata.CoverURL` 在存 suggestion 前換成已上傳的 MinIO object key（欄位語意改為 key；前端 presign 顯示）。

- [ ] **Step 5: Run tests, verify pass.**

- [ ] **Step 6: Commit**

```bash
git add internal/service/enrichment_service.go internal/service/enrichment_service_test.go internal/websocket/message.go internal/mock/scraper.go internal/mock/notifier.go
git commit -m "feat: add EnrichmentService.EnrichVideo with suggestion staging"
```

---

## Task 10: EnrichmentService — Accept / Reject suggestion

**Files:**
- Modify: `internal/service/enrichment_service.go`, `internal/service/enrichment_service_test.go`

**Interfaces:**
- Produces: `func (s *EnrichmentService) AcceptSuggestion(ctx context.Context, videoID, suggestionID string, override model.SuggestionOverride) error`；`func (s *EnrichmentService) RejectSuggestion(ctx context.Context, videoID, suggestionID string) error`。

- [ ] **Step 1: Write failing test**

```go
func TestAcceptSuggestion_AppliesMetadataAndActressesAndGenres(t *testing.T) {
	payload := model.EnrichedMetadata{
		Code: "DASD-626", Title: "原標題", Maker: "M",
		Genres:    []string{"巨乳"},
		Actresses: []model.ActressMeta{{NameJa: "女優A"}},
	}
	var updated model.VideoMetadataUpdate
	var linkedActress, linkedTag bool
	videoRepo := &mock.VideoRepo{UpdateMetadataFn: func(_ context.Context, id string, m model.VideoMetadataUpdate) error { updated = m; return nil }}
	sugRepo := &mock.SuggestionRepo{
		GetByIDFn: func(_ context.Context, id string) (*model.MetadataSuggestion, error) {
			return &model.MetadataSuggestion{ID: id, VideoID: "v1", Source: "javbus", Code: "DASD-626", Payload: payload}, nil
		},
		DeleteFn: func(_ context.Context, id string) error { return nil },
	}
	actRepo := &mock.ActressRepo{
		UpsertFn:          func(_ context.Context, a *model.Actress) error { a.ID = "a1"; return nil },
		AddVideoActressFn: func(_ context.Context, v, a string) error { linkedActress = true; return nil },
	}
	tagRepo := &mock.TagRepo{
		GetOrCreateByNameFn: func(_ context.Context, name, cat string) (*model.Tag, error) { return &model.Tag{ID: 7, Name: name, Category: cat}, nil },
		AddVideoTagFn:       func(_ context.Context, v string, id int) error { linkedTag = true; return nil },
	}
	svc := NewEnrichmentService(nil, videoRepo, actRepo, sugRepo, tagRepo, &mock.MinIO{}, &mock.Notifier{})
	newTitle := "覆寫標題"
	err := svc.AcceptSuggestion(context.Background(), "v1", "s1", model.SuggestionOverride{Title: &newTitle})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "覆寫標題" {
		t.Errorf("title = %q, want 覆寫標題 (override applied)", updated.Title)
	}
	if !linkedActress || !linkedTag {
		t.Errorf("actress linked=%v tag linked=%v", linkedActress, linkedTag)
	}
}
```

- [ ] **Step 2: Run, verify fail.**

- [ ] **Step 3: Implement**

`AcceptSuggestion`：
1. `sug, err := s.suggestionRepo.GetByID(ctx, suggestionID)`；`errors.Is(err, model.ErrNotFound)` 往上傳。
2. 校驗 `sug.VideoID == videoID`，否則回 `model.ErrNotFound`（避免跨片誤套）。
3. 套 override：title 用 `override.Title` 若非 nil 否則 `sug.Payload.Title`；genres 同理。
4. `videoRepo.UpdateMetadata(ctx, videoID, model.VideoMetadataUpdate{...})`（含 CoverKey=payload.CoverURL，此時已是 MinIO key）。
5. 每個 actress：`Upsert` → `AddVideoActress`。
6. 每個 genre：`tagRepo.GetOrCreateByName(name, "genre")` → `AddVideoTag`。
7. `suggestionRepo.Delete(ctx, suggestionID)`（或標記 accepted；Phase 1 用 Delete）。

`RejectSuggestion`：校驗歸屬 → `suggestionRepo.Delete`；若該片已無其他 suggestion，`SetEnrichmentStatus(none)`（可重抓）。

- [ ] **Step 4: Run tests, verify pass.**

- [ ] **Step 5: Commit**

```bash
git add internal/service/enrichment_service.go internal/service/enrichment_service_test.go
git commit -m "feat: add accept/reject suggestion to EnrichmentService"
```

---

## Task 11: EnrichmentService — 批次 job（backfill 模式）

**Files:**
- Modify: `internal/service/enrichment_service.go`, `internal/service/enrichment_service_test.go`

**Interfaces:**
- Produces: `func (s *EnrichmentService) StartBatchAsync(ctx context.Context, status, userID string) (*model.EnrichJob, error)`（沿用 `backfill_service.go` 的單一執行中限制 + 進度 + WS；`status` 預設過濾 `pending`）；`func (s *EnrichmentService) ActiveJob() *model.EnrichJob`。`model.EnrichJob` 結構比照 `model.BackfillJob`（ID/Total/Processed/Failed/State）。

- [ ] **Step 1: Write failing test**（mock `videoRepo.ListByEnrichmentStatusFn` 回 2 部，stub `EnrichVideo` 邏輯走 fakeScraper；斷言 job.Total==2、跑完 state=completed、重入回 `model.ErrConflict`）。參照 `internal/service/backfill_service_test.go` 既有測試風格。
- [ ] **Step 2: Run, verify fail.**
- [ ] **Step 3: Implement**（mutex 保護 `activeJob`；goroutine 迴圈呼叫 `EnrichVideo`，個別失敗計入 `Failed` 不中止；每片 WS `enrich_progress`；結束 WS `enrich_complete`。`StartBatchAsync` 若 `activeJob != nil && running` 回 `model.ErrConflict`。`model.EnrichJob` 加進 `internal/model/enrichment.go`）。
- [ ] **Step 4: Run tests, verify pass.**
- [ ] **Step 5: Commit**

```bash
git add internal/service/enrichment_service.go internal/service/enrichment_service_test.go internal/model/enrichment.go
git commit -m "feat: add batch enrichment job with progress and single-run guard"
```

---

## Task 12: EnrichmentHandler + 路由

**Files:**
- Create: `internal/handler/enrichment_handler.go`, `internal/handler/enrichment_handler_test.go`
- Modify: `cmd/server/main.go`（僅註冊路由，wiring 在 Task 14 完成）

**Interfaces:**
- Consumes: `*service.EnrichmentService`。
- Produces: `func NewEnrichmentHandler(svc *service.EnrichmentService) *EnrichmentHandler` + handler methods：`EnrichVideo`、`StartBatch`、`ActiveJob`、`ListSuggestions`、`AcceptSuggestion`、`RejectSuggestion`。

> handler 依賴 concrete `*service.EnrichmentService`（對齊既有 `VideoHandler` 依賴 concrete service 的慣例）。若要可測，handler 透過小 interface 依賴 service method；Phase 1 用既有慣例 + 在 handler test 中以真 service + mock repo 建構。

- [ ] **Step 1: Write failing test（accept 400/404 + reject 204）**

用 `httptest` + gin test context，service 以 mock repo 組成。範例斷言：
- `POST /videos/:id/suggestions/:sid/accept` 不存在的 sid → service 回 `model.ErrNotFound` → handler 回 404。
- 合法 → 200 + `SuccessResponse`。
- `DELETE` reject → 204。
- body 非法 JSON → 400。

- [ ] **Step 2: Run, verify fail.**

- [ ] **Step 3: Implement handler**

每個 method：解析 param/body → 呼叫 service → 依 error 映射 HTTP 碼（`errors.Is(err, model.ErrNotFound)`→404；`errors.Is(err, model.ErrCodeNotFound)`→422 或 200+狀態訊息；`errors.Is(err, model.ErrConflict)`→409；其餘→500）→ 組 `SuccessResponse`/`ErrorResponse`。`EnrichVideo`/`StartBatch` 回 202。log 只在此層。

- [ ] **Step 4: Register routes** in `cmd/server/main.go`（protected `api` group）:

```go
api.POST("/videos/:id/enrich", enrichHandler.EnrichVideo)
api.GET("/videos/:id/suggestions", enrichHandler.ListSuggestions)
api.POST("/videos/:id/suggestions/:sid/accept", enrichHandler.AcceptSuggestion)
api.DELETE("/videos/:id/suggestions/:sid", enrichHandler.RejectSuggestion)
api.POST("/enrich-jobs", enrichHandler.StartBatch)
api.GET("/enrich-jobs/active", enrichHandler.ActiveJob)
```

（`enrichHandler` 的建構在 Task 14 補；本步驟先寫路由行，編譯靠 Task 14。為保持本 task 可編譯，可暫時在 main.go 用已建構的 service —— 或將路由註冊與 Task 14 wiring 合併提交。實作者若採前者，確認 `go build` 綠再 commit。）

- [ ] **Step 5: Run handler tests, verify pass + build.**

- [ ] **Step 6: Commit**

```bash
git add internal/handler/enrichment_handler.go internal/handler/enrichment_handler_test.go cmd/server/main.go
git commit -m "feat: add enrichment handler and routes"
```

---

## Task 13: 整合進 ImportService（抽番號 seed 狀態）

**Files:**
- Modify: `internal/service/import_service.go`

**Interfaces:**
- Consumes: `avid.ExtractCode`、`model.Enrichment*` 常數。
- Produces: 匯入建立的 `Video` 帶 `Code` 與 `EnrichmentStatus`（`pending` 或 `no_code`）。

- [ ] **Step 1: Modify processOneFile**

在 `processOneFile` 組 `model.Video`（`videoRepo.Create` 之前）加：

```go
if code, ok := avid.ExtractCode(filename); ok {
	video.Code = code
	video.EnrichmentStatus = model.EnrichmentPending
} else {
	video.EnrichmentStatus = model.EnrichmentNoCode
}
```

（import 的 `Create` query 與 `model.Video` 對應欄位需含 `code`、`enrichment_status` —— 確認 `video_repo.go` 的 `queryCreateVideo` 與 INSERT 欄位清單已含這兩欄；若否，補上並對應 `Create` 的參數。）

- [ ] **Step 2: Verify**

Run: `go build ./...` → exit 0。
Run: `task test-integration`（import 流程觸及 DB/掃描；確認新欄位寫入無誤）。

- [ ] **Step 3: Commit**

```bash
git add internal/service/import_service.go internal/repository/video_repo.go
git commit -m "feat: seed enrichment status during import via avid code extraction"
```

---

## Task 14: Wiring + Config

**Files:**
- Modify: `cmd/server/main.go`, `internal/config/config.go`, `.env.example`（若存在）

**Interfaces:**
- Consumes: 全部上述建構子。
- Produces: 啟動時建構 scraper clients、aggregator、`EnrichmentService`、`EnrichmentHandler` 並注入路由；config 讀 `ENRICH_HTTP_TIMEOUT`、`ENRICH_USER_AGENT`（infra）。

- [ ] **Step 1: Add config fields**

`internal/config/config.go` 加 `EnrichHTTPTimeout time.Duration`、`EnrichUserAgent string`（從 env 讀，附預設：timeout 15s、UA `Vaultflix/1.0`）。業務參數（source 優先序 / rate / cookie）**不進 config**，走 request（Phase 1 批次 job 用預設順序 `[javbus, javlibrary]`、預設 rate；request override 留 handler 參數，可後續補）。

- [ ] **Step 2: Wire in main.go**

```go
httpClient := scraper.NewClient(scraper.ClientOptions{
	Timeout:     cfg.EnrichHTTPTimeout,
	UserAgent:   cfg.EnrichUserAgent,
	MinInterval: 2 * time.Second,
	MaxRetries:  2,
})
scrapers := []scraper.MetadataScraper{
	scraper.NewJavBusScraper(httpClient, ""),
	scraper.NewJavLibraryScraper(httpClient, ""),
}
enrichService := service.NewEnrichmentService(scrapers, videoRepo, actressRepo, suggestionRepo, tagRepo, minioSvc, hub)
enrichHandler := handler.NewEnrichmentHandler(enrichService)
```

（`actressRepo`、`suggestionRepo` 用 Task 3 建構子；`hub` 為既有 websocket Hub。）

- [ ] **Step 3: Verify full build + boot**

Run: `go build ./...` → exit 0。
Run: `task verify` → 綠（go vet/gofmt/test + web）。

- [ ] **Step 4: Commit**

```bash
git add cmd/server/main.go internal/config/config.go
git commit -m "chore: wire enrichment service, scrapers, and config"
```

---

## Task 15: 端到端整合驗證

**Files:** 無（驗證 task）。

- [ ] **Step 1:** `task verify` → 綠。
- [ ] **Step 2:** `task test-integration` → 綠（import 抽番號、enrich 寫 suggestion、accept 套用、migration 套用皆覆蓋）。
- [ ] **Step 3:** 對照 spec §14 驗收清單逐項打勾；缺漏者回補對應 task。
- [ ] **Step 4:** 收尾用 superpowers:finishing-a-development-branch 開 PR（base `main`；PR 描述註明 migration 013、與 admin 分支併行注意）。

---

## Self-Review

**Spec coverage（spec §→task）**：§2 番號抽取→T1；§3 架構→T9/T11/T13；§5 schema/model→T2；§6 repo→T3、scraper→T4-7、service→T9-11、handler→T12、WS→T9；§7 MinIO→T8；§8 config→T14；§9 錯誤處理→分散於 T4/T9/T12；§10 import 整合→T13；§11 依賴 goquery→T5；§12 測試→各 task TDD + T15;§13 併行→T2(013)/T15(PR)；§14 驗收→T15。**無遺漏**。

**Placeholder scan**：T5 `parseJavBus` 含一個刻意標示的 selector 佔位（`PardsAvatar()`）並附明確修正指示 + fixture 驗證步驟（scraper selector 本質需對真實 DOM 校準，已轉成「capture fixture → 調 selector → 綠」的可執行步驟）。其餘無 TBD/TODO。

**Type consistency**：`EnrichVideo(ctx, videoID, userID)`、`AcceptSuggestion(ctx, videoID, suggestionID, override)`、`MergeByPriority([]SourceResult)`、`MetadataScraper.ScrapeByCode`、repo method 名（`UpdateMetadata`/`SetEnrichmentStatus`/`ListByEnrichmentStatus`/`Upsert`/`AddVideoActress`/`GetOrCreateByName`）在 T3 定義與 T9-11 使用一致。`EnrichedMetadata.CoverURL` 語意在 T9 由 image URL 轉為 MinIO key —— 已於 T9/T10 標註，accept 直接當 CoverKey 用，一致。
