# remux VOD-on-the-fly Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** remux 影片回應完整 VOD HLS 清單(含 `#EXT-X-ENDLIST`)並按需產生 keyframe 對齊分段,使進度條可見、任意 seek 可用。

**Architecture:** 三個獨立單元取代 Phase 1 的單一線性 ffmpeg event session:`KeyframeProber`(ffprobe packet 掃描 → 邊界表 → 存 DB 新表 `video_keyframe_index`)、`ManifestBuilder`(純函式,邊界表 → VOD m3u8)、`SegmentCache`(per-video on-demand `ffmpeg -ss -c copy` 單段產生 + singleflight + idle sweep + LRU 容量上限)。探測三路觸發:admin backfill、import 後自動、首播 lazy fallback(503 + 前端「準備中」輪詢)。

**Tech Stack:** Go 1.22+(stdlib only,singleflight 手寫)、PostgreSQL 16(JSONB)、ffmpeg/ffprobe、React 18 + hls.js、vitest。

**Spec:** `docs/superpowers/specs/2026-07-21-remux-vod-on-the-fly-design.md`(已核可,含實測數據與已定案決定)

## Global Constraints

- 不引入新第三方依賴(singleflight 以 mutex + chan 手寫)。
- 每個 Go 檔 ≤300 行、每個 function ≤50 行;import 三組分隔。
- Error 一律 `%w` wrap、小寫開頭;只在 handler 層 log + HTTP response。
- `log/slog` 結構化欄位,禁止字串拼接。
- 跨層依賴 consumer-side 小 interface(比照 `codec_backfill_service.go` 的 `codecVideoRepo` pattern);pointer receiver。
- SQL 關鍵字大寫、parameterized query、query 放檔案頂部 const;migration down 完整可逆。
- 測試:table-driven、手寫 mock(不引框架)、`Test<Function>_<Scenario>` 命名;每 task 結束 `go test ./...`(或該 package)綠才 commit。
- Handler 回應格式:`model.SuccessResponse` / `model.ErrorResponse`;status code 依 CLAUDE.md 規則。
- **每個 task 完成後專案必須可編譯**(舊 event 路徑的刪除排在 cutover task,不提早)。
- Commit format:`<type>: <description>` + `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

---

### Task 1: Migration + model + repositories(邊界表持久層)

**Files:**
- Create: `migrations/018_create_video_keyframe_index.up.sql`
- Create: `migrations/018_create_video_keyframe_index.down.sql`
- Create: `internal/model/keyframe_index.go`
- Create: `internal/repository/keyframe_index_repo.go`
- Create: `internal/repository/video_keyframe_repo.go`
- Modify: `internal/repository/video_repo.go`(`VideoRepository` interface 加方法宣告)
- Modify: `internal/mock/video_repo_mock.go`(mock 補實作)

**Interfaces:**
- Consumes: `model.ErrNotFound`(`internal/model/errors.go` 既有)、`*pgxpool.Pool`。
- Produces(後續 task 依賴的精確簽名):
  - `model.SegmentBoundary{Start, Duration float64}`、`model.KeyframeIndex{VideoID string; Segments []model.SegmentBoundary; ProbedAt time.Time}`
  - `repository.NewKeyframeIndexRepository(pool *pgxpool.Pool) *keyframeIndexRepository`,methods:`Get(ctx, videoID string) (*model.KeyframeIndex, error)`(無資料回 `model.ErrNotFound`)、`Upsert(ctx context.Context, idx *model.KeyframeIndex) error`
  - `(*videoRepository).ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error)`

- [ ] **Step 1: 寫 migration**

`migrations/018_create_video_keyframe_index.up.sql`:

```sql
CREATE TABLE video_keyframe_index (
    video_id  UUID PRIMARY KEY REFERENCES videos(id) ON DELETE CASCADE,
    segments  JSONB NOT NULL,
    probed_at TIMESTAMPTZ NOT NULL
);
```

`migrations/018_create_video_keyframe_index.down.sql`:

```sql
DROP TABLE video_keyframe_index;
```

- [ ] **Step 2: model types**

`internal/model/keyframe_index.go`:

```go
package model

import "time"

// SegmentBoundary 描述一個 HLS 分段在原始檔中的時間範圍(秒)。
// Start 必為 keyframe pts(首段為 0),Duration 依 keyframe 分布浮動。
type SegmentBoundary struct {
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
}

// KeyframeIndex 是一部影片的 keyframe 對齊分段邊界表。
type KeyframeIndex struct {
	VideoID  string
	Segments []SegmentBoundary
	ProbedAt time.Time
}
```

- [ ] **Step 3: keyframe index repository**

`internal/repository/keyframe_index_repo.go`(pool 欄位型別、import 分組比照 `video_repo.go`):

```go
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

const queryGetKeyframeIndex = `
	SELECT segments, probed_at
	FROM video_keyframe_index
	WHERE video_id = $1
`

const queryUpsertKeyframeIndex = `
	INSERT INTO video_keyframe_index (video_id, segments, probed_at)
	VALUES ($1, $2, $3)
	ON CONFLICT (video_id) DO UPDATE SET segments = EXCLUDED.segments, probed_at = EXCLUDED.probed_at
`

// keyframeIndexRepository 持久化 keyframe 分段邊界表。
type keyframeIndexRepository struct {
	pool *pgxpool.Pool
}

// NewKeyframeIndexRepository 建立 keyframe index repository。
func NewKeyframeIndexRepository(pool *pgxpool.Pool) *keyframeIndexRepository {
	return &keyframeIndexRepository{pool: pool}
}

// Get 回傳影片的邊界表;不存在回 model.ErrNotFound。
func (r *keyframeIndexRepository) Get(ctx context.Context, videoID string) (*model.KeyframeIndex, error) {
	idx := &model.KeyframeIndex{VideoID: videoID}
	var raw []byte
	err := r.pool.QueryRow(ctx, queryGetKeyframeIndex, videoID).Scan(&raw, &idx.ProbedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get keyframe index for video %s: %w", videoID, err)
	}
	if err := json.Unmarshal(raw, &idx.Segments); err != nil {
		return nil, fmt.Errorf("failed to decode keyframe segments for video %s: %w", videoID, err)
	}
	return idx, nil
}

// Upsert 寫入或覆蓋邊界表。
func (r *keyframeIndexRepository) Upsert(ctx context.Context, idx *model.KeyframeIndex) error {
	raw, err := json.Marshal(idx.Segments)
	if err != nil {
		return fmt.Errorf("failed to encode keyframe segments for video %s: %w", idx.VideoID, err)
	}
	if _, err := r.pool.Exec(ctx, queryUpsertKeyframeIndex, idx.VideoID, raw, idx.ProbedAt); err != nil {
		return fmt.Errorf("failed to upsert keyframe index for video %s: %w", idx.VideoID, err)
	}
	return nil
}
```

- [ ] **Step 4: backfill 候選查詢**

`internal/repository/video_keyframe_repo.go`(receiver 是既有 `*videoRepository`,比照 `video_codec_repo.go`;remux 判定交由 service 層 `ClassifyPlayMode`,SQL 只做粗篩):

```go
package repository

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

const queryListKeyframeCandidates = `
	SELECT v.id, v.original_filename, v.source_id, v.file_path, v.video_codec, v.audio_codec
	FROM videos v
	LEFT JOIN video_keyframe_index k ON k.video_id = v.id
	WHERE k.video_id IS NULL
	  AND v.video_codec IS NOT NULL AND v.video_codec <> ''
	  AND v.source_id IS NOT NULL AND v.file_path IS NOT NULL
	ORDER BY v.created_at ASC
	LIMIT $1
`

// ListKeyframeCandidates 回傳尚無 keyframe 邊界表且 codec 已知的影片
// (是否為 remux 由 service 層以 ClassifyPlayMode 判定)。
func (r *videoRepository) ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error) {
	rows, err := r.pool.Query(ctx, queryListKeyframeCandidates, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list keyframe candidates: %w", err)
	}
	defer rows.Close()

	var videos []model.Video
	for rows.Next() {
		var v model.Video
		if err := rows.Scan(&v.ID, &v.OriginalFilename, &v.SourceID, &v.FilePath, &v.VideoCodec, &v.AudioCodec); err != nil {
			return nil, fmt.Errorf("failed to scan keyframe candidate: %w", err)
		}
		videos = append(videos, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate keyframe candidates: %w", err)
	}
	if videos == nil {
		videos = []model.Video{}
	}
	return videos, nil
}
```

注意:若 `video_codec`/`audio_codec` 在 DB 為 nullable 而 `model.Video` 欄位是 `string`,Scan 需比照 `video_repo.go` 既有 nullable 欄位處理方式(如 `pgtype.Text` 或 `COALESCE(v.video_codec, '')`);以 `COALESCE` 最簡,若既有檔案用其他方式則跟隨。

- [ ] **Step 4b: interface 與 mock 同步**

`NewVideoRepository` 回傳的是 `VideoRepository` **interface**(`video_repo.go:126`),main 的 `videoRepo` 靜態型別是該 interface,因此新方法必須加進 interface 才能滿足 Task 6 的 `keyframeVideoRepo` consumer interface。

`internal/repository/video_repo.go` 的 `VideoRepository` interface(`ListMissingCodecs` 宣告旁)加:

```go
	// ListKeyframeCandidates returns videos with known codecs but no keyframe
	// index yet (remux filtering happens in the service layer).
	ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error)
```

`internal/mock/video_repo_mock.go` 比照 `ListMissingCodecs` 的 mock pattern 加:

```go
	ListKeyframeCandidatesFunc  func(ctx context.Context, limit int) ([]model.Video, error)
```

```go
func (m *VideoRepository) ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error) {
	if m.ListKeyframeCandidatesFunc == nil {
		return nil, fmt.Errorf("mock: ListKeyframeCandidatesFunc not set")
	}
	return m.ListKeyframeCandidatesFunc(ctx, limit)
}
```

- [ ] **Step 5: 編譯 + vet**

Run: `cd /home/user/Vaultflix && go build ./... && go vet ./...`
Expected: 無錯誤。

- [ ] **Step 6: Commit**

```bash
git add migrations/018_* internal/model/keyframe_index.go internal/repository/keyframe_index_repo.go internal/repository/video_keyframe_repo.go internal/repository/video_repo.go internal/mock/video_repo_mock.go
git commit -m "feat: add video_keyframe_index table, model and repositories"
```

---

### Task 2: Keyframe 探測與分段分組(streaming package,TDD)

**Files:**
- Create: `internal/streaming/keyframe_probe.go`
- Create: `internal/streaming/keyframe_probe_test.go`

**Interfaces:**
- Consumes: `model.SegmentBoundary`(Task 1)。
- Produces:
  - `streaming.DefaultSegmentTarget = 6.0`(const)
  - `streaming.ProbeKeyframes(ctx context.Context, absPath string) ([]float64, float64, error)` — 回傳 (keyframe pts 列表, 總長秒, error)
  - `streaming.GroupSegments(kfPts []float64, total, target float64) []model.SegmentBoundary`(純函式)

- [ ] **Step 1: 寫失敗測試**

`internal/streaming/keyframe_probe_test.go`:

```go
package streaming

