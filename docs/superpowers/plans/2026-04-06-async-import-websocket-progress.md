# Phase 12: 非同步匯入 + WebSocket 進度推送 — 實作計畫

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 將同步阻塞的影片匯入改為非同步 — API 立即回傳 job ID，背景 goroutine 執行匯入，透過 WebSocket 推送逐檔進度給前端。

**Architecture:** Import service 新增 `Notifier` 依賴，用 `sync.Map` 管理 in-memory job 狀態、`sync.Mutex` 限制單一 job 執行。Handler 回傳 202 Accepted。前端 `VideoManagePage` 的匯入 modal 改用 media source 下拉 + WebSocket 即時進度。

**Tech Stack:** Go 1.22+, Gin, gorilla/websocket, React 18 + TypeScript, axios

---

## 檔案結構

| 動作 | 路徑 | 職責 |
|------|------|------|
| Create | `internal/model/import_job.go` | ImportJob / ImportError / ImportProgress 型別 |
| Modify | `internal/service/import_service.go` | 新增 Notifier 依賴、StartAsync、runImport、processOneFile、GetJob、GetActiveJob |
| Create | `internal/service/import_service_test.go` | Import service 測試 |
| Create | `internal/mock/notifier_mock.go` | mock Notifier |
| Modify | `internal/handler/video_handler.go` | Import handler 改 202、新增 GetImportJob / GetActiveImportJob |
| Modify | `internal/handler/video_handler_test.go` | 新增 handler 測試 |
| Modify | `internal/handler/ws_handler.go` | 移除 TestSend 方法 |
| Modify | `cmd/server/main.go` | DI 注入 notifier、新增路由、移除 ws-test 路由 |
| Modify | `web/src/types/index.ts` | 新增 ImportJob / ImportProgress / MediaSource 型別 |
| Modify | `web/src/api/admin.ts` | 改 importVideos、新增 getActiveImportJob / getImportJob / listMediaSources |
| Modify | `web/src/pages/admin/VideoManagePage.tsx` | 匯入 modal 改為非同步進度 UI |

---

## Task 1: Import Job Model

**Files:**
- Create: `internal/model/import_job.go`

- [ ] **Step 1: 建立 ImportJob 相關型別**

```go
// internal/model/import_job.go
package model

import "time"

// ImportJob 代表一次匯入作業的狀態（in-memory，不持久化）。
type ImportJob struct {
	ID          string        `json:"id"`
	SourceID    string        `json:"source_id"`
	SourceLabel string        `json:"source_label"`
	Status      string        `json:"status"`
	Total       int           `json:"total"`
	Processed   int           `json:"processed"`
	Imported    int           `json:"imported"`
	Skipped     int           `json:"skipped"`
	Failed      int           `json:"failed"`
	Errors      []ImportError `json:"errors"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  *time.Time    `json:"finished_at,omitempty"`
}

// ImportError 記錄單一檔案的匯入失敗資訊。
type ImportError struct {
	FileName string `json:"file_name"`
	Error    string `json:"error"`
}

