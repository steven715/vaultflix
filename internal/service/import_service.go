package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
	"github.com/steven/vaultflix/internal/websocket"
)

var supportedExtensions = map[string]bool{
	".mp4": true,
	".mkv": true,
	".avi": true,
	".wmv": true,
	".mov": true,
}

type fileResult struct {
	Status string // "success", "skipped", "error"
	Error  string
}

type ImportService struct {
	videoRepo  repository.VideoRepository
	media      mediaDeriver
	notifier   websocket.Notifier
	activeJobs sync.Map
	importMu   sync.Mutex
	// jobMu serialises every read/write of a stored ImportJob's fields against
	// the background runImport goroutine, so GetJob/GetActiveJob never race it.
	jobMu     sync.Mutex
	keyframes keyframeProber // optional;nil 時不觸發
}

func NewImportService(videoRepo repository.VideoRepository, media mediaDeriver, notifier websocket.Notifier) *ImportService {
	return &ImportService{
		videoRepo: videoRepo,
		media:     media,
		notifier:  notifier,
	}
}

// keyframeProber 觸發影片的非同步 keyframe 探測(由 *KeyframeService 實作)。
type keyframeProber interface {
	TriggerProbe(videoID, absPath string)
}

// SetKeyframeProber 注入 keyframe prober(比照 VideoService.SetUserServices 的後注入模式)。
func (s *ImportService) SetKeyframeProber(p keyframeProber) {
	s.keyframes = p
}

// StartAsync builds a job and launches a background import, returning job info immediately.
// Only one import job may run at a time; duplicate calls return model.ErrConflict.
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
	snapshot := s.snapshotJob(job)

	go func() {
		defer s.importMu.Unlock()
		s.runImport(context.Background(), job, source, userID)
	}()

	// Return a snapshot, not the live pointer: runImport mutates job under jobMu.
	return snapshot, nil
}

// updateJob applies fn to job while holding jobMu.
func (s *ImportService) updateJob(job *model.ImportJob, fn func(*model.ImportJob)) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	fn(job)
}

// snapshotJob returns a deep copy of job taken under jobMu.
func (s *ImportService) snapshotJob(job *model.ImportJob) *model.ImportJob {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	return cloneImportJobLocked(job)
}

// cloneImportJobLocked deep-copies j (Errors slice + FinishedAt pointer).
// Caller must hold jobMu. Returns nil for a nil job.
func cloneImportJobLocked(j *model.ImportJob) *model.ImportJob {
	if j == nil {
		return nil
	}
	cp := *j
	cp.Errors = append([]model.ImportError(nil), j.Errors...)
	if j.FinishedAt != nil {
		f := *j.FinishedAt
		cp.FinishedAt = &f
	}
	return &cp
}

func (s *ImportService) runImport(ctx context.Context, job *model.ImportJob, source *model.MediaSource, userID string) {
	jobID := job.ID

	defer func() {
		now := time.Now()
		s.updateJob(job, func(j *model.ImportJob) {
			j.FinishedAt = &now
			if j.Status != "failed" {
				if j.Failed > 0 && j.Imported == 0 {
					j.Status = "failed"
				} else {
					j.Status = "completed"
				}
			}
		})
		snap := s.snapshotJob(job)
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeImportComplete,
			Payload: snap,
		})
		slog.Info("import completed",
			"source_id", source.ID,
			"source_label", source.Label,
			"total_scanned", snap.Total,
			"imported", snap.Imported,
			"skipped", snap.Skipped,
			"failed", snap.Failed,
		)
	}()

	files, err := s.scanVideoFiles(source.MountPath)
	if err != nil {
		s.updateJob(job, func(j *model.ImportJob) {
			j.Status = "failed"
			j.Errors = append(j.Errors, model.ImportError{
				FileName: source.MountPath,
				Error:    err.Error(),
			})
		})
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeImportError,
			Payload: map[string]string{"job_id": jobID, "error": err.Error()},
		})
		return
	}

	total := len(files)
	s.updateJob(job, func(j *model.ImportJob) { j.Total = total })

	for i, filePath := range files {
		fileName := filepath.Base(filePath)

		s.notifier.SendToUser(userID, &websocket.Message{
			Type: websocket.TypeImportProgress,
			Payload: model.ImportProgress{
				JobID:    jobID,
				FileName: fileName,
				Current:  i + 1,
				Total:    total,
				Status:   "processing",
			},
		})

		result := s.processOneFile(ctx, source, filePath)

		s.updateJob(job, func(j *model.ImportJob) {
			j.Processed = i + 1
			switch result.Status {
			case "success":
				j.Imported++
			case "skipped":
				j.Skipped++
			case "error":
				j.Failed++
				j.Errors = append(j.Errors, model.ImportError{
					FileName: fileName,
					Error:    result.Error,
				})
			}
		})

		s.notifier.SendToUser(userID, &websocket.Message{
			Type: websocket.TypeImportProgress,
			Payload: model.ImportProgress{
				JobID:    jobID,
				FileName: fileName,
				Current:  i + 1,
				Total:    total,
				Status:   result.Status,
				Error:    result.Error,
			},
		})
	}
}

// GetJob returns a snapshot of the job with the given ID. Returns
// model.ErrNotFound if not found.
func (s *ImportService) GetJob(jobID string) (*model.ImportJob, error) {
	val, ok := s.activeJobs.Load(jobID)
	if !ok {
		return nil, model.ErrNotFound
	}
	return s.snapshotJob(val.(*model.ImportJob)), nil
}

// GetActiveJob returns a snapshot of the currently running job, if any.
// Returns nil when idle.
func (s *ImportService) GetActiveJob() *model.ImportJob {
	var active *model.ImportJob
	s.activeJobs.Range(func(key, value interface{}) bool {
		job := value.(*model.ImportJob)
		s.jobMu.Lock()
		var snap *model.ImportJob
		if job.Status == "running" {
			snap = cloneImportJobLocked(job)
		}
		s.jobMu.Unlock()
		if snap != nil {
			active = snap
			return false
		}
		return true
	})
	return active
}

// LockForTest locks the import mutex for testing purposes.
func (s *ImportService) LockForTest() {
	s.importMu.Lock()
}

// UnlockForTest unlocks the import mutex for testing purposes.
func (s *ImportService) UnlockForTest() {
	s.importMu.Unlock()
}

func (s *ImportService) scanVideoFiles(sourceDir string) ([]string, error) {
	var files []string

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to access path %s: %w", path, err)
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if supportedExtensions[ext] {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", sourceDir, err)
	}

	return files, nil
}