import (
	"math"
	"testing"
)

func TestParseKeyframeProbe_MixedOutput(t *testing.T) {
	out := "packet,0.000000,K__\n" +
		"packet,0.033000,___\n" +
		"packet,N/A,K__\n" + // 無 pts 的 keyframe packet 跳過
		"packet,8.341000,K__\n" +
		"format,120.500000\n"
	kf, total, err := parseKeyframeProbe(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kf) != 2 || kf[0] != 0 || kf[1] != 8.341 {
		t.Errorf("kf = %v, want [0 8.341]", kf)
	}
	if total != 120.5 {
		t.Errorf("total = %v, want 120.5", total)
	}
}

func TestParseKeyframeProbe_NoKeyframes(t *testing.T) {
	if _, _, err := parseKeyframeProbe("format,10.0\n"); err == nil {
		t.Error("expected error for output without keyframes")
	}
}

func TestParseKeyframeProbe_NoDuration(t *testing.T) {
	if _, _, err := parseKeyframeProbe("packet,0.000000,K__\nformat,N/A\n"); err == nil {
		t.Error("expected error for output without duration")
	}
}

func TestGroupSegments_Boundaries(t *testing.T) {
	dense := make([]float64, 0, 100) // 每 1.2s 一個 keyframe
	for i := 0; i < 100; i++ {
		dense = append(dense, float64(i)*1.2)
	}
	tests := []struct {
		name      string
		kf        []float64
		total     float64
		wantCount int
		wantLast  float64 // 末段結尾 = Start+Duration 應等於 total
	}{
		{name: "sparse keyframes every 8s", kf: []float64{0, 8, 16, 24}, total: 30, wantCount: 4, wantLast: 30},
		{name: "dense keyframes group to ~6s", kf: dense, total: 118.8, wantCount: 20, wantLast: 118.8},
		{name: "tail shorter than 1s merges into previous", kf: []float64{0, 7, 14}, total: 14.5, wantCount: 2, wantLast: 14.5},
		{name: "single keyframe whole file", kf: []float64{0}, total: 42, wantCount: 1, wantLast: 42},
		{name: "first keyframe not at zero still starts at 0", kf: []float64{1.5, 9.0}, total: 20, wantCount: 2, wantLast: 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			segs := GroupSegments(tc.kf, tc.total, DefaultSegmentTarget)
			if len(segs) != tc.wantCount {
				t.Fatalf("count = %d, want %d (segs=%v)", len(segs), tc.wantCount, segs)
			}
			if segs[0].Start != 0 {
				t.Errorf("first start = %v, want 0", segs[0].Start)
			}
			last := segs[len(segs)-1]
			if math.Abs(last.Start+last.Duration-tc.wantLast) > 1e-9 {
				t.Errorf("last end = %v, want %v", last.Start+last.Duration, tc.wantLast)
			}
			for i := 1; i < len(segs); i++ {
				if math.Abs(segs[i].Start-(segs[i-1].Start+segs[i-1].Duration)) > 1e-9 {
					t.Errorf("gap between segment %d and %d", i-1, i)
				}
			}
		})
	}
}

func TestGroupSegments_ZeroTotal(t *testing.T) {
	if segs := GroupSegments([]float64{0}, 0, DefaultSegmentTarget); segs != nil {
		t.Errorf("expected nil for zero total, got %v", segs)
	}
}
```

dense case 的期望值推導:每 1.2s 一個 keyframe、target 6.0 → 每段跨 5 個 keyframe 間隔(第一個 pts−cur ≥ 6 的是 +6.0)→ 段長 6.0,total 118.8 → 19 段 6.0s = 114.0,尾巴 4.8s ≥ 1s 自成一段 → 共 20 段。

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/streaming/ -run 'TestParseKeyframeProbe|TestGroupSegments' -v`
Expected: FAIL(`parseKeyframeProbe`、`GroupSegments` 未定義)。

- [ ] **Step 3: 實作**

`internal/streaming/keyframe_probe.go`:

```go
package streaming

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// DefaultSegmentTarget 是分段目標長度(秒);實際段長依 keyframe 分布浮動。
const DefaultSegmentTarget = 6.0

// ProbeKeyframes 以 packet-level 掃描(不解碼)取得影片的 keyframe pts 與總長秒數。
// 冷讀成本約 15s/GB(I/O bound),呼叫端負責 timeout 與非同步化。
func ProbeKeyframes(ctx context.Context, absPath string) ([]float64, float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags:format=duration",
		"-of", "csv",
		absPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, 0, fmt.Errorf("ffprobe keyframe scan failed for %s: %w", absPath, err)
	}
	return parseKeyframeProbe(string(out))
}

// parseKeyframeProbe 解析 csv 輸出:`packet,<pts_time>,<flags>` 與 `format,<duration>`。
func parseKeyframeProbe(out string) ([]float64, float64, error) {
	var kf []float64
	var total float64
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		switch {
		case len(fields) >= 3 && fields[0] == "packet":
			if !strings.Contains(fields[2], "K") {
				continue
			}
			pts, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				continue // pts 為 N/A 的 packet 跳過
			}
			kf = append(kf, pts)
		case len(fields) >= 2 && fields[0] == "format":
			if d, err := strconv.ParseFloat(fields[1], 64); err == nil {
				total = d
			}
		}
	}
	if len(kf) == 0 {
		return nil, 0, fmt.Errorf("no keyframes found in probe output")
	}
	if total <= 0 {
		return nil, 0, fmt.Errorf("no valid duration in probe output")
	}
	return kf, total, nil
}

// GroupSegments 把 keyframe pts 依目標段長分組成連續無間隙的 VOD 分段邊界。
// 首段強制從 0 開始;每個內部邊界皆為 keyframe pts(-c copy 只能在 keyframe 切);
// 末段補到 total,尾巴不足 1s 時併入前一段。total <= 0 回 nil。
func GroupSegments(kfPts []float64, total, target float64) []model.SegmentBoundary {
	if total <= 0 {
		return nil
	}
	var segs []model.SegmentBoundary
	cur := 0.0
	for _, pts := range kfPts {
		if pts-cur >= target {
			segs = append(segs, model.SegmentBoundary{Start: cur, Duration: pts - cur})
			cur = pts
		}
	}
	if total-cur >= 1.0 || len(segs) == 0 {
		segs = append(segs, model.SegmentBoundary{Start: cur, Duration: total - cur})
	} else {
		last := &segs[len(segs)-1]
		last.Duration = total - last.Start
	}
	return segs
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/streaming/ -v`
Expected: 新測試 PASS(既有 manager/transcoder 測試也仍 PASS)。

- [ ] **Step 5: Commit**

```bash
git add internal/streaming/keyframe_probe.go internal/streaming/keyframe_probe_test.go
git commit -m "feat: add keyframe probe and segment grouping"
```

---

### Task 3: VOD manifest builder(TDD)

**Files:**
- Create: `internal/streaming/manifest.go`
- Create: `internal/streaming/manifest_test.go`

**Interfaces:**
- Consumes: `model.SegmentBoundary`。
- Produces:
  - `streaming.BuildVODManifest(segs []model.SegmentBoundary) []byte`
  - `streaming.SegmentName(i int) string` → `"seg00042.ts"` 格式(handler 與 cache 共用)

- [ ] **Step 1: 寫失敗測試**

`internal/streaming/manifest_test.go`:

```go
package streaming

import (
	"strings"
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func TestBuildVODManifest_Structure(t *testing.T) {
	segs := []model.SegmentBoundary{
		{Start: 0, Duration: 8.341},
		{Start: 8.341, Duration: 6.0},
		{Start: 14.341, Duration: 3.2},
	}
	m := string(BuildVODManifest(segs))

	for _, want := range []string{
		"#EXTM3U",
		"#EXT-X-VERSION:3",
		"#EXT-X-PLAYLIST-TYPE:VOD",
		"#EXT-X-TARGETDURATION:9", // ceil(8.341)
		"#EXT-X-MEDIA-SEQUENCE:0",
		"#EXTINF:8.341000,",
		"seg00000.ts",
		"seg00002.ts",
		"#EXT-X-ENDLIST",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest missing %q:\n%s", want, m)
		}
	}
	if strings.Count(m, "#EXTINF:") != 3 {
		t.Errorf("EXTINF count = %d, want 3", strings.Count(m, "#EXTINF:"))
	}
	// ENDLIST 必須是最後一個 directive
	if !strings.HasSuffix(strings.TrimSpace(m), "#EXT-X-ENDLIST") {
		t.Errorf("manifest does not end with ENDLIST:\n%s", m)
	}
}

func TestSegmentName_Format(t *testing.T) {
	if got := SegmentName(0); got != "seg00000.ts" {
		t.Errorf("SegmentName(0) = %q", got)
	}
	if got := SegmentName(42); got != "seg00042.ts" {
		t.Errorf("SegmentName(42) = %q", got)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/streaming/ -run 'TestBuildVODManifest|TestSegmentName' -v`
Expected: FAIL(未定義)。

- [ ] **Step 3: 實作**

`internal/streaming/manifest.go`:

```go
package streaming

import (
	"fmt"
	"math"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// SegmentName 回傳第 i 段的檔名(與 handler 的 segment regex、快取檔名一致)。
func SegmentName(i int) string {
	return fmt.Sprintf("seg%05d.ts", i)
}

// BuildVODManifest 由分段邊界組出完整 VOD m3u8(含 ENDLIST)。
// segment URI 為相對檔名,token 由 handler 的 rewritePlaylistTokens 事後附加。
func BuildVODManifest(segs []model.SegmentBoundary) []byte {
	maxDur := 0.0
	for _, s := range segs {
		if s.Duration > maxDur {
			maxDur = s.Duration
		}
	}
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-PLAYLIST-TYPE:VOD\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(maxDur)))
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")
	for i, s := range segs {
		fmt.Fprintf(&b, "#EXTINF:%.6f,\n%s\n", s.Duration, SegmentName(i))
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return []byte(b.String())
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/streaming/ -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/streaming/manifest.go internal/streaming/manifest_test.go
git commit -m "feat: add VOD manifest builder"
```