// ImportProgress 是透過 WebSocket 推送的逐檔進度訊息。
type ImportProgress struct {
	JobID    string `json:"job_id"`
	FileName string `json:"file_name"`
	Current  int    `json:"current"`
	Total    int    `json:"total"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}
```

- [ ] **Step 2: 確認編譯通過**

```bash
cd /d/Vaultflix && docker compose exec api go build ./...
```

Expected: 編譯成功，無錯誤。

- [ ] **Step 3: Commit**

```bash
git add internal/model/import_job.go
git commit -m "feat: add ImportJob, ImportError, ImportProgress model types"
```

---

## Task 2: Mock Notifier

**Files:**
- Create: `internal/mock/notifier_mock.go`

- [ ] **Step 1: 建立 mock Notifier**

```go
// internal/mock/notifier_mock.go
package mock

import (
	"sync"

	"github.com/steven/vaultflix/internal/websocket"
)

// Notifier is a thread-safe mock for websocket.Notifier.
type Notifier struct {
	mu       sync.Mutex
	Messages []websocket.Message
}

func (m *Notifier) SendToUser(userID string, msg *websocket.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, *msg)
}

func (m *Notifier) Broadcast(msg *websocket.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, *msg)
}

// GetMessages returns a snapshot of all captured messages.
func (m *Notifier) GetMessages() []websocket.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]websocket.Message, len(m.Messages))
	copy(cp, m.Messages)
	return cp
}
```

- [ ] **Step 2: 確認編譯通過**

```bash
cd /d/Vaultflix && docker compose exec api go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/mock/notifier_mock.go
git commit -m "feat: add mock Notifier for testing WebSocket pushes"
```

---

## Task 3: 重構 Import Service — 非同步核心

**Files:**
- Modify: `internal/service/import_service.go`

這是最大的 task，將現有同步 `Run` 改為非同步 `StartAsync` + `runImport` + `processOneFile`。

- [ ] **Step 1: 修改 ImportService struct 和 constructor**

在 `internal/service/import_service.go` 中：

1. 移除 `ImportFailure` 和 `ImportResult` 型別（已被 `model.ImportJob` / `model.ImportError` 取代）
2. 修改 `ImportService` struct，新增 `notifier`、`activeJobs`、`importMu`
3. 修改 constructor

```go
// 移除舊的 ImportFailure 和 ImportResult 型別

// 新增 import
// "sync"
// "time"
// "github.com/steven/vaultflix/internal/websocket"

type fileResult struct {
	Status string // "success", "skipped", "error"
	Error  string
}

type ImportService struct {
	videoRepo  repository.VideoRepository
	minioSvc   MinIOClient
	notifier   websocket.Notifier
	activeJobs sync.Map
	importMu   sync.Mutex
}

func NewImportService(videoRepo repository.VideoRepository, minioSvc MinIOClient, notifier websocket.Notifier) *ImportService {
	return &ImportService{
		videoRepo: videoRepo,
		minioSvc:  minioSvc,
		notifier:  notifier,
	}
}
```

- [ ] **Step 2: 新增 StartAsync 方法**

```go
// StartAsync 建立 job 並啟動背景匯入，立即回傳 job 資訊。
// 同一時間只允許一個匯入 job 執行，重複呼叫回傳 model.ErrConflict。
func (s *ImportService) StartAsync(ctx context.Context, source *model.MediaSource, userID string) (*model.ImportJob, error) {
	if !s.importMu.TryLock() {
		return nil, model.ErrConflict
	}

	job := &model.ImportJob{
		ID:          uuid.New().String(),
		SourceID:    source.ID,
		SourceLabel: source.Label,
		Status:      "running",
		Errors:      []model.ImportError{},
		StartedAt:   time.Now(),
	}
	s.activeJobs.Store(job.ID, job)

	go func() {
		defer s.importMu.Unlock()
		s.runImport(context.Background(), job, source, userID)
	}()

	return job, nil
}
```

- [ ] **Step 3: 將現有 Run 方法重構為 runImport**

移除舊的 `Run` 方法，新增 `runImport`（private）。`runImport` 使用現有的 `scanVideoFiles` + 新的 `processOneFile`，並在迴圈中推送 WebSocket 訊息。

```go
func (s *ImportService) runImport(ctx context.Context, job *model.ImportJob, source *model.MediaSource, userID string) {
	defer func() {
		now := time.Now()
		job.FinishedAt = &now
		if job.Failed > 0 && job.Imported == 0 {
			job.Status = "failed"
		} else {
			job.Status = "completed"
		}
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeImportComplete,
			Payload: job,
		})
	}()

	files, err := s.scanVideoFiles(source.MountPath)
	if err != nil {
		job.Status = "failed"
		job.Errors = append(job.Errors, model.ImportError{
			FileName: source.MountPath,
			Error:    err.Error(),
		})
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeImportError,
			Payload: map[string]string{"job_id": job.ID, "error": err.Error()},
		})
		return
	}

	job.Total = len(files)

	for i, filePath := range files {
		fileName := filepath.Base(filePath)

		s.notifier.SendToUser(userID, &websocket.Message{
			Type: websocket.TypeImportProgress,
			Payload: model.ImportProgress{
				JobID:    job.ID,
				FileName: fileName,
				Current:  i + 1,
				Total:    job.Total,
				Status:   "processing",
			},
		})

		result := s.processOneFile(ctx, source, filePath)

		job.Processed = i + 1
		switch result.Status {
		case "success":
			job.Imported++
		case "skipped":
			job.Skipped++
		case "error":
			job.Failed++
			job.Errors = append(job.Errors, model.ImportError{
				FileName: fileName,
				Error:    result.Error,
			})
		}

		s.notifier.SendToUser(userID, &websocket.Message{
			Type: websocket.TypeImportProgress,
			Payload: model.ImportProgress{
				JobID:    job.ID,
				FileName: fileName,
				Current:  i + 1,
				Total:    job.Total,
				Status:   result.Status,
				Error:    result.Error,
			},
		})
	}

	slog.Info("import completed",
		"source_id", source.ID,
		"source_label", source.Label,
		"total_scanned", job.Total,
		"imported", job.Imported,
		"skipped", job.Skipped,
		"failed", job.Failed,
	)
}
```

- [ ] **Step 4: 將現有 processFile 重構為 processOneFile**

將現有的 `processFile` 方法改名為 `processOneFile`，回傳 `fileResult` 而非 error。移除舊的 `skipError` 型別。

```go
func (s *ImportService) processOneFile(ctx context.Context, source *model.MediaSource, filePath string) fileResult {
	filename := filepath.Base(filePath)

	stat, err := os.Stat(filePath)
	if err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to stat file %s: %v", filename, err)}
	}
	fileSize := stat.Size()

	relPath, err := filepath.Rel(source.MountPath, filePath)
	if err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to calculate relative path for %s: %v", filename, err)}
	}
	relPath = filepath.ToSlash(relPath)

	_, err = s.videoRepo.FindBySourceAndPath(ctx, source.ID, relPath)
	if err == nil {
		slog.Info("video skipped, already imported",
			"file", filename,
			"source_id", source.ID,
			"file_path", relPath,
		)
		return fileResult{Status: "skipped"}
	}
	if !errors.Is(err, model.ErrNotFound) {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to check duplicate for %s: %v", filename, err)}
	}

	metadata, err := s.probeMetadata(ctx, filePath)
	if err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to probe metadata for %s: %v", filename, err)}
	}

	videoID := uuid.New().String()

	thumbnailPath, err := s.generateThumbnail(ctx, filePath, metadata.durationSeconds)
	if err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to generate thumbnail for %s: %v", filename, err)}
	}
	defer os.Remove(thumbnailPath)

	thumbnailObjectKey := fmt.Sprintf("thumbnails/%s.jpg", videoID)

	if err := s.minioSvc.UploadThumbnail(ctx, thumbnailObjectKey, thumbnailPath); err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to upload thumbnail for %s: %v", filename, err)}
	}

	title := strings.TrimSuffix(filename, filepath.Ext(filename))

	video := &model.Video{
		ID:               videoID,
		Title:            title,
		Description:      "",
		MinIOObjectKey:   "",
		ThumbnailKey:     thumbnailObjectKey,
		DurationSeconds:  metadata.durationSeconds,
		Resolution:       metadata.resolution,
		FileSizeBytes:    fileSize,
		MimeType:         metadata.mimeType,
		OriginalFilename: filename,
		SourceID:         &source.ID,
		FilePath:         &relPath,
	}

	if err := s.videoRepo.Create(ctx, video); err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to save video record for %s: %v", filename, err)}
	}

	slog.Info("video imported",
		"video_id", videoID,
		"file", filename,
		"source_id", source.ID,
		"file_path", relPath,
		"duration", metadata.durationSeconds,
		"resolution", metadata.resolution,
		"size_bytes", fileSize,
	)

	return fileResult{Status: "success"}
}
```

同時移除舊的 `processFile`、`skipError`、`isSkipError`、`Run`、`ImportFailure`、`ImportResult`。

- [ ] **Step 5: 新增 GetJob 和 GetActiveJob 方法**

```go
// GetJob 取得 job 狀態。找不到回傳 model.ErrNotFound。
func (s *ImportService) GetJob(jobID string) (*model.ImportJob, error) {
	val, ok := s.activeJobs.Load(jobID)
	if !ok {
		return nil, model.ErrNotFound
	}
	return val.(*model.ImportJob), nil
}

// GetActiveJob 取得目前進行中的 job（如果有）。
// 無進行中的 job 時回傳 nil。
func (s *ImportService) GetActiveJob() *model.ImportJob {
	var active *model.ImportJob
	s.activeJobs.Range(func(key, value interface{}) bool {
		job := value.(*model.ImportJob)
		if job.Status == "running" {
			active = job
			return false
		}
		return true
	})
	return active
}
```

- [ ] **Step 6: 確認編譯通過**

注意：此時 `main.go` 和 `video_handler.go` 會因為 constructor 簽名變更而編譯失敗，這是預期的。先確認 `import_service.go` 本身沒有語法錯誤：

```bash
cd /d/Vaultflix && docker compose exec api go build ./internal/service/
```

如果因為 `main.go` 的引用導致間接失敗，可以暫時跳過，在 Task 5 修改 main.go 後統一驗證。

- [ ] **Step 7: Commit**

```bash
git add internal/service/import_service.go
git commit -m "refactor: convert import service from sync to async with WebSocket progress"
```

---

## Task 4: Import Service 測試

**Files:**
- Create: `internal/service/import_service_test.go`

Import service 因為依賴 `ffprobe` / `ffmpeg` 等外部指令，直接測試 `processOneFile` 不太實際。測試重點放在：StartAsync 的並發控制、GetJob/GetActiveJob 查詢、以及 runImport 的進度推送（透過注入可控的 mock）。

因為 `runImport` 內部呼叫 `scanVideoFiles` 和 `processOneFile` 是真實的檔案系統操作，需要用臨時目錄 + 假檔案。但 `ffprobe` 在測試環境不一定可用，所以我們只測試不需要 ffprobe 的場景（空目錄、不存在的目錄、已存在的檔案 skip）以及 StartAsync 的並發控制。

- [ ] **Step 1: 寫 TestGetJob_Found 和 TestGetJob_NotFound**

```go
// internal/service/import_service_test.go
package service

import (
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestGetJob_Found(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	job := &model.ImportJob{
		ID:     "job-123",
		Status: "running",
	}
	svc.activeJobs.Store(job.ID, job)

	got, err := svc.GetJob("job-123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.ID != "job-123" {
		t.Errorf("expected job ID job-123, got %s", got.ID)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	_, err := svc.GetJob("nonexistent")
	if err != model.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: 執行測試**

```bash
cd /d/Vaultflix && docker compose exec api go test ./internal/service/ -run TestGetJob -v
```

Expected: 2 個測試通過。

- [ ] **Step 3: 寫 TestGetActiveJob_Running 和 TestGetActiveJob_None**

```go
func TestGetActiveJob_Running(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	job := &model.ImportJob{
		ID:     "job-active",
		Status: "running",
	}
	svc.activeJobs.Store(job.ID, job)

	// 也存一個已完成的 job
	completed := &model.ImportJob{
		ID:     "job-done",
		Status: "completed",
	}
	svc.activeJobs.Store(completed.ID, completed)

	got := svc.GetActiveJob()
	if got == nil {
		t.Fatal("expected active job, got nil")
	}
	if got.ID != "job-active" {
		t.Errorf("expected job-active, got %s", got.ID)
	}
}

func TestGetActiveJob_None(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	// 只存已完成的 job
	completed := &model.ImportJob{
		ID:     "job-done",
		Status: "completed",
	}
	svc.activeJobs.Store(completed.ID, completed)

	got := svc.GetActiveJob()
	if got != nil {
		t.Errorf("expected nil, got job %s", got.ID)
	}
}
```

- [ ] **Step 4: 執行測試**

```bash
cd /d/Vaultflix && docker compose exec api go test ./internal/service/ -run TestGetActiveJob -v
```

Expected: 2 個測試通過。

- [ ] **Step 5: 寫 TestStartAsync_Conflict**

```go
func TestStartAsync_Conflict(t *testing.T) {
	notifier := &mock.Notifier{}
	videoRepo := &mock.VideoRepository{}
	svc := NewImportService(videoRepo, &mock.MinIOClient{}, notifier)

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Test Source",
		MountPath: t.TempDir(), // 空目錄，runImport 會很快完成
	}

	// 第一次呼叫成功
	job1, err := svc.StartAsync(nil, source, "user-1")
	if err != nil {
		t.Fatalf("first StartAsync should succeed, got %v", err)
	}
	if job1.Status != "running" {
		t.Errorf("expected status running, got %s", job1.Status)
	}

	// 等待背景 goroutine 取得 lock 後，第二次呼叫應該回傳 ErrConflict
	// 但因為空目錄會很快完成，我們先手動 lock
	svc.importMu.Lock()

	_, err = svc.StartAsync(nil, source, "user-1")
	if err != model.ErrConflict {
		t.Errorf("expected ErrConflict, got %v", err)
	}

	svc.importMu.Unlock()
}
```

- [ ] **Step 6: 執行測試**

```bash
cd /d/Vaultflix && docker compose exec api go test ./internal/service/ -run TestStartAsync_Conflict -v
```

Expected: 測試通過。

- [ ] **Step 7: 寫 TestStartAsync_EmptyDirectory — 驗證非同步 + 進度推送**

```go
func TestStartAsync_EmptyDirectory(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Empty Source",
		MountPath: t.TempDir(),
	}

	job, err := svc.StartAsync(nil, source, "user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	// 等待背景完成
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job to complete")
		default:
			got, _ := svc.GetJob(job.ID)
			if got.Status != "running" {
				// Job 完成
				if got.Status != "completed" {
					t.Errorf("expected completed, got %s", got.Status)
				}
				if got.Total != 0 {
					t.Errorf("expected total 0, got %d", got.Total)
				}
				if got.FinishedAt == nil {
					t.Error("expected FinishedAt to be set")
				}

				// 應該收到 import_complete 訊息
				msgs := notifier.GetMessages()
				if len(msgs) == 0 {
					t.Fatal("expected at least 1 notifier message")
				}
				lastMsg := msgs[len(msgs)-1]
				if lastMsg.Type != "import_complete" {
					t.Errorf("expected import_complete, got %s", lastMsg.Type)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
```

- [ ] **Step 8: 執行測試**

```bash
cd /d/Vaultflix && docker compose exec api go test ./internal/service/ -run TestStartAsync_EmptyDirectory -v
```

Expected: 測試通過。

- [ ] **Step 9: 寫 TestStartAsync_ScanError — 目錄不存在**

```go
func TestStartAsync_ScanError(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Bad Source",
		MountPath: "/nonexistent/path/that/does/not/exist",
	}

	job, err := svc.StartAsync(nil, source, "user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job to complete")
		default:
			got, _ := svc.GetJob(job.ID)
			if got.Status != "running" {
				if got.Status != "failed" {
					t.Errorf("expected failed, got %s", got.Status)
				}
				if len(got.Errors) == 0 {
					t.Error("expected at least 1 error")
				}

				msgs := notifier.GetMessages()
				hasImportError := false
				for _, msg := range msgs {
					if msg.Type == "import_error" {
						hasImportError = true
					}
				}
				if !hasImportError {
					t.Error("expected import_error message")
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
```

- [ ] **Step 10: 執行測試**

```bash
cd /d/Vaultflix && docker compose exec api go test ./internal/service/ -run TestStartAsync_ScanError -v
```

Expected: 測試通過。

- [ ] **Step 11: Commit**

```bash
git add internal/service/import_service_test.go
git commit -m "test: add import service tests for async job management"
```

---

## Task 5: 修改 Handler + 路由 + DI

**Files:**
- Modify: `internal/handler/video_handler.go`
- Modify: `internal/handler/ws_handler.go`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: 修改 video_handler.go — Import 方法改為非同步**

在 `internal/handler/video_handler.go` 中，修改 `Import` 方法：

```go
func (h *VideoHandler) Import(c *gin.Context) {
	var req importRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "source_id is required",
		})
		return
	}

	ctx := c.Request.Context()

	source, err := h.mediaSourceService.GetByID(ctx, req.SourceID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "media source not found",
			})
			return
		}
		slog.Error("failed to get media source", "error", err, "source_id", req.SourceID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to get media source",
		})
		return
	}

	if !source.Enabled {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "source_disabled",
			Message: "media source is currently disabled",
		})
		return
	}

	userID := c.GetString("user_id")

	job, err := h.importService.StartAsync(ctx, source, userID)
	if err != nil {
		if errors.Is(err, model.ErrConflict) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error:   "import_in_progress",
				Message: "another import job is already running",
			})
			return
		}
		slog.Error("failed to start import", "error", err, "source_id", req.SourceID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to start import",
		})
		return
	}

	c.JSON(http.StatusAccepted, model.SuccessResponse{Data: job})
}
```

- [ ] **Step 2: 新增 GetImportJob 和 GetActiveImportJob handler 方法**

在 `video_handler.go` 底部（`parseVideoFilter` 之前）新增：

```go
func (h *VideoHandler) GetImportJob(c *gin.Context) {
	jobID := c.Param("id")
	job, err := h.importService.GetJob(jobID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "import job not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to get import job",
		})
		return
	}
	c.JSON(http.StatusOK, model.SuccessResponse{Data: job})
}

