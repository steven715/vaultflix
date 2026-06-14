package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
	"github.com/steven/vaultflix/internal/websocket"
)

// BackfillService runs preview-clip generation against existing videos that
// were imported before preview support shipped, or whose previous backfill
// failed. Only one backfill job runs at a time per process; concurrent
// StartAsync calls return model.ErrConflict.
type BackfillService struct {
	videoRepo  repository.VideoRepository
	sourceRepo repository.MediaSourceRepository
	minioSvc   MinIOClient
	notifier   websocket.Notifier

	// generatePreview is the preview-clip producer; overridable so tests can
	// avoid shelling out to ffmpeg. Defaults to the package-level
	// generatePreviewClip in NewBackfillService.
	generatePreview func(ctx context.Context, srcPath string, durationSeconds int) (string, error)

	mu        sync.Mutex
	activeJob *model.BackfillJob
	cancelCh  chan struct{}
}

func NewBackfillService(
	videoRepo repository.VideoRepository,
	sourceRepo repository.MediaSourceRepository,
	minioSvc MinIOClient,
	notifier websocket.Notifier,
) *BackfillService {
	return &BackfillService{
		videoRepo:       videoRepo,
		sourceRepo:      sourceRepo,
		minioSvc:        minioSvc,
		notifier:        notifier,
		generatePreview: generatePreviewClip,
	}
}

// StartAsync launches a backfill worker in the background and returns the new
// job. Returns model.ErrConflict when another backfill is already running.
// Notifications are pushed to the WebSocket connections of userID.
func (s *BackfillService) StartAsync(userID string) (*model.BackfillJob, error) {
	s.mu.Lock()
	if s.activeJob != nil && s.activeJob.Status == "running" {
		s.mu.Unlock()
		return nil, model.ErrConflict
	}

	job := &model.BackfillJob{
		ID:        uuid.New().String(),
		Status:    "running",
		Errors:    []model.BackfillError{},
		StartedAt: time.Now(),
	}
	cancelCh := make(chan struct{})

	s.activeJob = job
	s.cancelCh = cancelCh
	snapshot := cloneJobLocked(job)
	s.mu.Unlock()

	go s.run(job, cancelCh, userID)

	// Return a snapshot, not the live pointer: the worker goroutine mutates job
	// under s.mu, so the caller must never read its fields directly.
	return snapshot, nil
}

// GetActiveJob returns a snapshot of the most recently started job (running or
// finished), or nil when no backfill has ever started in this process. The
// snapshot is safe to read/serialize concurrently with the running worker.
func (s *BackfillService) GetActiveJob() *model.BackfillJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneJobLocked(s.activeJob)
}

// updateJob applies fn to job while holding s.mu, serialising every mutation of
// the job's fields against snapshots taken by GetActiveJob / the worker.
func (s *BackfillService) updateJob(job *model.BackfillJob, fn func(*model.BackfillJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(job)
}

// snapshotOf returns a deep copy of job taken under s.mu.
func (s *BackfillService) snapshotOf(job *model.BackfillJob) *model.BackfillJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneJobLocked(job)
}

// cloneJobLocked returns a deep copy of j (including its Errors slice and
// FinishedAt pointer). Caller must hold s.mu. Returns nil for a nil job.
func cloneJobLocked(j *model.BackfillJob) *model.BackfillJob {
	if j == nil {
		return nil
	}
	cp := *j
	cp.Errors = append([]model.BackfillError(nil), j.Errors...)
	if j.FinishedAt != nil {
		f := *j.FinishedAt
		cp.FinishedAt = &f
	}
	return &cp
}

// Cancel requests the running job with the given ID to stop after the
// current video finishes. Returns model.ErrNotFound when no such job exists.
// Cancelling a finished job is a no-op (idempotent).
func (s *BackfillService) Cancel(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeJob == nil || s.activeJob.ID != jobID {
		return model.ErrNotFound
	}
	if s.activeJob.Status != "running" {
		return nil
	}
	select {
	case <-s.cancelCh:
		// already closed
	default:
		close(s.cancelCh)
	}
	return nil
}