---

### Task 4: 單段 ffmpeg generator(TDD)

**Files:**
- Create: `internal/streaming/segment_generator.go`
- Create: `internal/streaming/segment_generator_test.go`

**Interfaces:**
- Produces:
  - `streaming.SegmentGenerator` interface:`Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error`
  - `streaming.NewFFmpegSegmentGenerator() *FFmpegSegmentGenerator`

(舊 `transcoder.go` 本 task **不動**,cutover 時才刪。)

- [ ] **Step 1: 寫失敗測試**

`internal/streaming/segment_generator_test.go`:

```go
package streaming

import (
	"strings"
	"testing"
)

func TestBuildSegmentArgs_Order(t *testing.T) {
	args := buildSegmentArgs("/mnt/host/D/x.avi", "/cache/v1/seg00003.ts", 25.023, 8.341)
	joined := strings.Join(args, " ")

	// input seek:-ss 必須在 -i 之前(靠容器索引直接跳,不線性讀)
	ssIdx := indexOf(args, "-ss")
	iIdx := indexOf(args, "-i")
	if ssIdx == -1 || iIdx == -1 || ssIdx > iIdx {
		t.Errorf("-ss must come before -i: %s", joined)
	}
	for _, want := range []string{
		"-ss 25.023000",
		"-t 8.341000",
		"-c copy",
		"-output_ts_offset 25.023000",
		"-muxdelay 0",
		"-muxpreload 0",
		"-f mpegts",
		"/cache/v1/seg00003.ts",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/streaming/ -run TestBuildSegmentArgs -v`
Expected: FAIL(未定義)。

- [ ] **Step 3: 實作**

`internal/streaming/segment_generator.go`:

```go
package streaming

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

// SegmentGenerator 產生單一 mpegts 分段檔。
// 失敗時回 error 且不保證 outPath 狀態;呼叫端以暫存檔 + rename 保證原子性。
type SegmentGenerator interface {
	// Generate 從 inputPath 的 start 秒起切出 duration 秒寫入 outPath。
	Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error
}

// FFmpegSegmentGenerator 以 ffmpeg -c copy 實作 SegmentGenerator。
type FFmpegSegmentGenerator struct{}

// NewFFmpegSegmentGenerator 建立 FFmpegSegmentGenerator。
func NewFFmpegSegmentGenerator() *FFmpegSegmentGenerator { return &FFmpegSegmentGenerator{} }

// buildSegmentArgs 產生單段 remux 的 ffmpeg 參數。
// -ss 在 -i 前(input seek;start 即 keyframe pts,落點精準)。
// -output_ts_offset 讓段內 PTS 對齊 manifest 位置(預設每段重置為 0 會讓 hls.js 對齊錯亂);
// 若特定容器 offset 行為異常,備案是改用 -copyts(見 spec Trade-offs)。
func buildSegmentArgs(inputPath, outPath string, start, duration float64) []string {
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-ss", formatSeconds(start),
		"-i", inputPath,
		"-t", formatSeconds(duration),
		"-c", "copy",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-output_ts_offset", formatSeconds(start),
		"-f", "mpegts",
		"-y", outPath,
	}
}

func formatSeconds(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

// Generate 執行 ffmpeg 產生分段;錯誤訊息附 stderr 尾段以利除錯。
func (g *FFmpegSegmentGenerator) Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", buildSegmentArgs(inputPath, outPath, start, duration)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg segment generate failed (start=%s): %w: %s",
			formatSeconds(start), err, tail(string(out), 500))
	}
	return nil
}

// tail 回傳字串最後 n 個 byte(錯誤訊息截斷用)。
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/streaming/ -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/streaming/segment_generator.go internal/streaming/segment_generator_test.go
git commit -m "feat: add on-demand ffmpeg segment generator"
```

---

### Task 5: SegmentCache(singleflight + idle sweep + LRU,TDD)

**Files:**
- Create: `internal/streaming/segment_cache.go`
- Create: `internal/streaming/segment_cache_test.go`

**Interfaces:**
- Consumes: `streaming.SegmentGenerator`(Task 4)、`streaming.SegmentName`(Task 3)、`sanitizeKey`(既有 `manager.go`;cutover 刪 manager 時把 `sanitizeKey` 搬進本檔)。
- Produces:
  - `streaming.NewSegmentCache(gen SegmentGenerator, cacheDir string, idleTimeout time.Duration, maxBytes int64) (*SegmentCache, error)` — 建構時清空 cacheDir 既有內容(重啟後冷快取,帳目與磁碟一致)
  - `(*SegmentCache).EnsureSegment(ctx context.Context, videoID, inputPath string, idx int, seg model.SegmentBoundary) (string, error)` — 回傳分段檔絕對路徑
  - `(*SegmentCache).Sweep(now time.Time)`、`(*SegmentCache).StartSweeper(ctx context.Context)`

**注意**:本 task 期間 `manager.go` 仍存在,`sanitizeKey` 直接沿用(同 package)。

- [ ] **Step 1: 寫失敗測試**

`internal/streaming/segment_cache_test.go`:

```go
package streaming

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

// fakeGenerator 寫入固定大小的假分段,並計數呼叫次數。
type fakeGenerator struct {
	calls atomic.Int64
	size  int
	delay time.Duration
	fail  bool
}

func (g *fakeGenerator) Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error {
	g.calls.Add(1)
	if g.delay > 0 {
		select {
		case <-time.After(g.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if g.fail {
		return os.ErrInvalid
	}
	return os.WriteFile(outPath, make([]byte, g.size), 0o644)
}

func newTestCache(t *testing.T, gen SegmentGenerator, maxBytes int64) *SegmentCache {
	t.Helper()
	c, err := NewSegmentCache(gen, t.TempDir(), time.Minute, maxBytes)
	if err != nil {
		t.Fatalf("NewSegmentCache: %v", err)
	}
	return c
}

var seg0 = model.SegmentBoundary{Start: 0, Duration: 6}

func TestEnsureSegment_CacheHit(t *testing.T) {
	gen := &fakeGenerator{size: 10}
	c := newTestCache(t, gen, 0)

	p1, err := c.EnsureSegment(context.Background(), "v1", "/in.avi", 0, seg0)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	p2, err := c.EnsureSegment(context.Background(), "v1", "/in.avi", 0, seg0)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if p1 != p2 {
		t.Errorf("paths differ: %s vs %s", p1, p2)
	}
	if got := gen.calls.Load(); got != 1 {
		t.Errorf("generator calls = %d, want 1 (cache hit)", got)
	}
}

func TestEnsureSegment_SingleflightConcurrent(t *testing.T) {
	gen := &fakeGenerator{size: 10, delay: 50 * time.Millisecond}
	c := newTestCache(t, gen, 0)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = c.EnsureSegment(context.Background(), "v1", "/in.avi", 3, model.SegmentBoundary{Start: 18, Duration: 6})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
	if got := gen.calls.Load(); got != 1 {
		t.Errorf("generator calls = %d, want 1 (singleflight)", got)
	}
}

func TestEnsureSegment_GeneratorFailure(t *testing.T) {
	gen := &fakeGenerator{fail: true}
	c := newTestCache(t, gen, 0)

	if _, err := c.EnsureSegment(context.Background(), "v1", "/in.avi", 0, seg0); err == nil {
		t.Fatal("expected error from failing generator")
	}
	// 失敗不得留下半成品分段檔
	entries, _ := os.ReadDir(filepath.Join(c.cacheDir, "v1"))
	for _, e := range entries {
		t.Errorf("unexpected leftover file: %s", e.Name())
	}
}

func TestEnsureSegment_LRUEviction(t *testing.T) {
	gen := &fakeGenerator{size: 100}
	c := newTestCache(t, gen, 250) // 容量只夠 2 段多一點

	ctx := context.Background()
	if _, err := c.EnsureSegment(ctx, "old", "/in.avi", 0, seg0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond) // 確保 lastAccess 有序
	if _, err := c.EnsureSegment(ctx, "newer", "/in.avi", 0, seg0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	// 第三段使總量 300 > 250 → 踢最久未存取的 "old" 整片目錄
	if _, err := c.EnsureSegment(ctx, "newer", "/in.avi", 1, model.SegmentBoundary{Start: 6, Duration: 6}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(c.cacheDir, "old")); !os.IsNotExist(err) {
		t.Error("old video dir should be evicted")
	}
	if _, err := os.Stat(filepath.Join(c.cacheDir, "newer")); err != nil {
		t.Error("newer video dir should survive eviction")
	}
}

func TestSweep_RemovesIdleVideos(t *testing.T) {
	gen := &fakeGenerator{size: 10}
	c := newTestCache(t, gen, 0) // idleTimeout = time.Minute

	if _, err := c.EnsureSegment(context.Background(), "v1", "/in.avi", 0, seg0); err != nil {
		t.Fatal(err)
	}
	c.Sweep(time.Now().Add(2 * time.Minute))
	if _, err := os.Stat(filepath.Join(c.cacheDir, "v1")); !os.IsNotExist(err) {
		t.Error("idle video dir should be swept")
	}
}

func TestNewSegmentCache_WipesLeftovers(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSegmentCache(&fakeGenerator{}, dir, time.Minute, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale")); !os.IsNotExist(err) {
		t.Error("leftover dir should be wiped on startup")
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/streaming/ -run 'TestEnsureSegment|TestSweep_|TestNewSegmentCache' -v`
Expected: FAIL(未定義)。

- [ ] **Step 3: 實作**

`internal/streaming/segment_cache.go`:

```go
package streaming

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

// segmentGenTimeout 是單段 ffmpeg 產生的逾時上限(-c copy 正常亞秒級,
// 逾時代表輸入檔或磁碟異常)。
const segmentGenTimeout = 60 * time.Second

// SegmentCache 管理 per-video 的 on-demand HLS 分段快取。
// 同段並發請求去重(單一 ffmpeg);清理雙軌:idle sweep(整片目錄閒置逾時)
// + LRU 容量上限(總量超過 maxBytes 踢最久未存取的整片目錄)。
type SegmentCache struct {
	gen         SegmentGenerator
	cacheDir    string
	idleTimeout time.Duration
	maxBytes    int64 // <=0 表示不設容量上限

	mu       sync.Mutex
	videos   map[string]*videoCacheState
	inflight map[string]chan struct{}
}

type videoCacheState struct {
	dir        string
	sizeBytes  int64
	lastAccess time.Time
}

// NewSegmentCache 建立 SegmentCache 並清空 cacheDir 既有內容
// (重啟後冷快取,讓記憶體內大小帳目與磁碟一致)。
func NewSegmentCache(gen SegmentGenerator, cacheDir string, idleTimeout time.Duration, maxBytes int64) (*SegmentCache, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache dir: %w", err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(cacheDir, e.Name())); err != nil {
			return nil, fmt.Errorf("failed to clear cache dir: %w", err)
		}
	}
	return &SegmentCache{
		gen:         gen,
		cacheDir:    cacheDir,
		idleTimeout: idleTimeout,
		maxBytes:    maxBytes,
		videos:      make(map[string]*videoCacheState),
		inflight:    make(map[string]chan struct{}),
	}, nil
}

// EnsureSegment 回傳分段檔路徑;未快取時產生。同段並發請求只跑一次 ffmpeg。
func (c *SegmentCache) EnsureSegment(ctx context.Context, videoID, inputPath string, idx int, seg model.SegmentBoundary) (string, error) {
	name := SegmentName(idx)
	key := videoID + "/" + name
	dir := filepath.Join(c.cacheDir, sanitizeKey(videoID))
	path := filepath.Join(dir, name)

	for {
		c.mu.Lock()
		if st, ok := c.videos[videoID]; ok {
			st.lastAccess = time.Now()
		}
		if _, err := os.Stat(path); err == nil {
			c.mu.Unlock()
			return path, nil
		}
		ch, busy := c.inflight[key]
		if !busy {
			ch = make(chan struct{})
			c.inflight[key] = ch
			c.mu.Unlock()
			break
		}
		c.mu.Unlock()
		select {
		case <-ch: // 產生者完成(成功或失敗),回圈重查
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	genPath, err := c.generate(ctx, videoID, inputPath, dir, path, seg)

	c.mu.Lock()
	close(c.inflight[key])
	delete(c.inflight, key)
	c.mu.Unlock()
	return genPath, err
}

// generate 以暫存檔 + rename 產生分段,更新帳目並觸發 LRU 淘汰。
func (c *SegmentCache) generate(ctx context.Context, videoID, inputPath, dir, path string, seg model.SegmentBoundary) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create video cache dir: %w", err)
	}
	tmp := path + ".tmp"
	genCtx, cancel := context.WithTimeout(ctx, segmentGenTimeout)
	defer cancel()
	if err := c.gen.Generate(genCtx, inputPath, tmp, seg.Start, seg.Duration); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("failed to generate segment: %w", err)
	}
	info, err := os.Stat(tmp)
	if err != nil {
		return "", fmt.Errorf("generated segment missing: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("failed to finalize segment: %w", err)
	}

	c.mu.Lock()
	st, ok := c.videos[videoID]
	if !ok {
		st = &videoCacheState{dir: dir}
		c.videos[videoID] = st
	}
	st.sizeBytes += info.Size()
	st.lastAccess = time.Now()
	c.evictLocked(videoID)
	c.mu.Unlock()
	return path, nil
}

// evictLocked 在總量超過 maxBytes 時踢除最久未存取的整片目錄(跳過 current)。
// 呼叫端須持有 c.mu。
func (c *SegmentCache) evictLocked(current string) {
	if c.maxBytes <= 0 {
		return
	}
	for c.totalLocked() > c.maxBytes {
		victim := ""
		var oldest time.Time
		for id, st := range c.videos {
			if id == current {
				continue
			}
			if victim == "" || st.lastAccess.Before(oldest) {
				victim, oldest = id, st.lastAccess
			}
		}
		if victim == "" {
			return
		}
		st := c.videos[victim]
		if err := os.RemoveAll(st.dir); err != nil {
			slog.Error("failed to evict segment cache dir", "dir", st.dir, "error", err)
		}
		delete(c.videos, victim)
		slog.Info("segment cache evicted video", "video_id", victim, "freed_bytes", st.sizeBytes)
	}
}

func (c *SegmentCache) totalLocked() int64 {
	var total int64
	for _, st := range c.videos {
		total += st.sizeBytes
	}
	return total
}

// Sweep 清理 lastAccess 早於 now-idleTimeout 的影片快取目錄。
func (c *SegmentCache) Sweep(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, st := range c.videos {
		if now.Sub(st.lastAccess) <= c.idleTimeout {
			continue
		}
		if err := os.RemoveAll(st.dir); err != nil {
			slog.Error("failed to remove idle cache dir", "dir", st.dir, "error", err)
		}
		delete(c.videos, id)
	}
}

// StartSweeper 每 idleTimeout/2 跑一次 Sweep,直到 ctx 取消。
func (c *SegmentCache) StartSweeper(ctx context.Context) {
	interval := c.idleTimeout / 2
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				c.Sweep(t)
			}
		}
	}()
}
```

(檔案約 190 行,含註解,守 300 行上限。`sanitizeKey` 本 task 沿用 `manager.go` 的既有定義,cutover 時搬入本檔。)

- [ ] **Step 4: 跑測試確認通過(含 -race)**

Run: `go test ./internal/streaming/ -race -v`
Expected: 全部 PASS,無 data race。

- [ ] **Step 5: Commit**

```bash
git add internal/streaming/segment_cache.go internal/streaming/segment_cache_test.go
git commit -m "feat: add per-video segment cache with singleflight, idle sweep and LRU cap"
```

---

### Task 6: KeyframeService(查詢 / 非同步探測 / backfill,TDD)

**Files:**
- Create: `internal/service/keyframe_service.go`
- Create: `internal/service/keyframe_service_test.go`

**Interfaces:**
- Consumes: Task 1 repo 簽名、`streaming.ProbeKeyframes`、`streaming.GroupSegments`、`streaming.DefaultSegmentTarget`、既有 `ClassifyPlayMode`(`internal/service/play_mode.go`)、`model.PlayModeRemux`。
- Produces:
  - `service.NewKeyframeService(repo keyframeIndexRepo, videoRepo keyframeVideoRepo, sourceRepo keyframeSourceRepo) *KeyframeService`
  - `(*KeyframeService).GetSegments(ctx context.Context, videoID string) ([]model.SegmentBoundary, error)` — 無邊界表回 `model.ErrNotFound`
  - `(*KeyframeService).TriggerProbe(videoID, absPath string)` — 非同步、per-video 去重
  - `(*KeyframeService).RunBackfill(ctx context.Context) (processed, failed int, err error)`

- [ ] **Step 1: 寫失敗測試**

`internal/service/keyframe_service_test.go`(手寫 fake,比照 `codec_backfill_service_test.go` 風格):

```go
package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

type fakeKeyframeRepo struct {
	mu      sync.Mutex
	stored  map[string]*model.KeyframeIndex
	getErr  error
	upserts int
}

func newFakeKeyframeRepo() *fakeKeyframeRepo {
	return &fakeKeyframeRepo{stored: make(map[string]*model.KeyframeIndex)}
}

func (f *fakeKeyframeRepo) Get(ctx context.Context, videoID string) (*model.KeyframeIndex, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	idx, ok := f.stored[videoID]
	if !ok {
		return nil, model.ErrNotFound
	}
	return idx, nil
}

func (f *fakeKeyframeRepo) Upsert(ctx context.Context, idx *model.KeyframeIndex) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts++
	f.stored[idx.VideoID] = idx
	return nil
}

type fakeKfVideoRepo struct{ videos []model.Video }

func (f *fakeKfVideoRepo) ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error) {
	return f.videos, nil
}

type fakeKfSourceRepo struct{ source *model.MediaSource }

func (f *fakeKfSourceRepo) FindByID(ctx context.Context, id string) (*model.MediaSource, error) {
	if f.source == nil {
		return nil, model.ErrNotFound
	}
	return f.source, nil
}

func strPtr(s string) *string { return &s }

func TestGetSegments_NotFoundPassthrough(t *testing.T) {
	s := NewKeyframeService(newFakeKeyframeRepo(), &fakeKfVideoRepo{}, &fakeKfSourceRepo{})
	_, err := s.GetSegments(context.Background(), "missing")
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGetSegments_ReturnsStored(t *testing.T) {
	repo := newFakeKeyframeRepo()
	repo.stored["v1"] = &model.KeyframeIndex{
		VideoID:  "v1",
		Segments: []model.SegmentBoundary{{Start: 0, Duration: 8}},
	}
	s := NewKeyframeService(repo, &fakeKfVideoRepo{}, &fakeKfSourceRepo{})
	segs, err := s.GetSegments(context.Background(), "v1")
	if err != nil || len(segs) != 1 {
		t.Errorf("segs = %v, err = %v", segs, err)
	}
}

func TestTriggerProbe_DedupesConcurrentTriggers(t *testing.T) {
	repo := newFakeKeyframeRepo()
	s := NewKeyframeService(repo, &fakeKfVideoRepo{}, &fakeKfSourceRepo{})

	probeStarted := make(chan struct{})
	probeRelease := make(chan struct{})
	var probeCalls int
	var mu sync.Mutex
	s.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		mu.Lock()
		probeCalls++
		mu.Unlock()
		close(probeStarted)
		<-probeRelease
		return []float64{0, 8}, 16, nil
	}

	s.TriggerProbe("v1", "/in.avi")
	<-probeStarted
	s.TriggerProbe("v1", "/in.avi") // 探測進行中的重複觸發應被去重
	close(probeRelease)

	deadline := time.After(2 * time.Second)
	for {
		repo.mu.Lock()
		done := repo.upserts > 0
		repo.mu.Unlock()
		if done {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for probe result upsert")
		case <-time.After(10 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if probeCalls != 1 {
		t.Errorf("probe calls = %d, want 1 (dedupe)", probeCalls)
	}
	if repo.upserts != 1 {
		t.Errorf("upserts = %d, want 1", repo.upserts)
	}
}

func TestRunBackfill_FiltersNonRemuxAndCounts(t *testing.T) {
	repo := newFakeKeyframeRepo()
	videoRepo := &fakeKfVideoRepo{videos: []model.Video{
		{ID: "remux1", OriginalFilename: "a.avi", VideoCodec: "h264", AudioCodec: "aac",
			SourceID: strPtr("s1"), FilePath: strPtr("a.avi")},
		{ID: "direct1", OriginalFilename: "b.mp4", VideoCodec: "h264", AudioCodec: "aac",
			SourceID: strPtr("s1"), FilePath: strPtr("b.mp4")}, // direct → 跳過
		{ID: "transcode1", OriginalFilename: "c.wmv", VideoCodec: "wmv3", AudioCodec: "wmav2",
			SourceID: strPtr("s1"), FilePath: strPtr("c.wmv")}, // transcode → 跳過
	}}
	s := NewKeyframeService(repo, videoRepo, &fakeKfSourceRepo{
		source: &model.MediaSource{ID: "s1", MountPath: "/mnt/host/D"},
	})
	s.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		return []float64{0, 8}, 16, nil
	}

	processed, failed, err := s.RunBackfill(context.Background())
	if err != nil {
		t.Fatalf("RunBackfill: %v", err)
	}
	if processed != 1 || failed != 0 {
		t.Errorf("processed=%d failed=%d, want 1/0", processed, failed)
	}
	if _, ok := repo.stored["remux1"]; !ok {
		t.Error("remux1 index not stored")
	}
}

func TestRunBackfill_ProbeFailureCountsFailed(t *testing.T) {
	repo := newFakeKeyframeRepo()
	videoRepo := &fakeKfVideoRepo{videos: []model.Video{
		{ID: "remux1", OriginalFilename: "a.avi", VideoCodec: "h264", AudioCodec: "aac",
			SourceID: strPtr("s1"), FilePath: strPtr("a.avi")},
	}}
	s := NewKeyframeService(repo, videoRepo, &fakeKfSourceRepo{
		source: &model.MediaSource{ID: "s1", MountPath: "/mnt/host/D"},
	})
	s.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		return nil, 0, errors.New("boom")
	}

	processed, failed, err := s.RunBackfill(context.Background())
	if err != nil {
		t.Fatalf("RunBackfill: %v", err)
	}
	if processed != 0 || failed != 1 {
		t.Errorf("processed=%d failed=%d, want 0/1", processed, failed)
	}
}
```