func (h *VideoHandler) GetActiveImportJob(c *gin.Context) {
	job := h.importService.GetActiveJob()
	c.JSON(http.StatusOK, model.SuccessResponse{Data: job})
}
```

- [ ] **Step 3: 移除 ws_handler.go 中的 TestSend 方法和 wsTestRequest**

在 `internal/handler/ws_handler.go` 中，移除：
- `wsTestRequest` struct（第 61-64 行）
- `TestSend` 方法（第 69-89 行）

- [ ] **Step 4: 修改 main.go — 更新 DI 和路由**

在 `cmd/server/main.go` 中：

1. 修改 `importService` 的建構，注入 `hub` 作為 notifier：

```go
importService := service.NewImportService(videoRepo, minioService, hub)
```

2. 新增路由（在 `api.POST("/videos/import", videoHandler.Import)` 附近）：

```go
// Import job endpoints
api.GET("/import-jobs/active", videoHandler.GetActiveImportJob)
api.GET("/import-jobs/:id", videoHandler.GetImportJob)
```

注意：`/import-jobs/active` 必須在 `/import-jobs/:id` 之前。

3. 移除 ws-test 路由：

刪除這行：
```go
api.POST("/admin/ws-test", wsHandler.TestSend)
```

- [ ] **Step 5: 確認編譯通過**

```bash
cd /d/Vaultflix && docker compose exec api go build ./...
```

Expected: 編譯成功。

- [ ] **Step 6: 執行全部測試**

```bash
cd /d/Vaultflix && docker compose exec api go test ./... 2>&1 | tail -30
```

Expected: 所有測試通過。如果有因為 `NewImportService` 簽名變更而失敗的既有測試（例如 `video_handler_test.go` 中用到 `NewVideoHandler(nil, ...)`），需要修正。

- [ ] **Step 7: Commit**

```bash
git add internal/handler/video_handler.go internal/handler/ws_handler.go cmd/server/main.go
git commit -m "feat: async import handler with 202 response, add job query endpoints, remove ws-test"
```

---

## Task 6: Handler 測試

**Files:**
- Modify: `internal/handler/video_handler_test.go`

- [ ] **Step 1: 修正既有測試中因 Import Handler 依賴變更導致的編譯問題**

既有 `setupVideoRouter` 傳 `nil` 給 `importService`，如果新的 handler 沒有呼叫到 importService 的方法就不會 panic，但需要確認。如果既有測試仍能通過就不需要修改。

先執行確認：

```bash
cd /d/Vaultflix && docker compose exec api go test ./internal/handler/ -run TestListVideos -v
```

- [ ] **Step 2: 寫 TestImportHandler_Async_Conflict**

在 `video_handler_test.go` 底部新增：

```go
func TestImportHandler_Async_Conflict(t *testing.T) {
	notifier := &mock.Notifier{}
	importSvc := service.NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	// 手動鎖住 importMu 模擬正在執行的 job
	importSvc.LockForTest()

	mediaSourceSvc := service.NewMediaSourceService(&mock.MediaSourceRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return &model.MediaSource{ID: "src-1", Label: "Test", MountPath: "/tmp", Enabled: true}, nil
		},
	}, "/tmp")

	r := gin.New()
	h := NewVideoHandler(importSvc, nil, mediaSourceSvc)
	r.POST("/api/videos/import", func(c *gin.Context) {
		c.Set("user_id", "test-user")
		h.Import(c)
	})

	body := strings.NewReader(`{"source_id":"src-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/videos/import", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	importSvc.UnlockForTest()
}
```

注意：需要在 `import_service.go` 新增 `LockForTest` / `UnlockForTest` 測試輔助方法，或者直接在測試前用 `StartAsync` 觸發一個 job 然後再呼叫。

**更好的做法**：不加測試輔助方法，而是在 handler 測試中直接用 mock。但因為 handler 直接依賴 `*service.ImportService`（concrete type），目前無法 mock。

**最佳做法**：直接用真實 service 並鎖住 mutex。在 `import_service.go` 中新增：

```go
// LockForTest locks the import mutex for testing purposes.
// This should only be called in tests.
func (s *ImportService) LockForTest() {
	s.importMu.Lock()
}

// UnlockForTest unlocks the import mutex for testing purposes.
func (s *ImportService) UnlockForTest() {
	s.importMu.Unlock()
}
```

- [ ] **Step 3: 寫 TestGetActiveImportJobHandler_NoJob**

```go
func TestGetActiveImportJobHandler_NoJob(t *testing.T) {
	notifier := &mock.Notifier{}
	importSvc := service.NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	r := gin.New()
	h := NewVideoHandler(importSvc, nil, nil)
	r.GET("/api/import-jobs/active", h.GetActiveImportJob)

	req := httptest.NewRequest(http.MethodGet, "/api/import-jobs/active", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Data != nil {
		t.Errorf("expected null data, got %v", resp.Data)
	}
}
```

- [ ] **Step 4: 寫 TestGetImportJobHandler_NotFound**

```go
func TestGetImportJobHandler_NotFound(t *testing.T) {
	notifier := &mock.Notifier{}
	importSvc := service.NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	r := gin.New()
	h := NewVideoHandler(importSvc, nil, nil)
	r.GET("/api/import-jobs/:id", h.GetImportJob)

	req := httptest.NewRequest(http.MethodGet, "/api/import-jobs/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
```

- [ ] **Step 5: 執行所有 handler 測試**

```bash
cd /d/Vaultflix && docker compose exec api go test ./internal/handler/ -v
```

Expected: 所有測試通過。

- [ ] **Step 6: Commit**

```bash
git add internal/service/import_service.go internal/handler/video_handler_test.go
git commit -m "test: add handler tests for async import and job query endpoints"
```

---

## Task 7: 前端型別 + API 函式

**Files:**
- Modify: `web/src/types/index.ts`
- Modify: `web/src/api/admin.ts`

- [ ] **Step 1: 新增 TypeScript 型別**

在 `web/src/types/index.ts` 中，更新 `ImportResult` / `ImportFailure` 為新的型別，並新增 `ImportJob`、`ImportProgress`、`MediaSource`：

將原來的 `ImportFailure` 和 `ImportResult` 替換為：

```typescript
export interface ImportError {
  file_name: string
  error: string
}

export interface ImportJob {
  id: string
  source_id: string
  source_label: string
  status: 'running' | 'completed' | 'failed'
  total: number
  processed: number
  imported: number
  skipped: number
  failed: number
  errors: ImportError[]
  started_at: string
  finished_at?: string
}

export interface ImportProgress {
  job_id: string
  file_name: string
  current: number
  total: number
  status: 'processing' | 'success' | 'skipped' | 'error'
  error?: string
}

export interface MediaSource {
  id: string
  label: string
  mount_path: string
  enabled: boolean
  created_at: string
  updated_at: string
}
```

移除舊的 `ImportFailure` 和 `ImportResult`。

- [ ] **Step 2: 更新 admin.ts API 函式**

在 `web/src/api/admin.ts` 中：

1. 修改 import，移除 `ImportResult`，新增 `ImportJob`、`MediaSource`：

```typescript
import type {
  ImportJob,
  MediaSource,
  Video,
  Tag,
  DailyRecommendation,
  RecommendationItem,
  User,
} from '../types'
```

2. 替換 `importVideos`：

```typescript
export async function importVideos(sourceID: string): Promise<ImportJob> {
  const res = await client.post<ImportJob>('/videos/import', { source_id: sourceID })
  return res.data
}
```

3. 新增：

```typescript
export async function getActiveImportJob(): Promise<ImportJob | null> {
  const res = await client.get<ImportJob | null>('/import-jobs/active')
  return res.data
}

export async function getImportJob(id: string): Promise<ImportJob> {
  const res = await client.get<ImportJob>(`/import-jobs/${id}`)
  return res.data
}

export async function listMediaSources(): Promise<MediaSource[]> {
  const res = await client.get<MediaSource[]>('/media-sources')
  return res.data
}
```

- [ ] **Step 3: 確認前端編譯通過**

```bash
cd /d/Vaultflix && docker compose exec frontend npm run build 2>&1 | tail -20
```

Expected: 可能因為 `VideoManagePage.tsx` 仍引用舊的 `ImportResult` 而失敗。這是預期的，Task 8 會修正。如果只是型別引用問題，記下來繼續。

- [ ] **Step 4: Commit**

```bash
git add web/src/types/index.ts web/src/api/admin.ts
git commit -m "feat: update frontend types and API for async import jobs"
```

---

## Task 8: 前端匯入 UI 改造

**Files:**
- Modify: `web/src/pages/admin/VideoManagePage.tsx`

這是前端最大的改動。將匯入 modal 從「輸入目錄路徑 → 同步等待結果」改為「選擇 media source → 非同步 + WebSocket 進度」。

- [ ] **Step 1: 更新 imports 和型別引用**

在 `VideoManagePage.tsx` 頂部，更新 imports：

```typescript
import { useState, useEffect, useCallback, useRef } from 'react'
import { useSearchParams, Link } from 'react-router-dom'
import { listVideos } from '../../api/videos'
import { listTags } from '../../api/tags'
import { importVideos, updateVideo, deleteVideo, listMediaSources, getActiveImportJob } from '../../api/admin'
import type { VideoWithTags, TagWithCount, ImportJob, ImportProgress, ImportError as ImportErr, MediaSource } from '../../types'
import { useWS } from '../../contexts/WebSocketContext'
import Header from '../../components/Header'
import Pagination from '../../components/Pagination'
import TagInput from '../../components/TagInput'
import { formatDuration, formatFileSize } from '../../utils/format'
```

- [ ] **Step 2: 替換匯入相關 state**

移除舊的 `importDir`、`importing`、`importResult` state，替換為：

```typescript
// Import state
type ImportState = 'idle' | 'importing' | 'completed' | 'failed'
const [showImport, setShowImport] = useState(false)
const [importState, setImportState] = useState<ImportState>('idle')
const [mediaSources, setMediaSources] = useState<MediaSource[]>([])
const [selectedSourceID, setSelectedSourceID] = useState('')
const [currentJobId, setCurrentJobId] = useState<string | null>(null)
const [currentFile, setCurrentFile] = useState('')
const [processed, setProcessed] = useState(0)
const [total, setTotal] = useState(0)
const [imported, setImported] = useState(0)
const [skipped, setSkipped] = useState(0)
const [failed, setFailed] = useState(0)
const [importErrors, setImportErrors] = useState<ImportErr[]>([])
const [finalResult, setFinalResult] = useState<ImportJob | null>(null)
const [showErrors, setShowErrors] = useState(false)

const { lastMessage } = useWS()
```

- [ ] **Step 3: 新增匯入相關 effects 和 handlers**

```typescript
// Fetch media sources when import modal opens
useEffect(() => {
  if (!showImport) return
  listMediaSources()
    .then((sources) => {
      const enabled = sources.filter((s) => s.enabled)
      setMediaSources(enabled)
      if (enabled.length > 0 && !selectedSourceID) {
        setSelectedSourceID(enabled[0].id)
      }
    })
    .catch(() => setMediaSources([]))
}, [showImport])

// Check for active import job on mount
useEffect(() => {
  let cancelled = false
  getActiveImportJob().then((job) => {
    if (cancelled || !job) return
    setShowImport(true)
    setCurrentJobId(job.id)
    setImportState('importing')
    setProcessed(job.processed)
    setTotal(job.total)
    setImported(job.imported)
    setSkipped(job.skipped)
    setFailed(job.failed)
    setImportErrors(job.errors)
  }).catch(() => {})
  return () => { cancelled = true }
}, [])

// WebSocket progress listener
useEffect(() => {
  if (!lastMessage) return

  switch (lastMessage.type) {
    case 'import_progress': {
      const p = lastMessage.payload as ImportProgress
      if (p.job_id !== currentJobId) break
      if (p.status === 'processing') {
        setCurrentFile(p.file_name)
      } else {
        setProcessed(p.current)
        setTotal(p.total)
        if (p.status === 'success') setImported((prev) => prev + 1)
        if (p.status === 'skipped') setSkipped((prev) => prev + 1)
        if (p.status === 'error') {
          setFailed((prev) => prev + 1)
          setImportErrors((prev) => [...prev, { file_name: p.file_name, error: p.error || '' }])
        }
      }
      break
    }
    case 'import_complete': {
      const result = lastMessage.payload as ImportJob
      if (result.id !== currentJobId) break
      setFinalResult(result)
      setImportState(result.failed > 0 && result.imported === 0 ? 'failed' : 'completed')
      // Refresh video list
      updateParams({ page: '1' })
      break
    }
    case 'import_error': {
      setImportState('failed')
      break
    }
  }
}, [lastMessage, currentJobId])

// Start import handler
function resetImportState() {
  setImportState('idle')
  setCurrentJobId(null)
  setCurrentFile('')
  setProcessed(0)
  setTotal(0)
  setImported(0)
  setSkipped(0)
  setFailed(0)
  setImportErrors([])
  setFinalResult(null)
  setShowErrors(false)
}

async function handleStartImport() {
  if (!selectedSourceID) return
  try {
    const job = await importVideos(selectedSourceID)
    setCurrentJobId(job.id)
    setImportState('importing')
    setTotal(job.total)
  } catch (err: unknown) {
    const axiosErr = err as { response?: { status?: number } }
    if (axiosErr?.response?.status === 409) {
      // 已有匯入進行中，嘗試恢復
      getActiveImportJob().then((activeJob) => {
        if (activeJob) {
          setCurrentJobId(activeJob.id)
          setImportState('importing')
          setProcessed(activeJob.processed)
          setTotal(activeJob.total)
        }
      }).catch(() => {})
    }
  }
}
```

- [ ] **Step 4: 替換匯入 modal 的 JSX**

將整個 `{/* Import Modal */}` 區塊替換為：

```tsx
{/* Import Modal */}
{showImport && (
  <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => importState !== 'importing' && setShowImport(false)}>
    <div className="bg-gray-900 rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
      <h2 className="text-lg font-semibold text-white mb-4">匯入影片</h2>

      {importState === 'idle' && (
        <>
          <label className="block text-sm text-gray-400 mb-1">選擇媒體來源</label>
          {mediaSources.length === 0 ? (
            <p className="text-sm text-gray-500 mb-4">沒有可用的媒體來源</p>
          ) : (
            <select
              value={selectedSourceID}
              onChange={(e) => setSelectedSourceID(e.target.value)}
              className="w-full bg-gray-800 text-white text-sm rounded px-3 py-2 outline-none focus:ring-2 focus:ring-indigo-500 mb-4"
            >
              {mediaSources.map((s) => (
                <option key={s.id} value={s.id}>{s.label} ({s.mount_path})</option>
              ))}
            </select>
          )}
          <div className="flex justify-end gap-2">
            <button onClick={() => setShowImport(false)} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">取消</button>
            <button
              onClick={handleStartImport}
              disabled={!selectedSourceID || mediaSources.length === 0}
              className="bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white text-sm px-4 py-1.5 rounded"
            >
              開始匯入
            </button>
          </div>
        </>
      )}

      {importState === 'importing' && (
        <>
          <div className="mb-4">
            <div className="flex justify-between text-sm text-gray-400 mb-1">
              <span>進度</span>
              <span>{processed} / {total || '...'}</span>
            </div>
            <div className="w-full bg-gray-800 rounded-full h-2">
              <div
                className="bg-indigo-500 h-2 rounded-full transition-all duration-300"
                style={{ width: total > 0 ? `${(processed / total) * 100}%` : '0%' }}
              />
            </div>
          </div>
          {currentFile && (
            <p className="text-xs text-gray-500 mb-3 truncate">處理中: {currentFile}</p>
          )}
          <div className="grid grid-cols-3 gap-2 text-sm mb-4">
            <div className="text-center">
              <div className="text-green-400 font-medium">{imported}</div>
              <div className="text-gray-500 text-xs">成功</div>
            </div>
            <div className="text-center">
              <div className="text-gray-400 font-medium">{skipped}</div>
              <div className="text-gray-500 text-xs">跳過</div>
            </div>
            <div className="text-center">
              <div className="text-red-400 font-medium">{failed}</div>
              <div className="text-gray-500 text-xs">失敗</div>
            </div>
          </div>
          <p className="text-xs text-gray-600 text-center">匯入進行中，請勿關閉此視窗...</p>
        </>
      )}

      {(importState === 'completed' || importState === 'failed') && (
        <>
          <div className={`text-sm mb-3 ${importState === 'failed' ? 'text-red-400' : 'text-green-400'}`}>
            {importState === 'completed' ? '匯入完成' : '匯入失敗'}
          </div>
          <div className="space-y-2 text-sm mb-4">
            <div className="flex justify-between text-gray-300"><span>掃描檔案</span><span>{finalResult?.total ?? total}</span></div>
            <div className="flex justify-between text-green-400"><span>成功匯入</span><span>{finalResult?.imported ?? imported}</span></div>
            <div className="flex justify-between text-gray-400"><span>已跳過（重複）</span><span>{finalResult?.skipped ?? skipped}</span></div>
            <div className="flex justify-between text-red-400"><span>失敗</span><span>{finalResult?.failed ?? failed}</span></div>
          </div>
          {(finalResult?.errors?.length ?? importErrors.length) > 0 && (
            <div className="mb-4">
              <button
                onClick={() => setShowErrors(!showErrors)}
                className="text-xs text-red-400 hover:text-red-300 mb-1"
              >
                {showErrors ? '收起' : '展開'}失敗詳情 ({finalResult?.errors?.length ?? importErrors.length})
              </button>
              {showErrors && (
                <div className="bg-gray-800 rounded p-2 max-h-40 overflow-y-auto space-y-1">
                  {(finalResult?.errors ?? importErrors).map((e, i) => (
                    <div key={i} className="text-xs">
                      <span className="text-gray-300">{e.file_name}</span>
                      <span className="text-gray-600 ml-1">— {e.error}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
          <div className="flex justify-end gap-2">
            <button onClick={() => { resetImportState(); setShowImport(false) }} className="text-sm text-gray-400 hover:text-white px-3 py-1.5 rounded">關閉</button>
            <button
              onClick={resetImportState}
              className="bg-indigo-600 hover:bg-indigo-500 text-white text-sm px-4 py-1.5 rounded"
            >
              重新匯入
            </button>
          </div>
        </>
      )}
    </div>
  </div>
)}
```

- [ ] **Step 5: 更新「匯入影片」按鈕的 onClick**

將頂部按鈕的 onClick 改為：

```tsx
onClick={() => { setShowImport(true); resetImportState() }}
```

- [ ] **Step 6: 移除舊的 import handler 函式和未使用的 state**

移除舊的 `handleImport` 函式、`importDir` state、`importResult` state、`importing` state。

- [ ] **Step 7: 確認前端編譯通過**

```bash
cd /d/Vaultflix && docker compose exec frontend npm run build 2>&1 | tail -20
```

Expected: 編譯成功。

- [ ] **Step 8: Commit**

```bash
git add web/src/pages/admin/VideoManagePage.tsx
git commit -m "feat: replace sync import modal with async progress UI using WebSocket"
```

---

## Task 9: 全面驗證

**Files:** 無新增或修改

- [ ] **Step 1: 執行所有後端測試**

```bash
cd /d/Vaultflix && docker compose exec api go test ./... 2>&1 | tail -30
```

Expected: 所有測試通過。

- [ ] **Step 2: 確認前端編譯**

```bash
cd /d/Vaultflix && docker compose exec frontend npm run build 2>&1 | tail -20
```

Expected: 編譯成功。

- [ ] **Step 3: 確認 ws_handler_test.go 沒有引用已移除的 TestSend**

```bash
cd /d/Vaultflix && docker compose exec api go test ./internal/handler/ -run TestWS -v
```

如果有引用已移除方法的測試，需要移除或修正。

- [ ] **Step 4: Commit（如果有修正）**

只在 Step 3 需要修正時才 commit。

---

## 驗收清單

- [ ] `POST /api/videos/import` 回傳 **202 Accepted** + job ID
- [ ] 背景 goroutine 持續執行匯入，不因 HTTP response 結束而中斷
- [ ] WebSocket 收到逐檔 `import_progress` 訊息
- [ ] 匯入完成時收到 `import_complete` 訊息
- [ ] 匯入目錄不可讀時收到 `import_error` 訊息
- [ ] 同時提交兩個匯入回傳 **409 Conflict**
- [ ] `GET /api/import-jobs/:id` 回傳正確 job 狀態
- [ ] `GET /api/import-jobs/active` 有/無 running job 時回傳正確
- [ ] 前端顯示即時進度條 + 檔名 + 計數
- [ ] 前端匯入完成後顯示摘要
- [ ] 前端重整頁面後能恢復進度
- [ ] 匯入進行中按鈕停用
- [ ] `go test ./...` 全部通過
- [ ] `ws-test` 端點已移除
