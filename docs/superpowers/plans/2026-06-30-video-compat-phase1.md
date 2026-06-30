# 影片相容性 Phase 1（即時 remux）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 讓 codec 相容但容器不相容的影片（124 部 AVI/MKV 內含 H.264+AAC/MP3）能透過即時 HLS remux 在瀏覽器播放，並建立 Phase 2 transcode 共用的串流基礎建設。

**Architecture:** 匯入時用 ffprobe 多存 `video_codec`/`audio_codec`，一個純函式分類器把每部片判為 `direct`/`remux`/`transcode`。`direct` 走現有 `http.ServeFile`；`remux` 走新的 `internal/streaming` session manager —— 它對每個 (videoID,userID) 起一個 `ffmpeg -c copy -f hls` 程序寫 TS 分段到暫存目錄，由新 HLS handler 供給 playlist 與分段，前端以 hls.js 播放。`transcode` 暫時顯示提示不播放。

**Tech Stack:** Go 1.24+ / Gin / pgx / PostgreSQL 16 / ffmpeg（容器內已具備）/ React 19 + TypeScript / hls.js / vitest

## Global Constraints

- Go 1.22+（CI 用 1.24+）；PostgreSQL 16；React 19 + TypeScript。
- 分層嚴格：Handler 不寫 SQL、不碰 MinIO；Service 不碰 `*gin.Context`；Repository 不含業務邏輯。
- 跨層依賴用 interface（定義在使用端 package），constructor injection，pointer receiver，無全域可變狀態，無 `init()`。
- Error 用 `fmt.Errorf("...: %w", err)` wrap，小寫開頭不加句號；只在 handler 層 log + HTTP response。
- `log/slog` 結構化欄位，不拼字串。
- SQL 關鍵字大寫、欄位 snake_case、parameterized query、query 寫成檔頂 const。
- Migration 命名 `NNN_description.up.sql`/`.down.sql`，down 完整可逆。
- 路徑安全：使用者可控路徑必須 `filepath.Clean` + 前綴檢查 + 白名單，sentinel error 用 `model.ErrPathNotAllowed`。
- 每個 Go 檔 ≤300 行、每個 function ≤50 行；import 分三組。
- 測試：table-driven、mock 手寫放 `internal/mock/`、命名 `Test<Function>_<Scenario>`、覆蓋正常/404/403/400。
- 前端：API client interceptor 統一解 `{data}`；自動重試有上限且用 `useRef` 計數；`useEffect` async 用 cleanup flag。
- Done condition：`task verify` 綠 + `task test-integration` 綠（本功能改到串流）+ PR CI 綠。
- Commit type：`feat`/`fix`/`refactor`/`docs`/`chore`/`test`。每個 commit 訊息結尾加
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`。
- 分支：`feat/video-compat-phase1`（已建立，spec 已 commit 於其上）。

---

## File Structure

| 檔案 | 責任 |
|---|---|
| `migrations/014_add_video_codecs.up.sql` / `.down.sql` | 新增 `videos.video_codec`/`audio_codec` 欄位 |
| `internal/model/video.go`（修改） | `Video` 加 `VideoCodec`/`AudioCodec`；`VideoDetail` 加 `PlayMode`；新增 `PlayMode` 型別與常數 |
| `internal/service/play_mode.go`（新增） | 純函式 `ClassifyPlayMode(container, vcodec, acodec) PlayMode` |
| `internal/service/import_service.go`（修改） | probe 多抓 codec、修 MimeType、insert 帶 codec |
| `internal/repository/video_repo.go` + `video_repo_helpers.go`（修改） | 所有 INSERT/SELECT 加 codec 欄位與 scan |
| `internal/service/video_service.go`（修改） | `GetByID` 用分類器填 `PlayMode` |
| `internal/streaming/transcoder.go`（新增） | `Transcoder`/`TranscodeProc` interface + ffmpeg 實作 + arg builder |
| `internal/streaming/manager.go`（新增） | session manager：start/get/idle-cleanup |
| `internal/handler/hls_handler.go`（新增） | `HLSPlaylist`/`HLSSegment` handler |
| `internal/middleware/auth.go`（修改） | stream-scope token 放行 HLS 路由 |
| `internal/service/codec_backfill_service.go`（新增） | 補掃既有影片 codec |
| `internal/handler/codec_backfill_handler.go`（新增） | backfill 端點 |
| `cmd/server/main.go`（修改） | wire 新 service/handler + 註冊路由 |
| `docker-compose.yml` + `docker-compose.prod.yml`（修改） | `vaultflix-transcode-cache` 可寫 volume |
| `internal/config/config.go`（修改） | `TranscodeCacheDir` 設定 |
| `web/package.json`（修改） | 加 `hls.js` |
| `web/src/types/index.ts`（修改） | `VideoDetail` 加 `play_mode` |
| `web/src/pages/PlayerPage.tsx`（修改） | 依 `play_mode` 分流播放 |
| `internal/mock/video_repo_mock.go`（修改） | 同步 model 變更（若需要） |

設計偏離 spec 一處（已評估）：session manager 用 **`sync.Mutex` 守護的 map** 而非 spec 寫的「channel 序列化比照 Hub」。Hub 用 channel 是因為要 broadcast；session store 只是查表，mutex 更簡單且同樣滿足「不裸用 map」。這避免不必要的 actor 複雜度（符合 CLAUDE.md 反過度設計原則）。

---

## Task 1: DB migration — 新增 codec 欄位

**Files:**
- Create: `migrations/014_add_video_codecs.up.sql`
- Create: `migrations/014_add_video_codecs.down.sql`

**Interfaces:**
- Produces: `videos.video_codec varchar(64)`、`videos.audio_codec varchar(64)`（皆 nullable）

- [ ] **Step 1: 寫 up migration**

`migrations/014_add_video_codecs.up.sql`:
```sql
ALTER TABLE videos ADD COLUMN video_codec VARCHAR(64);
ALTER TABLE videos ADD COLUMN audio_codec VARCHAR(64);
```

- [ ] **Step 2: 寫 down migration**

`migrations/014_add_video_codecs.down.sql`:
```sql
ALTER TABLE videos DROP COLUMN audio_codec;
ALTER TABLE videos DROP COLUMN video_codec;
```

- [ ] **Step 3: 驗證 migration 可套用（乾淨 DB）**

Run: `docker compose exec -T postgres psql -U vaultflix -d vaultflix -c "ALTER TABLE videos ADD COLUMN video_codec VARCHAR(64); ALTER TABLE videos ADD COLUMN audio_codec VARCHAR(64);"`
Expected: `ALTER TABLE`（若已存在欄位代表後續 task 已跑過，可忽略）。實際 migration 套用由既有 migrate 機制在 app 啟動時執行。

- [ ] **Step 4: Commit**

```bash
git add migrations/014_add_video_codecs.up.sql migrations/014_add_video_codecs.down.sql
git commit -m "feat: add video_codec/audio_codec columns migration

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Model — codec 欄位與 PlayMode 型別

**Files:**
- Modify: `internal/model/video.go`

**Interfaces:**
- Produces:
  - `Video.VideoCodec string` (json `video_codec`)、`Video.AudioCodec string` (json `audio_codec`)
  - `type PlayMode string`；常數 `PlayModeDirect = "direct"`、`PlayModeRemux = "remux"`、`PlayModeTranscode = "transcode"`
  - `VideoDetail.PlayMode PlayMode` (json `play_mode`)

- [ ] **Step 1: 加 codec 欄位到 Video struct**

在 `internal/model/video.go` 的 `Video` struct 內，於 `MimeType` 後加：
```go
	MimeType         string     `json:"mime_type"`
	VideoCodec       string     `json:"video_codec,omitempty"`
	AudioCodec       string     `json:"audio_codec,omitempty"`
```

- [ ] **Step 2: 加 PlayMode 型別與常數**

在 `internal/model/video.go` 檔尾加：
```go
// PlayMode 表示前端應如何播放這部影片。
type PlayMode string

const (
	// PlayModeDirect：容器與編碼皆瀏覽器原生相容，直接 http.ServeFile。
	PlayModeDirect PlayMode = "direct"
	// PlayModeRemux：編碼相容、容器不相容，走即時 HLS -c copy。
	PlayModeRemux PlayMode = "remux"
	// PlayModeTranscode：編碼不相容，需真正轉碼（Phase 2 才支援）。
	PlayModeTranscode PlayMode = "transcode"
)
```