(若 `model.MediaSource` 的欄位名非 `MountPath`,以 `internal/model` 實際定義為準修正 fake;`codec_backfill_service.go` 用的是 `source.MountPath`。)

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/service/ -run 'TestGetSegments|TestTriggerProbe|TestRunBackfill' -v`
Expected: FAIL(未定義)。

- [ ] **Step 3: 實作**

`internal/service/keyframe_service.go`:

```go
package service

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/streaming"
)

// probeTimeout 是單片 keyframe 探測上限(冷讀約 15s/GB,6GB 檔約 90s,留足裕度)。
const probeTimeout = 30 * time.Minute

// keyframeProbeFunc 回傳 (keyframe pts 列表, 總長秒數, error)。
type keyframeProbeFunc func(ctx context.Context, absPath string) ([]float64, float64, error)

// keyframeIndexRepo 是 KeyframeService 所需的邊界表存取子集。
type keyframeIndexRepo interface {
	// Get 不存在時回 model.ErrNotFound。
	Get(ctx context.Context, videoID string) (*model.KeyframeIndex, error)
	Upsert(ctx context.Context, idx *model.KeyframeIndex) error
}

// keyframeVideoRepo 是 KeyframeService 所需的 video 查詢子集。
type keyframeVideoRepo interface {
	ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error)
}

// keyframeSourceRepo 是 KeyframeService 所需的 media source 查詢子集。
type keyframeSourceRepo interface {
	FindByID(ctx context.Context, id string) (*model.MediaSource, error)
}

// KeyframeService 提供邊界表查詢、非同步探測(去重)與 backfill。
type KeyframeService struct {
	repo       keyframeIndexRepo
	videoRepo  keyframeVideoRepo
	sourceRepo keyframeSourceRepo
	probe      keyframeProbeFunc

	mu       sync.Mutex
	inflight map[string]struct{}
}

// NewKeyframeService 建立 KeyframeService(使用真實 ffprobe 探測)。
func NewKeyframeService(repo keyframeIndexRepo, videoRepo keyframeVideoRepo, sourceRepo keyframeSourceRepo) *KeyframeService {
	return &KeyframeService{
		repo:       repo,
		videoRepo:  videoRepo,
		sourceRepo: sourceRepo,
		probe:      streaming.ProbeKeyframes,
		inflight:   make(map[string]struct{}),
	}
}

// GetSegments 回傳邊界表;無資料回 model.ErrNotFound(呼叫端可觸發 TriggerProbe)。
func (s *KeyframeService) GetSegments(ctx context.Context, videoID string) ([]model.SegmentBoundary, error) {
	idx, err := s.repo.Get(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return idx.Segments, nil
}

// TriggerProbe 非同步探測 absPath 並寫入邊界表;同片探測進行中時重複觸發為 no-op。
func (s *KeyframeService) TriggerProbe(videoID, absPath string) {
	s.mu.Lock()
	if _, busy := s.inflight[videoID]; busy {
		s.mu.Unlock()
		return
	}
	s.inflight[videoID] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.inflight, videoID)
			s.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		defer cancel()
		if err := s.probeAndStore(ctx, videoID, absPath); err != nil {
			slog.Warn("keyframe probe failed", "video_id", videoID, "error", err)
		}
	}()
}

