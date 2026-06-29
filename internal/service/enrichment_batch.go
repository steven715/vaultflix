package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/websocket"
)

// StartBatchAsync launches a batch enrichment worker in the background and
// returns a snapshot of the new job. Returns model.ErrConflict when another
// batch is already running. status filters which videos to process; an empty
// string defaults to model.EnrichmentPending. Notifications are pushed to the
// WebSocket connections of userID.
func (s *EnrichmentService) StartBatchAsync(ctx context.Context, status, userID string) (*model.EnrichJob, error) {
	if status == "" {
		status = model.EnrichmentPending
	}

	s.mu.Lock()
	if s.activeJob != nil && s.activeJob.Status == "running" {
		s.mu.Unlock()
		return nil, model.ErrConflict
	}

	job := &model.EnrichJob{
		ID:        uuid.New().String(),
		Status:    "running",
		StartedAt: time.Now(),
	}
	cancelCh := make(chan struct{})

	s.activeJob = job
	s.cancelCh = cancelCh
	snapshot := cloneEnrichJobLocked(job)
	s.mu.Unlock()

	go s.runBatch(job, cancelCh, status, userID)

	// Return a snapshot: the worker goroutine mutates job under s.mu.
	return snapshot, nil
}

// ActiveJob returns a snapshot of the most recently started batch enrichment
// job (running or finished), or nil when no batch has ever started in this
// process. Safe to read concurrently with the running worker.
func (s *EnrichmentService) ActiveJob() *model.EnrichJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneEnrichJobLocked(s.activeJob)
}

// CancelBatch requests the running job with the given ID to stop after the
// current video finishes. Returns model.ErrNotFound when no such job exists.
// Cancelling a finished job is a no-op (idempotent).
func (s *EnrichmentService) CancelBatch(jobID string) error {
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

// updateEnrichJob applies fn to job while holding s.mu, serialising every
// mutation of the job's fields against snapshots taken by ActiveJob / the
// worker.
func (s *EnrichmentService) updateEnrichJob(job *model.EnrichJob, fn func(*model.EnrichJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(job)
}

// snapshotEnrichJob returns a deep copy of job taken under s.mu.
func (s *EnrichmentService) snapshotEnrichJob(job *model.EnrichJob) *model.EnrichJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneEnrichJobLocked(job)
}

// cloneEnrichJobLocked returns a shallow copy of j (FinishedAt pointer is
// deep-copied). Caller must hold s.mu. Returns nil for a nil job.
func cloneEnrichJobLocked(j *model.EnrichJob) *model.EnrichJob {
	if j == nil {
		return nil
	}
	cp := *j
	if j.FinishedAt != nil {
		f := *j.FinishedAt
		cp.FinishedAt = &f
	}
	return &cp
}

// runBatch is the goroutine that drives the batch enrichment loop.
func (s *EnrichmentService) runBatch(job *model.EnrichJob, cancelCh chan struct{}, status, userID string) {
	jobID := job.ID

	defer func() {
		finished := time.Now()
		s.updateEnrichJob(job, func(j *model.EnrichJob) {
			j.FinishedAt = &finished
			j.CurrentVideoID = ""
			if j.Status == "running" {
				j.Status = "completed"
			}
		})
		snap := s.snapshotEnrichJob(job)
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeEnrichComplete,
			Payload: snap,
		})
		slog.Info("enrich batch finished",
			"job_id", snap.ID,
			"status", snap.Status,
			"total", snap.Total,
			"succeeded", snap.Succeeded,
			"failed", snap.Failed,
		)
	}()

	videos, err := s.videoRepo.ListByEnrichmentStatus(context.Background(), status)
	if err != nil {
		s.updateEnrichJob(job, func(j *model.EnrichJob) { j.Status = "failed" })
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeEnrichError,
			Payload: map[string]string{"job_id": jobID, "error": err.Error()},
		})
		return
	}

	total := len(videos)
	s.updateEnrichJob(job, func(j *model.EnrichJob) { j.Total = total })

	for i, v := range videos {
		select {
		case <-cancelCh:
			s.updateEnrichJob(job, func(j *model.EnrichJob) { j.Status = "cancelled" })
			return
		default:
		}

		s.updateEnrichJob(job, func(j *model.EnrichJob) { j.CurrentVideoID = v.ID })
		s.notifier.SendToUser(userID, &websocket.Message{
			Type: websocket.TypeEnrichProgress,
			Payload: map[string]interface{}{
				"job_id":   jobID,
				"video_id": v.ID,
				"current":  i + 1,
				"total":    total,
				"status":   "processing",
			},
		})

		enrichErr := s.EnrichVideo(context.Background(), v.ID, userID)

		s.updateEnrichJob(job, func(j *model.EnrichJob) {
			j.Processed = i + 1
			if enrichErr != nil {
				j.Failed++
			} else {
				j.Succeeded++
			}
		})

		if enrichErr != nil {
			slog.Warn("enrich batch: video failed",
				"job_id", jobID,
				"video_id", v.ID,
				"error", enrichErr,
			)
		}
	}
}