- [ ] **Step 3: 加 PlayMode 到 VideoDetail**

在 `VideoDetail` struct 內加欄位：
```go
type VideoDetail struct {
	VideoWithTags
	StreamURL     string   `json:"stream_url"`
	PlayMode      PlayMode `json:"play_mode"`
	IsFavorited   bool     `json:"is_favorited"`
	WatchProgress int      `json:"watch_progress"`
}
```

- [ ] **Step 4: 編譯確認**

Run: `go build ./internal/model/...`
Expected: 無錯誤

- [ ] **Step 5: Commit**

```bash
git add internal/model/video.go
git commit -m "feat: add codec fields and PlayMode type to video model

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: 播放策略分類器（純函式 + TDD）

**Files:**
- Create: `internal/service/play_mode.go`
- Test: `internal/service/play_mode_test.go`

**Interfaces:**
- Consumes: `model.PlayMode` 常數（Task 2）
- Produces: `func ClassifyPlayMode(container, videoCodec, audioCodec string) model.PlayMode`
  - `container`：副檔名去掉點、小寫（如 `mp4`/`mkv`/`avi`/`wmv`）
  - `videoCodec`/`audioCodec`：ffprobe 的 `codec_name`（如 `h264`/`mpeg4`/`aac`/`mp3`）

- [ ] **Step 1: 寫失敗測試**

`internal/service/play_mode_test.go`:
```go
package service