// probeAndStore 探測、分組並寫入邊界表。
func (s *KeyframeService) probeAndStore(ctx context.Context, videoID, absPath string) error {
	start := time.Now()
	kf, total, err := s.probe(ctx, absPath)
	if err != nil {
		return fmt.Errorf("probe failed: %w", err)
	}
	segs := streaming.GroupSegments(kf, total, streaming.DefaultSegmentTarget)
	if len(segs) == 0 {
		return fmt.Errorf("empty segment table for video %s", videoID)
	}
	idx := &model.KeyframeIndex{VideoID: videoID, Segments: segs, ProbedAt: time.Now()}
	if err := s.repo.Upsert(ctx, idx); err != nil {
		return fmt.Errorf("failed to store keyframe index: %w", err)
	}
	slog.Info("keyframe index stored",
		"video_id", videoID,
		"segments", len(segs),
		"elapsed_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

// RunBackfill 掃描缺邊界表的 remux 影片並循序探測,回傳 (processed, failed)。
func (s *KeyframeService) RunBackfill(ctx context.Context) (int, int, error) {
	videos, err := s.videoRepo.ListKeyframeCandidates(ctx, 10000)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list keyframe candidates: %w", err)
	}
	processed, failed := 0, 0
	for _, v := range videos {
		container := strings.TrimPrefix(filepath.Ext(v.OriginalFilename), ".")
		if ClassifyPlayMode(container, v.VideoCodec, v.AudioCodec) != model.PlayModeRemux {
			continue
		}
		source, err := s.sourceRepo.FindByID(ctx, *v.SourceID)
		if err != nil {
			slog.Warn("keyframe backfill: source lookup failed", "video_id", v.ID, "error", err)
			failed++
			continue
		}
		abs := filepath.Clean(filepath.Join(source.MountPath, *v.FilePath))
		if err := s.probeAndStore(ctx, v.ID, abs); err != nil {
			slog.Warn("keyframe backfill: probe failed", "video_id", v.ID, "error", err)
			failed++
			continue
		}
		processed++
	}
	slog.Info("keyframe backfill complete", "processed", processed, "failed", failed)
	return processed, failed, nil
}
```

- [ ] **Step 4: 跑測試確認通過(含 -race)**

Run: `go test ./internal/service/ -race -run 'TestGetSegments|TestTriggerProbe|TestRunBackfill' -v`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/service/keyframe_service.go internal/service/keyframe_service_test.go
git commit -m "feat: add keyframe service with async probe dedupe and backfill"
```

---

### Task 7: Cutover — HLS handler 重寫 + main 佈線 + 刪除 event 路徑

這是唯一「不可拆」的大 task:handler 簽名改變會牽動 main.go,舊 Manager 刪除也牽動 main.go,必須一氣呵成保持可編譯。

**Files:**
- Modify: `internal/handler/hls_handler.go`(重寫 Playlist/Segment;保留 `rewritePlaylistTokens`、`writePathError`;刪 `waitForFile`)
- Modify: `internal/handler/hls_handler_test.go`(保留既有 3 個測試,新增 fake-based 測試)
- Delete: `internal/streaming/manager.go`、`internal/streaming/manager_test.go`
- Modify: `internal/streaming/transcoder.go` → 刪除整檔(`Transcoder`/`TranscodeProc`/`FFmpegTranscoder`/`buildRemuxHLSArgs`/`PlaylistName`/`segmentPattern`),`transcoder_test.go` 一併刪
- Modify: `internal/streaming/segment_cache.go`(把 `manager.go` 的 `sanitizeKey` 搬進來)
- Modify: `internal/config/config.go`(新增 `TranscodeCacheMaxBytes`)
- Modify: `cmd/server/main.go:170-173` 佈線區
- Modify: `.env.example`(補 `TRANSCODE_CACHE_MAX_BYTES` 註解行)

**Interfaces:**
- Consumes: Task 3/5/6 全部 Produces;既有 `service.VideoService.ResolveDiskPath(ctx, videoID) (string, error)`。
- Produces:
  - handler 端 consumer interfaces(定義在 `hls_handler.go`):

```go
// diskPathResolver 解析影片在容器內的絕對路徑(由 *service.VideoService 實作)。
type diskPathResolver interface {
	ResolveDiskPath(ctx context.Context, videoID string) (string, error)
}

// keyframeProvider 提供邊界表查詢與非同步探測(由 *service.KeyframeService 實作)。
type keyframeProvider interface {
	// GetSegments 無邊界表時回 model.ErrNotFound。
	GetSegments(ctx context.Context, videoID string) ([]model.SegmentBoundary, error)
	TriggerProbe(videoID, absPath string)
}

// segmentEnsurer 確保分段存在並回傳檔案路徑(由 *streaming.SegmentCache 實作)。
type segmentEnsurer interface {
	EnsureSegment(ctx context.Context, videoID, inputPath string, idx int, seg model.SegmentBoundary) (string, error)
}
```

  - `handler.NewHLSHandler(videoSvc diskPathResolver, kf keyframeProvider, segs segmentEnsurer) *HLSHandler`
  - `config.Config.TranscodeCacheMaxBytes int64`(env `TRANSCODE_CACHE_MAX_BYTES`,預設 `21474836480` = 20GiB)

- [ ] **Step 1: 新增 handler 測試(先寫,會編譯失敗 = 失敗測試)**

在 `internal/handler/hls_handler_test.go` 保留既有 `TestHLSSegment_RejectsInvalidName`、`TestHLSSegment_RejectsNonSegmentName`、`TestRewritePlaylistTokens` 與 `splitLines`,新增:

```go
// --- fakes ---

type fakeResolver struct {
	path string
	err  error
}

func (f *fakeResolver) ResolveDiskPath(ctx context.Context, videoID string) (string, error) {
	return f.path, f.err
}

type fakeKeyframes struct {
	segs      []model.SegmentBoundary
	err       error
	triggered atomic.Int64
}

func (f *fakeKeyframes) GetSegments(ctx context.Context, videoID string) ([]model.SegmentBoundary, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.segs, nil
}

func (f *fakeKeyframes) TriggerProbe(videoID, absPath string) { f.triggered.Add(1) }

type fakeEnsurer struct {
	path string
	err  error
}

func (f *fakeEnsurer) EnsureSegment(ctx context.Context, videoID, inputPath string, idx int, seg model.SegmentBoundary) (string, error) {
	return f.path, f.err
}

func newHLSTestRouter(h *HLSHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/videos/:id/hls/index.m3u8", h.Playlist)
	r.GET("/api/videos/:id/hls/:segment", h.Segment)
	return r
}

func TestHLSPlaylist_ReturnsVODManifestWithTokens(t *testing.T) {
	kf := &fakeKeyframes{segs: []model.SegmentBoundary{
		{Start: 0, Duration: 8.341}, {Start: 8.341, Duration: 6.0},
	}}
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, kf, &fakeEnsurer{})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/index.m3u8?token=tok1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"#EXT-X-PLAYLIST-TYPE:VOD", "#EXT-X-ENDLIST", "seg00000.ts?token=tok1", "seg00001.ts?token=tok1"} {
		if !strings.Contains(body, want) {
			t.Errorf("playlist missing %q:\n%s", want, body)
		}
	}
}

func TestHLSPlaylist_NoIndexReturns503AndTriggersProbe(t *testing.T) {
	kf := &fakeKeyframes{err: model.ErrNotFound}
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, kf, &fakeEnsurer{})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/index.m3u8?token=t", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), "stream_not_ready") {
		t.Errorf("body missing stream_not_ready: %s", w.Body.String())
	}
	if kf.triggered.Load() != 1 {
		t.Errorf("probe triggered = %d, want 1", kf.triggered.Load())
	}
}

func TestHLSPlaylist_VideoNotFound(t *testing.T) {
	h := NewHLSHandler(&fakeResolver{err: model.ErrNotFound}, &fakeKeyframes{}, &fakeEnsurer{})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/nope/hls/index.m3u8", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHLSSegment_ServesGeneratedFile(t *testing.T) {
	segFile := filepath.Join(t.TempDir(), "seg00001.ts")
	if err := os.WriteFile(segFile, []byte("tsdata"), 0o644); err != nil {
		t.Fatal(err)
	}
	kf := &fakeKeyframes{segs: []model.SegmentBoundary{
		{Start: 0, Duration: 6}, {Start: 6, Duration: 6},
	}}
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, kf, &fakeEnsurer{path: segFile})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/seg00001.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "video/mp2t" {
		t.Errorf("content-type = %q, want video/mp2t", ct)
	}
	if w.Body.String() != "tsdata" {
		t.Errorf("body = %q, want tsdata", w.Body.String())
	}
}

func TestHLSSegment_IndexOutOfRange(t *testing.T) {
	kf := &fakeKeyframes{segs: []model.SegmentBoundary{{Start: 0, Duration: 6}}}
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, kf, &fakeEnsurer{})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/seg00007.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHLSSegment_NoIndexReturns404(t *testing.T) {
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, &fakeKeyframes{err: model.ErrNotFound}, &fakeEnsurer{})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/seg00000.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHLSSegment_EnsureFailureReturns500(t *testing.T) {
	kf := &fakeKeyframes{segs: []model.SegmentBoundary{{Start: 0, Duration: 6}}}
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, kf, &fakeEnsurer{err: errors.New("ffmpeg exploded")})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/seg00000.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
```

補 import:`context`, `errors`, `os`, `path/filepath`, `strings`, `sync/atomic`, `github.com/steven/vaultflix/internal/model`。
既有 `TestHLSSegment_RejectsInvalidName` 等用 `&HLSHandler{}` 零值 —— 新 handler 的 regex 檢查仍在依賴之前,零值可繼續用,不必改。

- [ ] **Step 2: 確認編譯失敗**

Run: `go build ./internal/handler/`
Expected: FAIL(`NewHLSHandler` 簽名不符、fake 型別未滿足)。

- [ ] **Step 3: 重寫 handler**

`internal/handler/hls_handler.go` 全檔重寫:

```go
package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/streaming"
)

var segmentNameRe = regexp.MustCompile(`^seg(\d{5})\.ts$`)

// diskPathResolver 解析影片在容器內的絕對路徑(由 *service.VideoService 實作)。
type diskPathResolver interface {
	ResolveDiskPath(ctx context.Context, videoID string) (string, error)
}

// keyframeProvider 提供邊界表查詢與非同步探測(由 *service.KeyframeService 實作)。
type keyframeProvider interface {
	// GetSegments 無邊界表時回 model.ErrNotFound。
	GetSegments(ctx context.Context, videoID string) ([]model.SegmentBoundary, error)
	TriggerProbe(videoID, absPath string)
}

// segmentEnsurer 確保分段存在並回傳檔案路徑(由 *streaming.SegmentCache 實作)。
type segmentEnsurer interface {
	EnsureSegment(ctx context.Context, videoID, inputPath string, idx int, seg model.SegmentBoundary) (string, error)
}

// HLSHandler 服務 VOD-on-the-fly 的 HLS manifest 與 on-demand 分段。
type HLSHandler struct {
	videoService diskPathResolver
	keyframes    keyframeProvider
	segments     segmentEnsurer
}

// NewHLSHandler 建立 HLSHandler。
func NewHLSHandler(videoSvc diskPathResolver, kf keyframeProvider, segs segmentEnsurer) *HLSHandler {
	return &HLSHandler{videoService: videoSvc, keyframes: kf, segments: segs}
}

// Playlist 由邊界表組出完整 VOD manifest(segment URI 內嵌 token)。
// 無邊界表時回 503 stream_not_ready 並觸發背景探測(首播 lazy fallback)。
// GET /api/videos/:id/hls/index.m3u8
func (h *HLSHandler) Playlist(c *gin.Context) {
	ctx := c.Request.Context()
	videoID := c.Param("id")
	token := c.Query("token")

	inputPath, err := h.videoService.ResolveDiskPath(ctx, videoID)
	if err != nil {
		h.writePathError(c, videoID, err)
		return
	}

	segs, err := h.keyframes.GetSegments(ctx, videoID)
	if errors.Is(err, model.ErrNotFound) {
		h.keyframes.TriggerProbe(videoID, inputPath)
		c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{
			Error:   "stream_not_ready",
			Message: "preparing stream for first playback, please retry",
		})
		return
	}
	if err != nil {
		slog.Error("failed to load keyframe index", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to load stream index",
		})
		return
	}

	manifest := streaming.BuildVODManifest(segs)
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl", rewritePlaylistTokens(manifest, token))
}

// Segment 確保並回傳指定分段(on-demand 產生 + 快取)。
// GET /api/videos/:id/hls/:segment
func (h *HLSHandler) Segment(c *gin.Context) {
	ctx := c.Request.Context()
	videoID := c.Param("id")
	segName := c.Param("segment")

	m := segmentNameRe.FindStringSubmatch(segName)
	if m == nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "invalid segment name",
		})
		return
	}
	idx, err := strconv.Atoi(m[1])
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "invalid segment index",
		})
		return
	}

	inputPath, err := h.videoService.ResolveDiskPath(ctx, videoID)
	if err != nil {
		h.writePathError(c, videoID, err)
		return
	}

	segs, err := h.keyframes.GetSegments(ctx, videoID)
	if errors.Is(err, model.ErrNotFound) {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error:   "not_found",
			Message: "no stream index for video",
		})
		return
	}
	if err != nil {
		slog.Error("failed to load keyframe index", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to load stream index",
		})
		return
	}
	if idx >= len(segs) {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error:   "not_found",
			Message: "segment out of range",
		})
		return
	}

	path, err := h.segments.EnsureSegment(ctx, videoID, inputPath, idx, segs[idx])
	if err != nil {
		slog.Error("failed to ensure segment", "error", err, "video_id", videoID, "segment", segName)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to generate segment",
		})
		return
	}

	c.Header("Content-Type", "video/mp2t")
	c.Header("Cache-Control", "no-cache")
	c.File(path)
}
```

`rewritePlaylistTokens` 與 `writePathError` 原樣保留(從舊檔複製,不改動);`waitForFile`、舊 import(`bufio` 以外仍被 `rewritePlaylistTokens` 用到的保留:`bufio`/`bytes`/`net/url`/`os`/`path/filepath`/`strings`/`time` 依實際使用裁剪,`gofmt`+`go vet` 會抓)。

- [ ] **Step 4: 刪除舊 event 路徑**

```bash
git rm internal/streaming/manager.go internal/streaming/manager_test.go internal/streaming/transcoder.go internal/streaming/transcoder_test.go
```

把 `sanitizeKey`(原 `manager.go:134-145`)搬到 `segment_cache.go` 底部:

```go
// sanitizeKey 把 videoID 變成安全的單層目錄名(避免路徑穿越)。
func sanitizeKey(key string) string {
	out := make([]rune, 0, len(key))
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
```

- [ ] **Step 5: config 新增容量上限**

`internal/config/config.go`:在 `TranscodeCacheDir` 欄位後加:

```go
	// TranscodeCacheMaxBytes bounds the on-demand segment cache size; the
	// least-recently-accessed video dir is evicted when the total exceeds it.
	// <=0 disables the cap (idle sweep still applies).
	TranscodeCacheMaxBytes int64
```

在 Load 的對應位置加:

```go
		TranscodeCacheMaxBytes: getEnvInt64("TRANSCODE_CACHE_MAX_BYTES", 20*1024*1024*1024),
```

檔尾比照 `getEnvInt` 加 helper:

```go
func getEnvInt64(key string, fallback int64) int64 {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}
```

(若既有 `getEnvInt` 的實作風格不同 —— 例如印 warning —— 跟隨其風格。)

`.env.example` 在 TRANSCODE 相關區塊(或檔尾)加:

```
# On-demand HLS segment cache size cap in bytes (default 20GiB). <=0 disables.
TRANSCODE_CACHE_MAX_BYTES=21474836480
```

- [ ] **Step 6: main.go 佈線**

`cmd/server/main.go:170-173` 改為:

```go
	segmentGen := streaming.NewFFmpegSegmentGenerator()
	segmentCache, err := streaming.NewSegmentCache(segmentGen, cfg.TranscodeCacheDir, 60*time.Second, cfg.TranscodeCacheMaxBytes)
	if err != nil {
		slog.Error("failed to init segment cache", "error", err)
		os.Exit(1)
	}
	segmentCache.StartSweeper(ctx)

	keyframeIndexRepo := repository.NewKeyframeIndexRepository(pool)
	keyframeService := service.NewKeyframeService(keyframeIndexRepo, videoRepo, mediaSourceRepo)
	hlsHandler := handler.NewHLSHandler(videoService, keyframeService, segmentCache)
```

(fatal 錯誤處理比照 main.go 既有 pattern —— 若其他初始化失敗是 `log.Fatalf` 就跟隨;`pool` 變數名以 main.go 實際為準。)

- [ ] **Step 7: 全量測試 + verify**

Run: `go build ./... && go test ./... && task verify`
Expected: 全綠。舊 manager/transcoder 測試已隨檔案刪除;`ListKeyframeCandidates` 已在 Task 1 同步進 interface 與 mock,無殘留編譯錯誤。

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat: serve VOD manifest with on-demand segments, replace event-remux session"
```

---

### Task 8: Backfill 端點 + import 後自動探測

**Files:**
- Create: `internal/handler/keyframe_backfill_handler.go`
- Create: `internal/handler/keyframe_backfill_handler_test.go`
- Modify: `internal/service/import_service.go`(掛 prober hook)
- Modify: `internal/service/import_service_test.go`(補 hook 測試;若無既有測試檔則新建只含此測試)
- Modify: `cmd/server/main.go`(handler 建構 + route + `SetKeyframeProber`)

**Interfaces:**
- Consumes: `(*KeyframeService).RunBackfill`、`(*KeyframeService).TriggerProbe`(Task 6)、`ClassifyPlayMode`。
- Produces:
  - `handler.NewKeyframeBackfillHandler(svc keyframeBackfiller) *KeyframeBackfillHandler`,`keyframeBackfiller interface { RunBackfill(ctx context.Context) (int, int, error) }`
  - Route:`POST /api/admin/videos/backfill-keyframes`(admin casbin wildcard 已涵蓋,不需改 policy.csv)
  - `(*ImportService).SetKeyframeProber(p keyframeProber)`,`keyframeProber interface { TriggerProbe(videoID, absPath string) }`

- [ ] **Step 1: handler 測試(先寫)**

`internal/handler/keyframe_backfill_handler_test.go`:

```go
package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeKeyframeBackfiller struct {
	processed, failed int
	err               error
}

func (f *fakeKeyframeBackfiller) RunBackfill(ctx context.Context) (int, int, error) {
	return f.processed, f.failed, f.err
}

func TestKeyframeBackfillRun_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewKeyframeBackfillHandler(&fakeKeyframeBackfiller{processed: 3, failed: 1})
	r := gin.New()
	r.POST("/admin/videos/backfill-keyframes", h.Run)

	req := httptest.NewRequest(http.MethodPost, "/admin/videos/backfill-keyframes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"processed":3`) || !strings.Contains(w.Body.String(), `"failed":1`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestKeyframeBackfillRun_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewKeyframeBackfillHandler(&fakeKeyframeBackfiller{err: errors.New("db down")})
	r := gin.New()
	r.POST("/admin/videos/backfill-keyframes", h.Run)

	req := httptest.NewRequest(http.MethodPost, "/admin/videos/backfill-keyframes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/handler/ -run TestKeyframeBackfill -v`
Expected: FAIL(未定義)。

- [ ] **Step 3: handler 實作**

`internal/handler/keyframe_backfill_handler.go`(比照 `codec_backfill_handler.go`,但依賴用 consumer-side interface 以利測試):

```go
package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

// keyframeBackfiller 執行 keyframe 邊界表 backfill(由 *service.KeyframeService 實作)。
type keyframeBackfiller interface {
	RunBackfill(ctx context.Context) (int, int, error)
}

// KeyframeBackfillHandler handles admin-triggered keyframe-index backfill requests.
type KeyframeBackfillHandler struct {
	svc keyframeBackfiller
}

// NewKeyframeBackfillHandler 建立 KeyframeBackfillHandler。
func NewKeyframeBackfillHandler(svc keyframeBackfiller) *KeyframeBackfillHandler {
	return &KeyframeBackfillHandler{svc: svc}
}

// Run 同步 backfill 所有缺 keyframe 邊界表的 remux 影片(admin only)。
// 注意:全量約 85 分鐘 I/O,呼叫端(curl/Admin UI)自行決定 timeout。
func (h *KeyframeBackfillHandler) Run(c *gin.Context) {
	processed, failed, err := h.svc.RunBackfill(c.Request.Context())
	if err != nil {
		slog.Error("keyframe backfill failed", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "keyframe backfill failed",
		})
		return
	}
	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{"processed": processed, "failed": failed},
	})
}
```

- [ ] **Step 4: import hook**

`internal/service/import_service.go`:

1. struct 加欄位(在 `ImportService` 定義處):

```go
	keyframes keyframeProber // optional;nil 時不觸發
```

2. 型別 + setter(放在 `NewImportService` 後):

```go
// keyframeProber 觸發影片的非同步 keyframe 探測(由 *KeyframeService 實作)。
type keyframeProber interface {
	TriggerProbe(videoID, absPath string)
}

// SetKeyframeProber 注入 keyframe prober(比照 VideoService.SetUserServices 的後注入模式)。
func (s *ImportService) SetKeyframeProber(p keyframeProber) {
	s.keyframes = p
}
```

3. `processOneFile` 內,`s.videoRepo.Create(ctx, video)` 成功後、`slog.Info("video imported", ...)` 之前加:

```go
	if s.keyframes != nil {
		ext := strings.TrimPrefix(filepath.Ext(filename), ".")
		if ClassifyPlayMode(ext, metadata.videoCodec, metadata.audioCodec) == model.PlayModeRemux {
			s.keyframes.TriggerProbe(videoID, filePath)
		}
	}
```

4. 測試(加入 `import_service_test.go`;fake prober 記錄呼叫)——測 `processOneFile` 層級若既有測試已 mock 其依賴則沿用其 harness;若既有測試只測其他層級,新增最小單元測試直接驗證 hook 條件邏輯(把 hook 抽成小函式亦可):

```go
func TestImportKeyframeHook_TriggersOnlyForRemux(t *testing.T) {
	// 驗證條件:avi/h264/aac → 觸發;mp4/h264/aac → 不觸發。
	// 若 processOneFile 難以單測,把步驟 3 的判斷抽成:
	//   func shouldProbeKeyframes(ext, videoCodec, audioCodec string) bool {
	//       return ClassifyPlayMode(ext, videoCodec, audioCodec) == model.PlayModeRemux
	//   }
	// 並以 table-driven 測 shouldProbeKeyframes。
	tests := []struct {
		ext, vc, ac string
		want        bool
	}{
		{"avi", "h264", "aac", true},
		{"mkv", "h264", "mp3", true},
		{"mp4", "h264", "aac", false},
		{"wmv", "wmv3", "wmav2", false},
	}
	for _, tc := range tests {
		if got := shouldProbeKeyframes(tc.ext, tc.vc, tc.ac); got != tc.want {
			t.Errorf("shouldProbeKeyframes(%s,%s,%s) = %v, want %v", tc.ext, tc.vc, tc.ac, got, tc.want)
		}
	}
}
```

採用抽出 `shouldProbeKeyframes` 的版本(hook 處呼叫它),測試面乾淨。

- [ ] **Step 5: main.go 佈線**

在 Task 7 佈線區之後(`hlsHandler := ...` 附近)加:

```go
	keyframeBackfillHandler := handler.NewKeyframeBackfillHandler(keyframeService)
	importService.SetKeyframeProber(keyframeService)
```

Route 區(`/admin/videos/backfill-codecs` 旁,main.go:275 附近)加:

```go
		api.POST("/admin/videos/backfill-keyframes", keyframeBackfillHandler.Run)
```

- [ ] **Step 6: 測試 + verify**

Run: `go test ./... && task verify`
Expected: 全綠。

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: add keyframe backfill endpoint and auto-probe on import"
```

---

### Task 9: 前端「首次播放準備中」(hls.js 503 重試)

**Files:**
- Create: `web/src/lib/hlsError.ts`
- Create: `web/src/lib/hlsError.test.ts`
- Modify: `web/src/pages/PlayerPage.tsx`(remux effect,約 210-236 行,與 player JSX 區)

**Interfaces:**
- Consumes: 後端 Playlist 503 `stream_not_ready`(Task 7)。
- Produces: `classifyHlsError(data, retryCount): 'retry-preparing' | 'fatal' | 'ignore'`、`MAX_PREPARING_RETRIES = 20`、`PREPARING_RETRY_DELAY_MS = 3000`。

- [ ] **Step 1: 寫失敗測試**

`web/src/lib/hlsError.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { classifyHlsError, MAX_PREPARING_RETRIES } from './hlsError'

describe('classifyHlsError', () => {
  it('non-fatal errors are ignored', () => {
    expect(classifyHlsError({ fatal: false, response: { code: 503 } }, 0)).toBe('ignore')
  })

  it('fatal 503 under retry cap → retry-preparing', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 503 } }, 0)).toBe('retry-preparing')
    expect(classifyHlsError({ fatal: true, response: { code: 503 } }, MAX_PREPARING_RETRIES - 1)).toBe('retry-preparing')
  })

  it('fatal 503 at retry cap → fatal', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 503 } }, MAX_PREPARING_RETRIES)).toBe('fatal')
  })

  it('fatal non-503 → fatal', () => {
    expect(classifyHlsError({ fatal: true, response: { code: 404 } }, 0)).toBe('fatal')
    expect(classifyHlsError({ fatal: true }, 0)).toBe('fatal')
  })
})
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `cd web && npx vitest run src/lib/hlsError.test.ts`
Expected: FAIL(模組不存在)。

