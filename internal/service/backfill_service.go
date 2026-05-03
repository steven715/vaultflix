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
	s.mu.Unlock()

	go s.run(job, cancelCh, userID)

	return job, nil
}

// GetActiveJob returns the most recently started job (running or finished),
// or nil when no backfill has ever started in this process.
func (s *BackfillService) GetActiveJob() *model.BackfillJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeJob
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
	defer func() {
		finished := time.Now()
		job.FinishedAt = &finished
		job.CurrentVideoID = ""
		if job.Status == "running" {
			job.Status = "completed"
		}
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeBackfillComplete,
			Payload: job,
		})
		slog.Info("backfill job finished",
			"job_id", job.ID,
			"status", job.Status,
			"total", job.Total,
			"succeeded", job.Succeeded,
			"failed", job.Failed,
		)
	}()

	videos, err := s.videoRepo.ListMissingPreviews(context.Background())
	if err != nil {
		job.Status = "failed"
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeBackfillError,
			Payload: map[string]string{"job_id": job.ID, "error": err.Error()},
		})
		return
	}
	job.Total = len(videos)

	for i, v := range videos {
		select {
		case <-cancelCh:
			job.Status = "cancelled"
			return
		default:
		}

		job.CurrentVideoID = v.ID
		s.notifier.SendToUser(userID, &websocket.Message{
			Type: websocket.TypeBackfillProgress,
			Payload: model.BackfillProgress{
				JobID:            job.ID,
				VideoID:          v.ID,
				OriginalFilename: v.OriginalFilename,
				Current:          i + 1,
				Total:            job.Total,
				Status:           "processing",
			},
		})

		processErr := s.processOneVideo(&v)
		job.Processed = i + 1

		if processErr != nil {
			job.Failed++
			job.Errors = append(job.Errors, model.BackfillError{
				VideoID:          v.ID,
				OriginalFilename: v.OriginalFilename,
				Error:            processErr.Error(),
			})
			slog.Warn("backfill preview failed",
				"job_id", job.ID,
				"video_id", v.ID,
				"file", v.OriginalFilename,
				"error", processErr,
			)
			s.notifier.SendToUser(userID, &websocket.Message{
				Type: websocket.TypeBackfillProgress,
				Payload: model.BackfillProgress{
					JobID:            job.ID,
					VideoID:          v.ID,
					OriginalFilename: v.OriginalFilename,
					Current:          i + 1,
					Total:            job.Total,
					Status:           "error",
					Error:            processErr.Error(),
				},
			})
		} else {
			job.Succeeded++
			s.notifier.SendToUser(userID, &websocket.Message{
				Type: websocket.TypeBackfillProgress,
				Payload: model.BackfillProgress{
					JobID:            job.ID,
					VideoID:          v.ID,
					OriginalFilename: v.OriginalFilename,
					Current:          i + 1,
					Total:            job.Total,
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