func (s *BackfillService) run(job *model.BackfillJob, cancelCh chan struct{}, userID string) {
	// jobID and Total are read in notification payloads below; jobID is immutable
	// and total is captured once so we never read mutable job fields without the lock.
	jobID := job.ID

	defer func() {
		finished := time.Now()
		s.updateJob(job, func(j *model.BackfillJob) {
			j.FinishedAt = &finished
			j.CurrentVideoID = ""
			if j.Status == "running" {
				j.Status = "completed"
			}
		})
		snap := s.snapshotOf(job)
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeBackfillComplete,
			Payload: snap,
		})
		slog.Info("backfill job finished",
			"job_id", snap.ID,
			"status", snap.Status,
			"total", snap.Total,
			"succeeded", snap.Succeeded,
			"failed", snap.Failed,
		)
	}()

	videos, err := s.videoRepo.ListMissingPreviews(context.Background())
	if err != nil {
		s.updateJob(job, func(j *model.BackfillJob) { j.Status = "failed" })
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeBackfillError,
			Payload: map[string]string{"job_id": jobID, "error": err.Error()},
		})
		return
	}
	total := len(videos)
	s.updateJob(job, func(j *model.BackfillJob) { j.Total = total })

	for i, v := range videos {
		select {
		case <-cancelCh:
			s.updateJob(job, func(j *model.BackfillJob) { j.Status = "cancelled" })
			return
		default:
		}

		s.updateJob(job, func(j *model.BackfillJob) { j.CurrentVideoID = v.ID })
		s.notifier.SendToUser(userID, &websocket.Message{
			Type: websocket.TypeBackfillProgress,
			Payload: model.BackfillProgress{
				JobID:            jobID,
				VideoID:          v.ID,
				OriginalFilename: v.OriginalFilename,
				Current:          i + 1,
				Total:            total,
				Status:           "processing",
			},
		})

		processErr := s.processOneVideo(&v)

		s.updateJob(job, func(j *model.BackfillJob) {
			j.Processed = i + 1
			if processErr != nil {
				j.Failed++
				j.Errors = append(j.Errors, model.BackfillError{
					VideoID:          v.ID,
					OriginalFilename: v.OriginalFilename,
					Error:            processErr.Error(),
				})
			} else {
				j.Succeeded++
			}
		})

		if processErr != nil {
			slog.Warn("backfill preview failed",
				"job_id", jobID,
				"video_id", v.ID,
				"file", v.OriginalFilename,
				"error", processErr,
			)
			s.notifier.SendToUser(userID, &websocket.Message{
				Type: websocket.TypeBackfillProgress,
				Payload: model.BackfillProgress{
					JobID:            jobID,
					VideoID:          v.ID,
					OriginalFilename: v.OriginalFilename,
					Current:          i + 1,
					Total:            total,
					Status:           "error",
					Error:            processErr.Error(),
				},
			})
		} else {
			s.notifier.SendToUser(userID, &websocket.Message{
				Type: websocket.TypeBackfillProgress,
				Payload: model.BackfillProgress{
					JobID:            jobID,
					VideoID:          v.ID,
					OriginalFilename: v.OriginalFilename,
					Current:          i + 1,
					Total:            total,
					Status:           "success",
				},
			})
		}
	}
}

// processOneVideo generates and uploads a preview for one video. Uses
// context.Background() for ffmpeg and MinIO so a cancellation request takes
// effect only between videos — never killing an in-flight ffmpeg process.
func (s *BackfillService) processOneVideo(v *model.Video) error {
	if v.SourceID == nil || v.FilePath == nil {
		return errors.New("video has no source/file_path; legacy MinIO-stored videos cannot be backfilled")
	}

	source, err := s.sourceRepo.FindByID(context.Background(), *v.SourceID)
	if err != nil {
		return fmt.Errorf("look up media source %s: %w", *v.SourceID, err)
	}

	absPath := filepath.Join(source.MountPath, *v.FilePath)

	previewPath, err := s.generatePreview(context.Background(), absPath, v.DurationSeconds)
	if err != nil {
		return fmt.Errorf("generate preview clip: %w", err)
	}
	defer os.Remove(previewPath)

	objectKey := fmt.Sprintf("previews/%s.mp4", v.ID)
	if err := s.minioSvc.UploadPreview(context.Background(), objectKey, previewPath); err != nil {
		return fmt.Errorf("upload preview: %w", err)
	}

	if err := s.videoRepo.UpdatePreviewKey(context.Background(), v.ID, objectKey); err != nil {
		return fmt.Errorf("update preview key: %w", err)
	}
	return nil
}