- [ ] **Step 3: 實作 helper**

`web/src/lib/hlsError.ts`:

```ts
// hls.js 錯誤分類:manifest / segment 請求拿到 503(stream_not_ready,
// 首播 keyframe 探測進行中)屬「準備中」可輪詢重試;其他 fatal 錯誤即失敗。
// 重試上限 20 次 × 3s = 60s,涵蓋約 4GB 檔的冷讀探測(15s/GB)。
export const MAX_PREPARING_RETRIES = 20
export const PREPARING_RETRY_DELAY_MS = 3000

export type HlsErrorAction = 'retry-preparing' | 'fatal' | 'ignore'

export interface HlsErrorDataLike {
  fatal: boolean
  response?: { code?: number }
}

export function classifyHlsError(data: HlsErrorDataLike, retryCount: number): HlsErrorAction {
  if (!data.fatal) return 'ignore'
  if (data.response?.code === 503 && retryCount < MAX_PREPARING_RETRIES) return 'retry-preparing'
  return 'fatal'
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `cd web && npx vitest run src/lib/hlsError.test.ts`
Expected: PASS。

- [ ] **Step 5: PlayerPage 接線**

`web/src/pages/PlayerPage.tsx`:

1. 新增 state / ref(既有 state 宣告區):

```tsx
  const [preparing, setPreparing] = useState(false)
  const preparingRetryRef = useRef(0)