import (
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func TestClassifyPlayMode(t *testing.T) {
	tests := []struct {
		name      string
		container string
		vcodec    string
		acodec    string
		want      model.PlayMode
	}{
		{"mp4 h264 aac is direct", "mp4", "h264", "aac", model.PlayModeDirect},
		{"mp4 h264 mp3 is direct", "mp4", "h264", "mp3", model.PlayModeDirect},
		{"mov h264 aac is direct", "mov", "h264", "aac", model.PlayModeDirect},
		{"avi h264 aac is remux", "avi", "h264", "aac", model.PlayModeRemux},
		{"avi h264 mp3 is remux", "avi", "h264", "mp3", model.PlayModeRemux},
		{"mkv h264 aac is remux", "mkv", "h264", "aac", model.PlayModeRemux},
		{"avi mpeg4 mp3 is transcode", "avi", "mpeg4", "mp3", model.PlayModeTranscode},
		{"mp4 mpeg4 aac is transcode", "mp4", "mpeg4", "aac", model.PlayModeTranscode},
		{"wmv wmv2 wmav2 is transcode", "wmv", "wmv2", "wmav2", model.PlayModeTranscode},
		{"mp4 hevc aac is transcode", "mp4", "hevc", "aac", model.PlayModeTranscode},
		{"mkv mpeg2video ac3 is transcode", "mkv", "mpeg2video", "ac3", model.PlayModeTranscode},
		{"uppercase normalized", "MP4", "H264", "AAC", model.PlayModeDirect},
		{"unknown video codec is transcode", "mp4", "", "aac", model.PlayModeTranscode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPlayMode(tt.container, tt.vcodec, tt.acodec)
			if got != tt.want {
				t.Errorf("ClassifyPlayMode(%q,%q,%q) = %q, want %q",
					tt.container, tt.vcodec, tt.acodec, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/service/ -run TestClassifyPlayMode -v`
Expected: FAIL（`undefined: ClassifyPlayMode`）

- [ ] **Step 3: 寫實作**

`internal/service/play_mode.go`:
```go
package service

import (
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// 瀏覽器 <video> 原生可解的編碼與容器集合（Phase 1 保守取交集）。
var (
	browserVideoCodecs = map[string]bool{"h264": true}
	browserAudioCodecs = map[string]bool{"aac": true, "mp3": true}
	browserContainers  = map[string]bool{"mp4": true, "mov": true}
)

// ClassifyPlayMode 依容器副檔名與編碼判定播放策略。
// direct：容器+編碼皆相容；remux：編碼相容但容器不相容；
// transcode：視訊或音訊編碼不相容。
func ClassifyPlayMode(container, videoCodec, audioCodec string) model.PlayMode {
	c := strings.ToLower(strings.TrimPrefix(container, "."))
	v := strings.ToLower(videoCodec)
	a := strings.ToLower(audioCodec)

	if !browserVideoCodecs[v] || !browserAudioCodecs[a] {
		return model.PlayModeTranscode
	}
	if !browserContainers[c] {
		return model.PlayModeRemux
	}
	return model.PlayModeDirect
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/service/ -run TestClassifyPlayMode -v`
Expected: PASS（全部 case）

- [ ] **Step 5: Commit**

```bash
git add internal/service/play_mode.go internal/service/play_mode_test.go
git commit -m "feat: add browser play-mode classifier

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: 匯入時探測 codec + 修 MimeType（TDD parse）

**Files:**
- Modify: `internal/service/import_service.go`
- Test: `internal/service/import_service_test.go`（新增 test func；若檔不存在則建立）

**Interfaces:**
- Consumes: ffprobe JSON output
- Produces:
  - `videoMetadata` 加欄位 `videoCodec string`、`audioCodec string`
  - 新增純函式 `parseProbeOutput(raw []byte, ext string) (*videoMetadata, error)`（把原本內嵌在 `probeMetadata` 的解析邏輯抽出，便於測試）

- [ ] **Step 1: 寫失敗測試（解析 ffprobe JSON）**

在 `internal/service/import_service_test.go` 加：
```go
func TestParseProbeOutput_ExtractsCodecs(t *testing.T) {
	raw := []byte(`{
		"format": {"duration": "120.5", "size": "1048576"},
		"streams": [
			{"codec_type": "video", "codec_name": "h264", "width": 1920, "height": 1080},
			{"codec_type": "audio", "codec_name": "aac"}
		]
	}`)
	md, err := parseProbeOutput(raw, ".mkv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md.videoCodec != "h264" {
		t.Errorf("videoCodec = %q, want h264", md.videoCodec)
	}
	if md.audioCodec != "aac" {
		t.Errorf("audioCodec = %q, want aac", md.audioCodec)
	}
	if md.durationSeconds != 120 {
		t.Errorf("durationSeconds = %d, want 120", md.durationSeconds)
	}
	if md.resolution != "1920x1080" {
		t.Errorf("resolution = %q, want 1920x1080", md.resolution)
	}
}

func TestParseProbeOutput_DirectMP4GetsVideoMP4Mime(t *testing.T) {
	raw := []byte(`{"format":{"duration":"10"},"streams":[
		{"codec_type":"video","codec_name":"h264","width":640,"height":480},
		{"codec_type":"audio","codec_name":"aac"}]}`)
	md, err := parseProbeOutput(raw, ".mp4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if md.mimeType != "video/mp4" {
		t.Errorf("mimeType = %q, want video/mp4", md.mimeType)
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/service/ -run TestParseProbeOutput -v`
Expected: FAIL（`undefined: parseProbeOutput` 與 `videoMetadata` 無 codec 欄位）

- [ ] **Step 3: 擴充 struct 與抽出 parse 函式**

在 `internal/service/import_service.go`：

(a) `videoMetadata` 加欄位：
```go
type videoMetadata struct {
	durationSeconds int
	resolution      string
	mimeType        string
	videoCodec      string
	audioCodec      string
}
```

(b) `ffprobeStream` 加 `CodecName`：
```go
type ffprobeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}
```

(c) 新增純函式（放在 `probeMetadata` 之後）：
```go
func parseProbeOutput(raw []byte, ext string) (*videoMetadata, error) {
	var probe ffprobeOutput
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	duration, _ := strconv.ParseFloat(probe.Format.Duration, 64)

	var resolution, videoCodec, audioCodec string
	for _, stream := range probe.Streams {
		switch stream.CodecType {
		case "video":
			if videoCodec == "" {
				videoCodec = stream.CodecName
				resolution = fmt.Sprintf("%dx%d", stream.Width, stream.Height)
			}
		case "audio":
			if audioCodec == "" {
				audioCodec = stream.CodecName
			}
		}
	}

	return &videoMetadata{
		durationSeconds: int(duration),
		resolution:      resolution,
		mimeType:        mimeTypeFor(strings.ToLower(ext), videoCodec, audioCodec),
		videoCodec:      videoCodec,
		audioCodec:      audioCodec,
	}, nil
}

// mimeTypeFor 只對 direct 影片回精確的 video/mp4；其餘交由播放策略決定路徑，
// 不再用副檔名硬猜（修正既有 bug：.avi 被標 video/x-msvideo 導致瀏覽器直接拒播）。
func mimeTypeFor(ext, videoCodec, audioCodec string) string {
	if ClassifyPlayMode(ext, videoCodec, audioCodec) == model.PlayModeDirect {
		return "video/mp4"
	}
	return extensionToMIME(ext)
}
```

(d) 把 `probeMetadata` 改成呼叫 `parseProbeOutput`：
```go
func (s *ImportService) probeMetadata(ctx context.Context, filePath string) (*videoMetadata, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		return nil, fmt.Errorf("ffprobe failed: %w, stderr: %s", err, stderr)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	return parseProbeOutput(output, ext)
}
```

(e) 確認 `internal/model` 已 import（`import_service.go` 應已 import；若無則加 `"github.com/steven/vaultflix/internal/model"`）。

- [ ] **Step 4: insert 帶 codec**

在 `import_service.go` 建立 `video := &model.Video{...}` 處，加：
```go
		MimeType:         metadata.mimeType,
		VideoCodec:       metadata.videoCodec,
		AudioCodec:       metadata.audioCodec,
```

- [ ] **Step 5: 跑測試確認通過**

Run: `go test ./internal/service/ -run TestParseProbeOutput -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/service/import_service.go internal/service/import_service_test.go
git commit -m "feat: probe and store video/audio codec at import, fix mime type

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Repository — 持久化與讀取 codec 欄位

**Files:**
- Modify: `internal/repository/video_repo.go`
- Modify: `internal/repository/video_repo_helpers.go`（若 scan 邏輯在此）

**Interfaces:**
- Consumes: `model.Video.VideoCodec`/`AudioCodec`（Task 2）
- Produces: 所有讀寫 video 的 query 都含 `video_codec`/`audio_codec`

說明：repo SQL 在整合測試中驗證（CLAUDE.md 規定 repo 不連真實 DB 做單元測試）。本 task 的「測試」是 `go build` + Task 12 整合測試。codec 欄位 nullable，用 `COALESCE(video_codec,'')` 避免 scan null 進 string 出錯（沿用既有 COALESCE 慣例）。

- [ ] **Step 1: 更新 INSERT query**

在 `video_repo.go` 的 `queryInsertVideo`（檔頂 const，約 line 65）把欄位與 placeholder 補上 `video_codec, audio_codec`。例如原本：
```sql
INSERT INTO videos (id, title, description, minio_object_key, thumbnail_key, preview_key,
                    duration_seconds, resolution, file_size_bytes, mime_type, ...)
VALUES ($1, $2, ...)
```
在 `mime_type` 後加 `video_codec, audio_codec`，並對應加兩個 placeholder（接續編號），且在 `Exec(...)` 的參數列對應位置加 `video.VideoCodec, video.AudioCodec`。

- [ ] **Step 2: 更新所有 SELECT query 與 scan**

對 `video_repo.go` 中每個列出欄位的 SELECT（GetByID、List dataQuery、其他），在 `mime_type` 後加 `COALESCE(video_codec, '') AS video_codec, COALESCE(audio_codec, '') AS audio_codec`，並在對應的 `rows.Scan(...)` / `row.Scan(...)` 參數列於 `&v.MimeType` 後加 `&v.VideoCodec, &v.AudioCodec`。

掃描 helper 若集中在 `video_repo_helpers.go`（如 `scanVideo`），只需改一處。先確認：
Run: `grep -n "MimeType\|mime_type\|scanVideo\|rows.Scan\|row.Scan" internal/repository/video_repo.go internal/repository/video_repo_helpers.go`
依結果在每個 scan 點補兩個欄位。

- [ ] **Step 3: 編譯確認**

Run: `go build ./...`
Expected: 無錯誤

- [ ] **Step 4: go vet + gofmt**

Run: `go vet ./internal/repository/... && gofmt -l internal/repository/`
Expected: 無輸出

- [ ] **Step 5: Commit**

```bash
git add internal/repository/video_repo.go internal/repository/video_repo_helpers.go
git commit -m "feat: persist and read codec columns in video repository

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: VideoDetail 帶 play_mode

**Files:**
- Modify: `internal/service/video_service.go`
- Test: `internal/service/video_service_test.go`

**Interfaces:**
- Consumes: `ClassifyPlayMode`（Task 3）、`Video.VideoCodec/AudioCodec`、`Video.OriginalFilename`
- Produces: `VideoDetail.PlayMode` 已填值

- [ ] **Step 1: 寫失敗測試**

在 `internal/service/video_service_test.go` 加（沿用該檔既有 mock 與 newTestVideoService 風格；下例假設既有 helper，若名稱不同請對應調整）：
```go
func TestGetByID_SetsPlayModeRemux(t *testing.T) {
	svc, mocks := newTestVideoService(t) // 沿用既有 helper
	mocks.videoRepo.getByIDFunc = func(_ context.Context, id string) (*model.Video, error) {
		return &model.Video{
			ID:               id,
			OriginalFilename: "movie.mkv",
			VideoCodec:       "h264",
			AudioCodec:       "aac",
		}, nil
	}
	detail, err := svc.GetByID(context.Background(), "v1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.PlayMode != model.PlayModeRemux {
		t.Errorf("PlayMode = %q, want remux", detail.PlayMode)
	}
}
```
（若該測試檔沒有 `newTestVideoService` helper，改用檔內既有建構 service 的方式並設定 `getByID` 行為。）

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/service/ -run TestGetByID_SetsPlayModeRemux -v`
Expected: FAIL（`PlayMode` 為空字串）

- [ ] **Step 3: 在 GetByID 填 PlayMode**

在 `video_service.go` 的 `GetByID`，建立 `detail` 後加（container 用副檔名）：
```go
	container := strings.TrimPrefix(filepath.Ext(video.OriginalFilename), ".")
	detail.PlayMode = ClassifyPlayMode(container, video.VideoCodec, video.AudioCodec)
```
確認檔案已 import `"path/filepath"` 與 `"strings"`（若無則加入標準庫 import 群組）。

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/service/ -run TestGetByID_SetsPlayModeRemux -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/video_service.go internal/service/video_service_test.go
git commit -m "feat: expose play_mode in video detail

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: Streaming — Transcoder interface + ffmpeg HLS arg builder（TDD）

**Files:**
- Create: `internal/streaming/transcoder.go`
- Test: `internal/streaming/transcoder_test.go`

**Interfaces:**
- Produces:
  - `func buildRemuxHLSArgs(inputPath, outDir string) []string`
  - `type TranscodeProc interface { Stop() error; Done() <-chan struct{} }`
  - `type Transcoder interface { Start(ctx context.Context, inputPath, outDir string) (TranscodeProc, error) }`
  - `func NewFFmpegTranscoder() *FFmpegTranscoder`（實作 `Transcoder`）
  - 常數 `PlaylistName = "index.m3u8"`、`segmentPattern = "seg%05d.ts"`

- [ ] **Step 1: 寫失敗測試（arg builder）**

`internal/streaming/transcoder_test.go`:
```go
package streaming

import (
	"strings"
	"testing"
)

func TestBuildRemuxHLSArgs_CopiesCodecsAndEventPlaylist(t *testing.T) {
	args := buildRemuxHLSArgs("/mnt/host/D/movie.mkv", "/cache/sess1")
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"-i /mnt/host/D/movie.mkv",
		"-c copy",
		"-f hls",
		"-hls_playlist_type event",
		"/cache/sess1/index.m3u8",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q; got: %s", want, joined)
		}
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/streaming/ -run TestBuildRemuxHLSArgs -v`
Expected: FAIL（package/func 不存在）

- [ ] **Step 3: 寫實作**

`internal/streaming/transcoder.go`:
```go
package streaming

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

const (
	// PlaylistName 是每個 session 暫存目錄內的 HLS playlist 檔名。
	PlaylistName = "index.m3u8"
	// segmentPattern 是 ffmpeg 寫出的分段檔名樣板。
	segmentPattern = "seg%05d.ts"
)

// buildRemuxHLSArgs 產生「容器換殼不重新編碼」的 ffmpeg HLS 參數。
// 用 event playlist 讓 hls.js 能在分段陸續產出時即時開播。
func buildRemuxHLSArgs(inputPath, outDir string) []string {
	return []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-c", "copy",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "event",
		"-hls_segment_filename", filepath.Join(outDir, segmentPattern),
		filepath.Join(outDir, PlaylistName),
	}
}

// TranscodeProc 是一個進行中的 HLS 產出程序。
type TranscodeProc interface {
	// Stop 終止程序並回傳等待結果；對已結束的程序為 no-op。
	Stop() error
	// Done 在程序自然結束時關閉。
	Done() <-chan struct{}
}

// Transcoder 啟動把 inputPath 轉成 HLS（playlist + 分段）寫入 outDir 的程序。
// 找不到輸入或無法啟動時回傳 error。
type Transcoder interface {
	Start(ctx context.Context, inputPath, outDir string) (TranscodeProc, error)
}

// FFmpegTranscoder 以 -c copy 即時 remux 為 HLS。
type FFmpegTranscoder struct{}

func NewFFmpegTranscoder() *FFmpegTranscoder { return &FFmpegTranscoder{} }

func (t *FFmpegTranscoder) Start(ctx context.Context, inputPath, outDir string) (TranscodeProc, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", buildRemuxHLSArgs(inputPath, outDir)...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg remux: %w", err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	return &ffmpegProc{cmd: cmd, done: done}, nil
}

type ffmpegProc struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *ffmpegProc) Done() <-chan struct{} { return p.done }

func (p *ffmpegProc) Stop() error {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	<-p.done
	return nil
}
```

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/streaming/ -run TestBuildRemuxHLSArgs -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/streaming/transcoder.go internal/streaming/transcoder_test.go
git commit -m "feat: add ffmpeg HLS remux transcoder and arg builder

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Streaming — session manager（TDD，含 idle 清理）

**Files:**
- Create: `internal/streaming/manager.go`
- Test: `internal/streaming/manager_test.go`

**Interfaces:**
- Consumes: `Transcoder`/`TranscodeProc`、`PlaylistName`（Task 7）
- Produces:
  - `type Manager struct{ ... }`
  - `func NewManager(t Transcoder, cacheDir string, idleTimeout time.Duration) *Manager`
  - `func (m *Manager) EnsureSession(ctx context.Context, videoID, userID, inputPath string) (*Session, error)`
    — 啟動或回傳既有 session，並更新 lastAccess。
  - `func (m *Manager) Touch(videoID, userID string)` — 更新 lastAccess（分段請求用）。
  - `func (m *Manager) SessionDir(videoID, userID string) (string, bool)` — 回 session 暫存目錄。
  - `func (m *Manager) Sweep(now time.Time)` — 清理逾時 session（測試可直接呼叫，正式由背景 goroutine 定期呼叫）。
  - `func (m *Manager) StartSweeper(ctx context.Context)` — 啟動背景清理 ticker。
  - `type Session struct{ Dir string }`（對外只暴露 Dir）

- [ ] **Step 1: 寫失敗測試**

`internal/streaming/manager_test.go`:
```go
package streaming

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeTranscoder 不跑 ffmpeg：建立一個假的 playlist 檔，回傳一個可手動結束的 proc。
type fakeTranscoder struct{ started int }
type fakeProc struct{ done chan struct{} }

func (p *fakeProc) Done() <-chan struct{} { return p.done }
func (p *fakeProc) Stop() error           { select { case <-p.done: default: close(p.done) }; return nil }

func (f *fakeTranscoder) Start(_ context.Context, _, outDir string) (TranscodeProc, error) {
	f.started++
	_ = os.WriteFile(filepath.Join(outDir, PlaylistName), []byte("#EXTM3U\n"), 0o644)
	return &fakeProc{done: make(chan struct{})}, nil
}

func TestEnsureSession_ReusesSameSession(t *testing.T) {
	ft := &fakeTranscoder{}
	m := NewManager(ft, t.TempDir(), time.Minute)
	if _, err := m.EnsureSession(context.Background(), "v1", "u1", "/in.mkv"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, err := m.EnsureSession(context.Background(), "v1", "u1", "/in.mkv"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if ft.started != 1 {
		t.Errorf("transcoder started %d times, want 1 (session reused)", ft.started)
	}
}

func TestSweep_RemovesIdleSession(t *testing.T) {
	ft := &fakeTranscoder{}
	m := NewManager(ft, t.TempDir(), 10*time.Second)
	s, _ := m.EnsureSession(context.Background(), "v1", "u1", "/in.mkv")
	if _, err := os.Stat(s.Dir); err != nil {
		t.Fatalf("session dir should exist: %v", err)
	}
	// 模擬時間前進超過 idleTimeout
	m.Sweep(time.Now().Add(time.Minute))
	if _, ok := m.SessionDir("v1", "u1"); ok {
		t.Error("idle session should have been removed")
	}
	if _, err := os.Stat(s.Dir); !os.IsNotExist(err) {
		t.Error("session dir should have been deleted")
	}
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/streaming/ -run "TestEnsureSession|TestSweep" -v`
Expected: FAIL（`NewManager` 等未定義）

- [ ] **Step 3: 寫實作**

`internal/streaming/manager.go`:
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
)

// Manager 管理每個 (videoID,userID) 的即時 HLS session。
// 用 mutex 守護 map（非裸用），不需 actor/channel 複雜度。
type Manager struct {
	transcoder  Transcoder
	cacheDir    string
	idleTimeout time.Duration

	mu       sync.Mutex
	sessions map[string]*sessionState
}

type sessionState struct {
	dir        string
	proc       TranscodeProc
	lastAccess time.Time
}

// Session 是對外暴露的 session 視圖。
type Session struct{ Dir string }

func NewManager(t Transcoder, cacheDir string, idleTimeout time.Duration) *Manager {
	return &Manager{
		transcoder:  t,
		cacheDir:    cacheDir,
		idleTimeout: idleTimeout,
		sessions:    make(map[string]*sessionState),
	}
}

func sessionKey(videoID, userID string) string { return videoID + "|" + userID }

// EnsureSession 啟動或回傳既有 session，並更新 lastAccess。
func (m *Manager) EnsureSession(ctx context.Context, videoID, userID, inputPath string) (*Session, error) {
	key := sessionKey(videoID, userID)

	m.mu.Lock()
	defer m.mu.Unlock()

	if st, ok := m.sessions[key]; ok {
		st.lastAccess = time.Now()
		return &Session{Dir: st.dir}, nil
	}

	dir := filepath.Join(m.cacheDir, sanitizeKey(key))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create session dir: %w", err)
	}

	proc, err := m.transcoder.Start(ctx, inputPath, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("failed to start transcoder: %w", err)
	}

	m.sessions[key] = &sessionState{dir: dir, proc: proc, lastAccess: time.Now()}
	return &Session{Dir: dir}, nil
}

// Touch 更新 session 的 lastAccess（分段請求時呼叫）。
func (m *Manager) Touch(videoID, userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.sessions[sessionKey(videoID, userID)]; ok {
		st.lastAccess = time.Now()
	}
}

// SessionDir 回傳 session 暫存目錄；不存在回 ok=false。
func (m *Manager) SessionDir(videoID, userID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.sessions[sessionKey(videoID, userID)]
	if !ok {
		return "", false
	}
	return st.dir, true
}

// Sweep 清理 lastAccess 早於 now-idleTimeout 的 session。
func (m *Manager) Sweep(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, st := range m.sessions {
		if now.Sub(st.lastAccess) <= m.idleTimeout {
			continue
		}
		if err := st.proc.Stop(); err != nil {
			slog.Warn("failed to stop transcode proc", "key", key, "error", err)
		}
		if err := os.RemoveAll(st.dir); err != nil {
			slog.Warn("failed to remove session dir", "dir", st.dir, "error", err)
		}
		delete(m.sessions, key)
	}
}

// StartSweeper 每 idleTimeout/2 跑一次 Sweep，直到 ctx 取消。
func (m *Manager) StartSweeper(ctx context.Context) {
	interval := m.idleTimeout / 2
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
				m.Sweep(t)
			}
		}
	}()
}

// sanitizeKey 把 session key 變成安全的單層目錄名（避免路徑穿越）。
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

- [ ] **Step 4: 跑測試確認通過**

Run: `go test ./internal/streaming/ -v`
Expected: PASS（全部）

- [ ] **Step 5: Commit**

```bash
git add internal/streaming/manager.go internal/streaming/manager_test.go
git commit -m "feat: add HLS streaming session manager with idle cleanup

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: HLS handler（playlist + segment，含路徑安全）

**Files:**
- Create: `internal/handler/hls_handler.go`
- Test: `internal/handler/hls_handler_test.go`

**Interfaces:**
- Consumes:
  - `*streaming.Manager`（Task 8：`EnsureSession`/`Touch`/`SessionDir`、`streaming.PlaylistName`）
  - `videoService.GetByID`、`mediaSourceService.GetByID`（解析磁碟路徑，沿用 `Stream()` 邏輯）
  - `service.AllowedMountPrefix`、`model.ErrNotFound`
- Produces:
  - `type HLSHandler struct{ ... }` + `func NewHLSHandler(videoSvc, sourceSvc, mgr) *HLSHandler`
  - `func (h *HLSHandler) Playlist(c *gin.Context)` → `GET /api/videos/:id/hls/index.m3u8`
  - `func (h *HLSHandler) Segment(c *gin.Context)` → `GET /api/videos/:id/hls/:segment`
  - 分段名驗證：`^seg\d{5}\.ts$`

說明：路徑解析（source mount + file_path + path-traversal 檢查）與 `video_handler.go` 的 `Stream()` 重複。為避免兩處漂移，將該段抽成 `videoService` 的方法 `ResolveDiskPath(ctx, videoID) (string, error)` 並讓 `Stream()` 與本 handler 共用（重構納入本 task）。

- [ ] **Step 1: 抽出共用路徑解析（重構）**

在 `internal/service/video_service.go` 加方法（回傳已驗證、存在的絕對路徑；找不到回 `model.ErrNotFound`，來源停用回 `model.ErrConflict`）：
```go
// ResolveDiskPath 回傳影片在磁碟上的安全絕對路徑。
// 影片不存在或非磁碟來源回 model.ErrNotFound；來源停用回 model.ErrConflict。
func (s *VideoService) ResolveDiskPath(ctx context.Context, videoID string) (string, error) {
	video, err := s.videoRepo.GetByID(ctx, videoID)
	if err != nil {
		return "", fmt.Errorf("failed to get video %s: %w", videoID, err)
	}
	if video.SourceID == nil || video.FilePath == nil {
		return "", model.ErrNotFound
	}
	source, err := s.sourceRepo.GetByID(ctx, *video.SourceID)
	if err != nil {
		return "", fmt.Errorf("failed to get source %s: %w", *video.SourceID, err)
	}
	if !source.Enabled {
		return "", model.ErrConflict
	}

	cleanPath := filepath.Clean(filepath.Join(source.MountPath, *video.FilePath))
	cleanMount := filepath.Clean(source.MountPath)
	if !strings.HasPrefix(cleanPath, cleanMount+string(filepath.Separator)) && cleanPath != cleanMount {
		return "", model.ErrPathNotAllowed
	}
	if _, err := os.Stat(cleanPath); err != nil {
		if os.IsNotExist(err) {
			return "", model.ErrPathNotExist
		}
		return "", fmt.Errorf("failed to stat %s: %w", cleanPath, err)
	}
	return cleanPath, nil
}
```
確認 `video_service.go` 有 `sourceRepo` 欄位；若 VideoService 目前不依賴 source repo，改為接受已注入的 `mediaSourceService`（檢查現有 struct 欄位後擇一，保持 interface 依賴）。

Run: `grep -n "sourceRepo\|mediaSourceService\|MediaSource" internal/service/video_service.go`
依結果決定用哪個既有依賴；不要新增重複依賴。

- [ ] **Step 2: 寫失敗測試（handler）**

`internal/handler/hls_handler_test.go`（沿用 `video_handler_test.go` 的 mock 風格）:
```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHLSSegment_RejectsInvalidName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &HLSHandler{} // 本 case 在路徑校驗就回 400，不觸及依賴
	r := gin.New()
	r.GET("/api/videos/:id/hls/:segment", h.Segment)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/..%2f..%2fetc%2fpasswd", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHLSSegment_RejectsNonSegmentName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &HLSHandler{}
	r := gin.New()
	r.GET("/api/videos/:id/hls/:segment", h.Segment)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/index.m3u8.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
```

- [ ] **Step 3: 跑測試確認失敗**

Run: `go test ./internal/handler/ -run TestHLSSegment -v`
Expected: FAIL（`HLSHandler` 未定義）

- [ ] **Step 4: 寫 handler 實作**

`internal/handler/hls_handler.go`:
```go
package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
	"github.com/steven/vaultflix/internal/streaming"
)

var segmentNameRe = regexp.MustCompile(`^seg\d{5}\.ts$`)

// HLSHandler 服務即時 remux 的 HLS playlist 與分段。
type HLSHandler struct {
	videoService *service.VideoService
	manager      *streaming.Manager
}

func NewHLSHandler(videoSvc *service.VideoService, mgr *streaming.Manager) *HLSHandler {
	return &HLSHandler{videoService: videoSvc, manager: mgr}
}

// Playlist 啟動/取得 session 並回傳 HLS playlist。
func (h *HLSHandler) Playlist(c *gin.Context) {
	ctx := c.Request.Context()
	videoID := c.Param("id")
	userID := c.GetString("user_id")

	inputPath, err := h.videoService.ResolveDiskPath(ctx, videoID)
	if err != nil {
		h.writePathError(c, videoID, err)
		return
	}

	sess, err := h.manager.EnsureSession(ctx, videoID, userID, inputPath)
	if err != nil {
		slog.Error("failed to ensure hls session", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to start stream",
		})
		return
	}

	playlistPath := filepath.Join(sess.Dir, streaming.PlaylistName)
	// event playlist 由 ffmpeg 漸進寫入；初次可能尚未出現，短暫輪詢。
	if !waitForFile(playlistPath, 10*time.Second) {
		c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{
			Error:   "stream_not_ready",
			Message: "stream is starting, please retry",
		})
		return
	}

	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.Header("Cache-Control", "no-cache")
	c.File(playlistPath)
}

// Segment 回傳指定 .ts 分段。
func (h *HLSHandler) Segment(c *gin.Context) {
	videoID := c.Param("id")
	userID := c.GetString("user_id")
	seg := c.Param("segment")

	if !segmentNameRe.MatchString(seg) {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "invalid segment name",
		})
		return
	}

	dir, ok := h.manager.SessionDir(videoID, userID)
	if !ok {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error:   "not_found",
			Message: "no active stream session",
		})
		return
	}
	h.manager.Touch(videoID, userID)

	segPath := filepath.Join(dir, seg)
	// 二重防護：解析後仍須在 session 目錄內。
	if filepath.Dir(segPath) != filepath.Clean(dir) {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "invalid segment path",
		})
		return
	}
	if _, err := os.Stat(segPath); err != nil {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error:   "not_found",
			Message: "segment not found",
		})
		return
	}

	c.Header("Content-Type", "video/mp2t")
	c.File(segPath)
}

func (h *HLSHandler) writePathError(c *gin.Context, videoID string, err error) {
	switch {
	case errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrPathNotExist):
		c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "video not playable"})
	case errors.Is(err, model.ErrConflict):
		c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: "source_unavailable", Message: "media source disabled"})
	case errors.Is(err, model.ErrPathNotAllowed):
		c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "path_not_allowed", Message: "path outside allowed area"})
	default:
		slog.Error("failed to resolve disk path", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to access video"})
	}
}

func waitForFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
```

- [ ] **Step 5: 跑測試確認通過**

Run: `go test ./internal/handler/ -run TestHLSSegment -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/handler/hls_handler.go internal/handler/hls_handler_test.go internal/service/video_service.go
git commit -m "feat: add HLS playlist/segment handler with path safety

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Auth — stream-scope token 放行 HLS 路由

**Files:**
- Modify: `internal/middleware/auth.go`
- Test: `internal/middleware/stream_scope_test.go`

**Interfaces:**
- Consumes: `model.StreamTokenScope`
- Produces: stream-scope token 可用於 `/api/videos/:id/stream` 及兩個 HLS 路由，且 video_id 須相符

- [ ] **Step 1: 寫失敗測試**

在 `internal/middleware/stream_scope_test.go` 加（沿用該檔既有 helper 建立帶 scope token 的 request；下例示意，依既有測試風格調整）：
```go
func TestStreamScope_AllowsHLSPlaylist(t *testing.T) {
	r := newStreamScopeRouter(t) // 沿用既有 helper
	r.GET("/api/videos/:id/hls/index.m3u8", ok)

	req := streamTokenRequest(t, "GET", "/api/videos/vid123/hls/index.m3u8", "vid123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestStreamScope_AllowsHLSSegment(t *testing.T) {
	r := newStreamScopeRouter(t)
	r.GET("/api/videos/:id/hls/:segment", ok)

	req := streamTokenRequest(t, "GET", "/api/videos/vid123/hls/seg00001.ts", "vid123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
```
（若既有測試沒有 `newStreamScopeRouter`/`streamTokenRequest`/`ok` helper，請參照 `stream_scope_test.go` 既有的 setup 方式建構等效的 request 與 router。）

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./internal/middleware/ -run TestStreamScope_AllowsHLS -v`
Expected: FAIL（HLS 路由回 403）

- [ ] **Step 3: 改 auth.go 放行清單**

在 `internal/middleware/auth.go`，把單一 `streamRoutePath` 常數擴成清單：
```go
// streamRoutePaths 是 scope=stream token 可使用的路由（皆綁定 video_id）。
var streamRoutePaths = map[string]bool{
	"/api/videos/:id/stream":            true,
	"/api/videos/:id/hls/index.m3u8":    true,
	"/api/videos/:id/hls/:segment":      true,
}
```
（移除舊的 `const streamRoutePath = ...`，並更新引用處。）

把 scope 檢查改為：
```go
		if scope, _ := claims["scope"].(string); scope == model.StreamTokenScope {
			boundVideoID, _ := claims["video_id"].(string)
			if !streamRoutePaths[c.FullPath()] || boundVideoID == "" || boundVideoID != c.Param("id") {
				c.AbortWithStatusJSON(http.StatusForbidden, model.ErrorResponse{
					Error:   "forbidden",
					Message: "stream token cannot access this resource",
				})
				return
			}
		}
```

- [ ] **Step 4: 跑全部 middleware 測試確認通過**

Run: `go test ./internal/middleware/ -v`
Expected: PASS（含既有 stream scope 測試與新 HLS 測試）

- [ ] **Step 5: Commit**

```bash
git add internal/middleware/auth.go internal/middleware/stream_scope_test.go
git commit -m "feat: allow stream-scope token on HLS routes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: Config + Docker volume + 路由 wiring

**Files:**
- Modify: `internal/config/config.go`
- Modify: `docker-compose.yml`、`docker-compose.prod.yml`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `streaming.NewManager`/`NewFFmpegTranscoder`、`handler.NewHLSHandler`
- Produces: `Config.TranscodeCacheDir`；HLS 路由註冊；sweeper 啟動

- [ ] **Step 1: 加 config 欄位**

在 `internal/config/config.go` 的 `Config` struct 加 `TranscodeCacheDir string`，並在建構處加：
```go
		TranscodeCacheDir: getEnv("TRANSCODE_CACHE_DIR", "/var/cache/vaultflix/transcode"),
```

- [ ] **Step 2: compose 加可寫 volume**

在 `docker-compose.yml` 的 `vaultflix-api` service `volumes:` 區塊加：
```yaml
      - vaultflix-transcode-cache:/var/cache/vaultflix/transcode
```
並在檔尾 `volumes:` 頂層加：
```yaml
  vaultflix-transcode-cache:
```
`docker-compose.prod.yml` 比照（若 prod 用 `!override` 機制覆寫 volumes，依該檔現有模式加入對等定義，不複製其他 infra）。
Run: `grep -n "volumes:" docker-compose.yml docker-compose.prod.yml`
依結果在正確位置加入。

- [ ] **Step 3: main.go wire + 註冊路由 + 啟動 sweeper**

在 `cmd/server/main.go`：

(a) 建立 manager（放在 videoHandler 建立附近）：
```go
	transcoder := streaming.NewFFmpegTranscoder()
	streamManager := streaming.NewManager(transcoder, cfg.TranscodeCacheDir, 60*time.Second)
	streamManager.StartSweeper(context.Background())
	hlsHandler := handler.NewHLSHandler(videoService, streamManager)
```
確認已 import `"context"`、`"time"`、`"github.com/steven/vaultflix/internal/streaming"`。

(b) 在 video 端點區塊（`api.GET("/videos/:id/stream", ...)` 附近）加：
```go
		api.GET("/videos/:id/hls/index.m3u8", hlsHandler.Playlist)
		api.GET("/videos/:id/hls/:segment", hlsHandler.Segment)
```

- [ ] **Step 4: 編譯 + verify**

Run: `go build ./... && task verify`
Expected: build 成功；`task verify` 綠

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go docker-compose.yml docker-compose.prod.yml cmd/server/main.go
git commit -m "feat: wire HLS streaming manager, routes, and transcode cache volume

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 12: Codec backfill（補掃既有影片）

**Files:**
- Create: `internal/service/codec_backfill_service.go`
- Create: `internal/handler/codec_backfill_handler.go`
- Modify: `cmd/server/main.go`（wire + admin 路由）
- Modify: `internal/repository/video_repo.go`（加 `UpdateCodecs` 與一個列出缺 codec 影片的 query）
- Test: `internal/service/codec_backfill_service_test.go`

**Interfaces:**
- Consumes: `videoRepo`、`sourceRepo`、ffprobe（透過可注入的 probe func）
- Produces:
  - `VideoRepository.UpdateCodecs(ctx, id, videoCodec, audioCodec string) error`
  - `VideoRepository.ListMissingCodecs(ctx, limit int) ([]model.Video, error)`（`video_codec IS NULL OR video_codec = ''`，且 source_id/file_path 非空）
  - `CodecBackfillService.Run(ctx) (processed int, failed int, err error)`
  - `POST /api/admin/videos/backfill-codecs`（admin）

- [ ] **Step 1: 加 repo 方法**

在 `internal/repository/video_repo.go` 加 const query 與方法：
```go
const queryUpdateCodecs = `
	UPDATE videos
	SET video_codec = $2, audio_codec = $3, updated_at = NOW()
	WHERE id = $1
`

const queryListMissingCodecs = `
	SELECT v.id, v.original_filename, v.source_id, v.file_path
	FROM videos v
	WHERE (v.video_codec IS NULL OR v.video_codec = '')
	  AND v.source_id IS NOT NULL AND v.file_path IS NOT NULL
	LIMIT $1
`
```
實作 `UpdateCodecs`（`Exec`）與 `ListMissingCodecs`（scan id/original_filename/source_id/file_path 進 `model.Video`）。將兩方法加進 `VideoRepository` interface 與 `internal/mock/video_repo_mock.go`。

- [ ] **Step 2: 寫失敗測試（service，mock probe）**

`internal/service/codec_backfill_service_test.go`:
```go
package service

import (
	"context"
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func TestCodecBackfill_Run_UpdatesMissing(t *testing.T) {
	src := "s1"
	fp := "movie.mkv"
	repo := &fakeCodecRepo{
		missing: []model.Video{{ID: "v1", OriginalFilename: "movie.mkv", SourceID: &src, FilePath: &fp}},
	}
	svc := NewCodecBackfillService(repo, &fakeSourceRepo{mount: "/mnt/host/D"})
	svc.probe = func(_ context.Context, _ string) (string, string, error) {
		return "h264", "aac", nil
	}

	processed, failed, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if processed != 1 || failed != 0 {
		t.Errorf("processed=%d failed=%d, want 1/0", processed, failed)
	}
	if repo.updated["v1"] != [2]string{"h264", "aac"} {
		t.Errorf("v1 codecs = %v, want [h264 aac]", repo.updated["v1"])
	}
}
```
（在測試檔內定義 `fakeCodecRepo`/`fakeSourceRepo` 滿足所需 interface 子集；參照 `internal/mock/` 既有風格。）

- [ ] **Step 3: 跑測試確認失敗**

Run: `go test ./internal/service/ -run TestCodecBackfill -v`
Expected: FAIL（`NewCodecBackfillService` 未定義）

- [ ] **Step 4: 寫 service 實作**

`internal/service/codec_backfill_service.go`:
```go
package service

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// codecProbeFunc 回傳 (videoCodec, audioCodec, error)。
type codecProbeFunc func(ctx context.Context, absPath string) (string, string, error)

// CodecBackfillService 補掃缺 codec 的既有影片。
type CodecBackfillService struct {
	videoRepo  repository.VideoRepository
	sourceRepo repository.MediaSourceRepository
	probe      codecProbeFunc
}

func NewCodecBackfillService(v repository.VideoRepository, s repository.MediaSourceRepository) *CodecBackfillService {
	return &CodecBackfillService{videoRepo: v, sourceRepo: s, probe: probeCodecs}
}

// Run 補掃所有缺 codec 的影片，回傳成功/失敗數。
func (s *CodecBackfillService) Run(ctx context.Context) (int, int, error) {
	videos, err := s.videoRepo.ListMissingCodecs(ctx, 10000)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list videos missing codecs: %w", err)
	}

	processed, failed := 0, 0
	for _, v := range videos {
		source, err := s.sourceRepo.GetByID(ctx, *v.SourceID)
		if err != nil {
			slog.Warn("backfill: source lookup failed", "video_id", v.ID, "error", err)
			failed++
			continue
		}
		abs := filepath.Clean(filepath.Join(source.MountPath, *v.FilePath))
		vc, ac, err := s.probe(ctx, abs)
		if err != nil {
			slog.Warn("backfill: probe failed", "video_id", v.ID, "error", err)
			failed++
			continue
		}
		if err := s.videoRepo.UpdateCodecs(ctx, v.ID, vc, ac); err != nil {
			slog.Warn("backfill: update failed", "video_id", v.ID, "error", err)
			failed++
			continue
		}
		processed++
	}
	slog.Info("codec backfill complete", "processed", processed, "failed", failed)
	return processed, failed, nil
}

func probeCodecs(ctx context.Context, absPath string) (string, string, error) {
	run := func(stream string) (string, error) {
		cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error",
			"-select_streams", stream, "-show_entries", "stream=codec_name",
			"-of", "default=nw=1:nk=1", absPath)
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("ffprobe %s failed: %w", stream, err)
		}
		return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), nil
	}
	vc, err := run("v:0")
	if err != nil {
		return "", "", err
	}
	ac, _ := run("a:0") // 無音軌可接受
	return vc, ac, nil
}
```

- [ ] **Step 5: 跑測試確認通過**

Run: `go test ./internal/service/ -run TestCodecBackfill -v`
Expected: PASS

- [ ] **Step 6: handler + 路由 + wiring**

`internal/handler/codec_backfill_handler.go`:
```go
package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

type CodecBackfillHandler struct {
	svc *service.CodecBackfillService
}

func NewCodecBackfillHandler(svc *service.CodecBackfillService) *CodecBackfillHandler {
	return &CodecBackfillHandler{svc: svc}
}

// Run 同步補掃缺 codec 的影片（admin only）。
func (h *CodecBackfillHandler) Run(c *gin.Context) {
	processed, failed, err := h.svc.Run(c.Request.Context())
	if err != nil {
		slog.Error("codec backfill failed", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "codec backfill failed",
		})
		return
	}
	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{"processed": processed, "failed": failed},
	})
}
```
在 `cmd/server/main.go` wire 並在 admin 區塊（`backfill-previews` 附近）註冊：
```go
	codecBackfillService := service.NewCodecBackfillService(videoRepo, mediaSourceRepo)
	codecBackfillHandler := handler.NewCodecBackfillHandler(codecBackfillService)
	// ...
		api.POST("/admin/videos/backfill-codecs", codecBackfillHandler.Run)
```

- [ ] **Step 7: verify**

Run: `go build ./... && task verify`
Expected: build 成功；`task verify` 綠

- [ ] **Step 8: Commit**

```bash
git add internal/service/codec_backfill_service.go internal/service/codec_backfill_service_test.go internal/handler/codec_backfill_handler.go internal/repository/video_repo.go internal/mock/video_repo_mock.go cmd/server/main.go
git commit -m "feat: add codec backfill for existing videos

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 13: 前端 — hls.js + PlayerPage 依 play_mode 分流

**Files:**
- Modify: `web/package.json`
- Modify: `web/src/types/index.ts`
- Modify: `web/src/pages/PlayerPage.tsx`
- Test: `web/src/pages/PlayerPage.test.tsx`（新增；若無測試基建則加 vitest 測試）

**Interfaces:**
- Consumes: `VideoDetail.play_mode`、`getStreamToken`、`video.stream_url`
- Produces: `direct` 走原生 `<video src>`；`remux` 用 hls.js 掛 `/api/videos/:id/hls/index.m3u8?token=`；`transcode` 顯示提示

- [ ] **Step 1: 加 hls.js 依賴**

Run: `cd web && npm install hls.js@^1.5.0`
Expected: `package.json` dependencies 出現 `hls.js`

- [ ] **Step 2: 加 play_mode 型別**

在 `web/src/types/index.ts` 的 `VideoDetail` interface 加：
```ts
  play_mode: 'direct' | 'remux' | 'transcode'
```

- [ ] **Step 3: 寫失敗測試（transcode 顯示提示）**

`web/src/pages/PlayerPage.test.tsx`（沿用既有 vitest + React Testing Library 設定；mock api 模組）：
```tsx
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import PlayerPage from './PlayerPage'
import * as videosApi from '../api/videos'

vi.mock('../api/videos')

const base = {
  id: 'v1', title: 'T', description: '', tags: [], resolution: '1920x1080',
  duration_seconds: 10, file_size_bytes: 1, created_at: '2026-06-30T00:00:00Z',
  stream_url: '/api/videos/v1/stream', is_favorited: false, watch_progress: 0,
}

function renderPlayer() {
  return render(
    <MemoryRouter initialEntries={['/watch/v1']}>
      <Routes><Route path="/watch/:id" element={<PlayerPage />} /></Routes>
    </MemoryRouter>,
  )
}

describe('PlayerPage play_mode', () => {
  beforeEach(() => {
    vi.mocked(videosApi.getStreamToken).mockResolvedValue({ token: 'tok', expires_in: 60 })
    vi.mocked(videosApi.listVideos).mockResolvedValue({ data: [], total: 0, page: 1, page_size: 12 } as never)
  })

  it('shows a notice for transcode videos instead of a player', async () => {
    vi.mocked(videosApi.getVideo).mockResolvedValue({ ...base, play_mode: 'transcode' } as never)
    renderPlayer()
    expect(await screen.findByText(/尚未支援|Phase 2|無法播放/)).toBeInTheDocument()
  })
})
```

- [ ] **Step 4: 跑測試確認失敗**

Run: `cd web && npx vitest run src/pages/PlayerPage.test.tsx`
Expected: FAIL（找不到提示文字）

- [ ] **Step 5: 改 PlayerPage 分流播放**

在 `web/src/pages/PlayerPage.tsx`：

(a) 加 import：
```tsx
import Hls from 'hls.js'
```

(b) 新增 effect：當 `play_mode === 'remux'` 時用 hls.js 掛載 HLS（取代「streamToken 變動就 `videoRef.current.load()`」對 remux 的處理）。在現有 streamToken effect 之後加：
```tsx
  // remux 影片用 hls.js 播放即時 HLS；direct 影片維持原生 src。
  useEffect(() => {
    if (!video || video.play_mode !== 'remux' || !streamToken || !videoRef.current) return
    const el = videoRef.current
    const url = `/api/videos/${video.id}/hls/index.m3u8?token=${streamToken}`

    // Safari 原生支援 HLS。
    if (el.canPlayType('application/vnd.apple.mpegurl')) {
      el.src = url
      return
    }
    if (!Hls.isSupported()) {
      setError('此瀏覽器不支援串流播放')
      return
    }
    const hls = new Hls()
    hls.loadSource(url)
    hls.attachMedia(el)
    return () => {
      hls.destroy()
    }
  }, [video, streamToken])
```

(c) 把 `<video>` 的 `src` 只在 `direct` 模式設定（remux 由上面 effect 設定）：
```tsx
              <video
                ref={videoRef}
                controls
                preload="metadata"
                src={
                  video.play_mode === 'direct' && streamToken
                    ? `${video.stream_url}?token=${streamToken}`
                    : undefined
                }
                className="aspect-video w-full"
                onError={handleVideoError}
                onTimeUpdate={handleTimeUpdate}
                onPause={handlePause}
                onLoadedMetadata={handleLoadedMetadata}
                onVolumeChange={handleVolumeChange}
              />
```

(d) 在 player 容器前，當 `play_mode === 'transcode'` 時顯示提示而非播放器。把 `<div className="relative overflow-hidden rounded-lg bg-black">...</div>` 包成條件：
```tsx
            {video.play_mode === 'transcode' ? (
              <div className="flex aspect-video w-full items-center justify-center rounded-lg bg-surface text-center text-sm text-muted">
                <div className="px-6">
                  此影片格式（{video.video_codec || '未知編碼'}）尚未支援線上播放，
                  將於後續版本（Phase 2 轉碼）支援。
                </div>
              </div>
            ) : (
              <div className="relative overflow-hidden rounded-lg bg-black">
                {/* 既有 <video> 區塊 */}
              </div>
            )}
```
（若 `video_codec` 未在前端型別，於 `types/index.ts` 的 `Video` 加 `video_codec?: string`。）

(e) `handleVideoError` 對 remux 模式的 token 過期重試仍適用（重試會重設 streamToken，觸發上面 effect 重新 attach），維持既有重試上限邏輯不變。

- [ ] **Step 6: 跑測試確認通過**

Run: `cd web && npx vitest run src/pages/PlayerPage.test.tsx`
Expected: PASS

- [ ] **Step 7: 前端 gate**

Run: `cd web && npm run lint && npx tsc --noEmit && npx vitest run`
Expected: 全綠（zero eslint error/warning）

- [ ] **Step 8: Commit**

```bash
git add web/package.json web/package-lock.json web/src/types/index.ts web/src/pages/PlayerPage.tsx web/src/pages/PlayerPage.test.tsx
git commit -m "feat: branch player by play_mode, add hls.js for remux streaming

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 14: 整合測試 — mkv/h264 remux → HLS 播放鏈

**Files:**
- Modify: `scripts/test_all.sh`（或既有整合測試 fixture 目錄與測試）
- Create: 小型 fixture `testdata/fixtures/sample_h264.mkv`（用 ffmpeg 產生 2 秒測試片）
- Create/Modify: 整合測試（沿用 `enrich` 整合測試的放置方式）

**Interfaces:**
- Consumes: 完整 stack（`task test-integration`）
- Produces: 驗證 import → play_mode=remux → HLS playlist 200 + 至少一個分段可取得

- [ ] **Step 1: 產生 fixture**

Run:
```bash
docker compose exec -T vaultflix-api sh -c 'ffmpeg -hide_banner -f lavfi -i testsrc=size=320x240:rate=15:duration=2 -f lavfi -i sine=frequency=440:duration=2 -c:v libx264 -c:a aac -f matroska /tmp/sample_h264.mkv && ls -l /tmp/sample_h264.mkv'
```
Expected: 產出 `.mkv`。把它複製到 repo fixture 路徑：
```bash
docker compose cp vaultflix-api:/tmp/sample_h264.mkv testdata/fixtures/sample_h264.mkv
```
（fixture 目錄若不同，依既有整合測試的 fixture 放置慣例調整。）

- [ ] **Step 2: 寫整合測試步驟（在 scripts/test_all.sh 或對應 Go 整合測試）**

新增一段，模擬登入取得 token，對一個已知 remux 影片：
1. `GET /api/videos/:id` → 斷言 `data.play_mode == "remux"`
2. `GET /api/videos/:id/stream-token` → 取 stream token
3. `GET /api/videos/:id/hls/index.m3u8?token=...` → 斷言 200 且 body 含 `#EXTM3U`
4. 從 playlist 解出第一個 `seg00000.ts`，`GET /api/videos/:id/hls/seg00000.ts?token=...` → 斷言 200

依 `scripts/test_all.sh` 既有風格（curl + jq + 斷言）撰寫。先讀該檔了解既有斷言 helper：
Run: `sed -n '1,40p' scripts/test_all.sh`

- [ ] **Step 3: 跑整合測試**

Run: `task test-integration`
Expected: 綠（含新 HLS 斷言）

- [ ] **Step 4: Commit**

```bash
git add testdata/fixtures/sample_h264.mkv scripts/test_all.sh
git commit -m "test: integration coverage for mkv remux to HLS playback chain

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 15: 全量驗證 + backfill 既有片庫 + 收尾

**Files:** 無新增（驗證與資料補掃）

- [ ] **Step 1: 全量驗證**

Run: `task verify && task test-integration`
Expected: 皆綠

- [ ] **Step 2: 部署並 backfill 既有 499 部 codec**

Run:
```bash
task deploy
# 取得 admin token 後（依既有登入流程）：
curl -s -X POST http://localhost:3000/api/admin/videos/backfill-codecs -H "Authorization: Bearer <ADMIN_TOKEN>"
```
Expected: 回 `{"data":{"processed":N,"failed":M}}`，N 接近 498。

- [ ] **Step 3: 抽查 remux 播放**

在瀏覽器開一部已知 `avi/h264/aac` 或 `mkv/h264/aac` 影片，確認能播放且可拖曳（seek）。`transcode` 影片確認顯示提示。

- [ ] **Step 4: 用 superpowers:finishing-a-development-branch 收尾（PR）**

開 PR，等 CI 綠後合併。

---

## Self-Review

**Spec coverage：**
- 分類器 → Task 3 ✓
- codec 欄位 migration + probe + 修 MimeType → Task 1, 4 ✓
- repo 持久化 → Task 5 ✓
- play_mode 進 VideoDetail → Task 6 ✓
- 即時 HLS（session manager、transcoder、暫存、idle 清理）→ Task 7, 8 ✓
- HLS handler + 路徑安全 → Task 9 ✓
- 串流授權（stream token 放行 HLS）→ Task 10 ✓
- compose 可寫 volume + config + wiring → Task 11 ✓
- backfill 既有影片 → Task 12, 15 ✓
- 前端 hls.js + play_mode 分流 + transcode 提示 → Task 13 ✓
- 整合測試 mkv→HLS → Task 14 ✓
- 明確排除 transcode/NVENC/segment-on-demand → 未進任何 task ✓（Phase 2）

**Placeholder scan：** 各 task 皆有實際程式碼或實際 SQL/指令；少數步驟要求先 `grep` 確認既有 helper 名稱（因既有測試 helper 命名需現場核對），非 placeholder 而是「對既有程式碼的精確對齊」。

**Type consistency：**
- `model.PlayMode` 與三常數（Task 2）在 Task 3/6/13 一致使用。
- `ClassifyPlayMode(container, videoCodec, audioCodec)` 簽名在 Task 3 定義，Task 4/6 呼叫一致。
- `Transcoder.Start(ctx, inputPath, outDir)`、`TranscodeProc.Stop()/Done()`（Task 7）在 Task 8 manager 與測試一致使用。
- `Manager.EnsureSession/Touch/SessionDir/Sweep/StartSweeper`（Task 8）在 Task 9/11 一致使用。
- `streaming.PlaylistName`（Task 7）在 Task 8/9 一致使用。
- `VideoService.ResolveDiskPath`（Task 9）由 HLS handler 使用；同時建議 `Stream()` 改用以避免漂移。
- `VideoRepository.UpdateCodecs/ListMissingCodecs`（Task 12）在 service 與 mock 一致。