```

2. 改寫 remux effect(現約 211-236 行)。保留 Safari 原生分支與 `Hls.isSupported()` 檢查,hls.js 分支改為:

```tsx
    const hls = new Hls()
    let retryTimer: number | undefined
    hls.on(Hls.Events.ERROR, (_evt, data) => {
      const action = classifyHlsError(data, preparingRetryRef.current)
      if (action === 'retry-preparing') {
        preparingRetryRef.current += 1
        setPreparing(true)
        retryTimer = window.setTimeout(() => hls.loadSource(url), PREPARING_RETRY_DELAY_MS)
      } else if (action === 'fatal') {
        setPreparing(false)
        setError(preparingRetryRef.current > 0 ? '首次播放準備逾時,請稍後重試' : '串流載入失敗')
      }
    })
    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      preparingRetryRef.current = 0
      setPreparing(false)
    })
    hls.loadSource(url)
    hls.attachMedia(el)
    return () => {
      if (retryTimer) window.clearTimeout(retryTimer)
      hls.destroy()
    }
```

import 補:`import { classifyHlsError, PREPARING_RETRY_DELAY_MS } from '../lib/hlsError'`(路徑依 web/src 實際結構;若 pages 對 lib 是 `../lib` 就是這樣)。

注意 lint(react-hooks@7):`setPreparing` 只在事件 callback / timer 內呼叫(async 豁免),不在 effect body 同步呼叫;effect 依賴陣列維持 `[video?.id, video?.play_mode, streamToken]` 不變。

3. Player JSX 加 preparing 覆蓋提示(放在 video 元素同一容器,比照既有 loading/error 呈現風格;文案固定):

```tsx
{preparing && (
  <div className="player-preparing-overlay">
    首次播放準備中,索引建立後將自動開始…
  </div>
)}
```

樣式比照該頁既有 overlay/提示元素的做法(className 命名跟隨現有慣例;若該頁用 inline style 或 CSS module 則跟隨)。

4. Safari 原生 HLS 分支(`canPlayType` true):不加輪詢(使用者環境為 Chrome/PWA;Safari 首播 503 會觸發 `<video>` 的既有 error 處理路徑)。在該分支加一行註解說明此限制。

- [ ] **Step 6: 前端全量檢查**

Run: `cd web && npx tsc --noEmit && npm run lint && npx vitest run`
Expected: 全綠(含既有 PlayerPage 測試不破)。

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/hlsError.ts web/src/lib/hlsError.test.ts web/src/pages/PlayerPage.tsx
git commit -m "feat: show preparing state and retry on first-play 503 for remux HLS"
```

---

### Task 10: 整合測試更新 + 全量驗證

**Files:**
- Modify: `scripts/test_hls.sh`

**Interfaces:**
- Consumes: Task 7/8 的端點行為(VOD manifest、on-demand segment、lazy 503)。

- [ ] **Step 1: 更新 test_hls.sh**

在既有步驟上修改(檔案結構保留):

1. **步驟 [5] 改為輪詢**(首播可能 503 → 探測完成後 200;fixture 數 MB,探測 <2s):

```bash
# =====================================================================
# [5] GET HLS playlist — 首播觸發探測後應在時限內回 200 完整 VOD 清單
# =====================================================================
echo ""
bold "[5] GET /api/videos/:id/hls/index.m3u8 — 200 + 完整 VOD 清單"

PLAYLIST_CODE=""
for i in $(seq 1 60); do
    PLAYLIST_CODE=$(curl -s -o /tmp/hls_playlist.m3u8 -w "%{http_code}" \
        "${API_BASE}/api/videos/${MKV_ID}/hls/index.m3u8?token=${STREAM_TOKEN}")
    if [ "$PLAYLIST_CODE" = "200" ]; then
        break
    fi
    sleep 0.5
done
assert_eq "HLS playlist 回 200(容忍首播 lazy 探測)" "200" "$PLAYLIST_CODE"

PLAYLIST_BODY=$(cat /tmp/hls_playlist.m3u8)
assert_contains "playlist 包含 #EXTM3U" "$PLAYLIST_BODY" "#EXTM3U"
assert_contains "playlist 為 VOD 類型" "$PLAYLIST_BODY" "#EXT-X-PLAYLIST-TYPE:VOD"
assert_contains "playlist 包含 ENDLIST(進度條前提)" "$PLAYLIST_BODY" "#EXT-X-ENDLIST"
```

2. **步驟 [6] 簡化**(manifest 已完整,不需輪詢等 segment 行):直接從 `/tmp/hls_playlist.m3u8` grep `^seg[0-9]{5}\.ts\?token=` 取第一行,保留原 assert。

3. **步驟 [7] 保留**(首段 200)。

4. **新增步驟 [8]:跳抓最後一段**(驗 on-demand 中段/末段產生,對應任意 seek):

```bash
# =====================================================================
# [8] 跳抓最後一段(未線性播放前直接 seek)→ 200
# =====================================================================
echo ""
bold "[8] GET 最後一段(on-demand 產生)→ 200"

LAST_SEG_URI=$(grep -E '^seg[0-9]{5}\.ts\?token=' /tmp/hls_playlist.m3u8 | tail -1)
LAST_SEG_NAME=$(echo "$LAST_SEG_URI" | cut -d'?' -f1)
LAST_SEG_QUERY=$(echo "$LAST_SEG_URI" | cut -d'?' -f2-)

LAST_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "${API_BASE}/api/videos/${MKV_ID}/hls/${LAST_SEG_NAME}?${LAST_SEG_QUERY}")
assert_eq "最後一段 (${LAST_SEG_NAME}) 回 200" "200" "$LAST_CODE"
```

5. **新增步驟 [9]:超出範圍 → 404**:

```bash
# =====================================================================
# [9] 超出邊界表的 segment → 404
# =====================================================================
echo ""
bold "[9] GET seg99999.ts(超出範圍)→ 404"

OOR_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "${API_BASE}/api/videos/${MKV_ID}/hls/seg99999.ts?token=${STREAM_TOKEN}")
assert_eq "超出範圍 segment 回 404" "404" "$OOR_CODE"
```

- [ ] **Step 2: 跑整合測試**

Run: `task test-integration`
Expected: 全 suite PASS(特別是 hls)。注意:此指令會起乾淨全棧(留意 down -v 行為,勿在有要保留資料的環境跑錯 compose)。

- [ ] **Step 3: 全量 verify**

Run: `task verify`
Expected: 綠。

- [ ] **Step 4: Commit**

```bash
git add scripts/test_hls.sh
git commit -m "test: assert VOD manifest, on-demand mid-segment fetch and range check"
```

---

### Task 11: 部署 + 手動驗收(Chrome DevTools)+ backfill

**Files:** 無程式碼變更(驗收 task)。

- [ ] **Step 1: 本機部署**

```bash
task deploy
```

(建置含新 Go API 的 image 並 up;前端有變更,nginx image 亦重建 —— `task deploy` 已涵蓋。)

- [ ] **Step 2: 觸發全量 backfill**

```bash
TOKEN=$(curl -s -X POST http://localhost/api/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"<admin>","password":"<pass>"}' | jq -r '.data.token')
curl -s -X POST -m 7200 "http://localhost/api/admin/videos/backfill-keyframes" \
  -H "Authorization: Bearer ${TOKEN}"
```

Expected: `{"data":{"processed":~124,"failed":0}}`(約 85 分鐘;可先驗收 Step 3 的單片 lazy 路徑,backfill 平行跑)。API base 依實際部署 port 調整。

- [ ] **Step 3: Chrome DevTools 驗收(spec 驗收條件)**

用 chrome-devtools MCP:

1. `navigate_page` 開 `http://localhost` → 登入 → 進一支 remux 影片(如 EYAN-054)播放頁。
2. `evaluate_script`:

```js
(() => {
  const v = document.querySelector('video')
  return {
    duration: v.duration,
    seekableEnd: v.seekable.length ? v.seekable.end(0) : null,
    seekableLen: v.seekable.length,
  }
})()
```

Expected: `duration` 為有限值(≈影片長度)、`seekableLen >= 1`、`seekableEnd ≈ duration`。

3. `evaluate_script` 往前 seek:`document.querySelector('video').currentTime = duration * 0.7`,等 2s 後驗 `!video.paused && video.currentTime > duration*0.69` 且 `readyState >= 3`;再往回 seek `duration * 0.2` 同驗。
4. `take_screenshot` 確認原生 controls 進度條與總時長顯示。
5. 首播未探測片(挑一支 backfill 未到的):確認出現「首次播放準備中」提示,之後自動開播。

- [ ] **Step 4: 驗收結果記錄**

把 Step 2/3 結果(processed/failed 數、duration/seekable 實測值、截圖結論)貼回對話,對照 spec「測試與驗收」節逐項打勾。

---

## 驗收總表(對照 spec)

| 條件 | 驗證位置 |
|---|---|
| `video.duration` 有限、seekable 涵蓋全片、進度條可見 | Task 11 Step 3 |
| 往前/往後 seek 都能播 | Task 11 Step 3 |
| manifest 含 ENDLIST、VOD 類型 | Task 3 單元 + Task 10 整合 |
| 任意段 on-demand 產生(含末段)、超界 404 | Task 5/7 單元 + Task 10 整合 |
| 首播 lazy:503 + 準備中 + 自動開播 | Task 7/9 單元 + Task 11 Step 3.5 |
| backfill 全量跑完 | Task 11 Step 2 |
| `task verify` 綠 | Task 7/8/10 |
| `task test-integration` 綠 | Task 10 |
| PR CI 綠 | finishing-a-development-branch 階段 |
