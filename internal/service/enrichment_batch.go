package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/websocket"
)

// JobKindEnrichment tags Enrichment Jobs on the JobRunner.
const JobKindEnrichment = "enrichment"

// StartBatchAsync launches an Enrichment Job in the background and returns a
// snapshot of it. Returns model.ErrConflict when another Enrichment Job is
// already running. req.Status filters which videos to process; an empty
// string defaults to model.EnrichmentPending. Notifications are pushed to the
// WebSocket connections of userID.
func (s *EnrichmentService) StartBatchAsync(ctx context.Context, req model.EnrichBatchRequest, userID string) (*model.EnrichJob, error) {
	if req.Status == "" {
		req.Status = model.EnrichmentPending
	}
	job, err := s.jobs.Start(JobSpec{
		Kind: JobKindEnrichment, Task: &enrichmentItems{svc: s, req: req, userID: userID},
		Reporter: &enrichmentReport{notifier: s.notifier, userID: userID},
	})
	if err != nil {
		return nil, fmt.Errorf("start enrichment job: %w", err)
	}
	return toEnrichJob(job), nil
}

// ActiveJob returns a snapshot of the most recently started Enrichment Job
// (running or finished), or nil when none has started in this process.
func (s *EnrichmentService) ActiveJob() *model.EnrichJob {
	job := s.jobs.Latest()
	if job == nil || job.Kind != JobKindEnrichment {
		return nil
	}
	return toEnrichJob(job)
}

// CancelBatch requests the running job with the given ID to stop after the
// current video finishes. Returns model.ErrNotFound when no such job exists.
// Cancelling a finished job is a no-op (idempotent).
func (s *EnrichmentService) CancelBatch(jobID string) error {
	if err := s.jobs.Cancel(jobID); err != nil {
		return fmt.Errorf("cancel enrichment job %s: %w", jobID, err)
	}
	return nil
}

// enrichmentItems lists the Videos one Enrichment Job processes.
type enrichmentItems struct {
	svc    *EnrichmentService
	req    model.EnrichBatchRequest
	userID string
}

func (e *enrichmentItems) Items(ctx context.Context) ([]JobItem, error) {
	videos, err := e.svc.videoRepo.ListByEnrichmentStatus(ctx, e.req.Status)
	if err != nil {
		return nil, fmt.Errorf("list videos with enrichment status %s: %w", e.req.Status, err)
	}
	if e.req.Limit > 0 && len(videos) > e.req.Limit {
		videos = videos[:e.req.Limit]
	}
	items := make([]JobItem, len(videos))
	for i, v := range videos {
		items[i] = JobItem{ID: v.ID, Name: v.OriginalFilename, Run: func(ctx context.Context) error {
			return e.enrichOne(ctx, v.ID)
		}}
	}
	return items, nil
}

// enrichOne enriches one Video and, when asked, accepts its best Suggestion.
func (e *enrichmentItems) enrichOne(ctx context.Context, videoID string) error {
	if err := e.svc.EnrichVideo(ctx, videoID, e.userID); err != nil {
		return fmt.Errorf("enrich video %s: %w", videoID, err)
	}
	if !e.req.AutoAccept {
		return nil
	}
	if err := e.svc.autoAcceptHighestPriority(ctx, videoID, e.userID); err != nil {
		return fmt.Errorf("auto-accept suggestion for video %s: %w", videoID, err)
	}
	return nil
}

// enrichmentReport sends one Enrichment Job's messages. It reports only the
// start of each Video; per-Video failures go to the log.
type enrichmentReport struct {
	notifier websocket.Notifier
	userID   string
}

func (e *enrichmentReport) ItemStarted(job Job, it JobItem, current int) {
	e.send(websocket.TypeEnrichProgress, map[string]interface{}{
		"job_id": job.ID, "video_id": it.ID, "current": current, "total": job.Total, "status": "processing",
	})
}

func (e *enrichmentReport) ItemDone(job Job, res JobItemResult) {
	if res.Err != nil {
		slog.Warn("enrich batch: video failed", "job_id", job.ID, "video_id", res.Item.ID, "error", res.Err)
	}
}

func (e *enrichmentReport) Failed(job Job, err error) {
	e.send(websocket.TypeEnrichError, map[string]string{"job_id": job.ID, "error": err.Error()})
}

func (e *enrichmentReport) Finished(job Job) {
	snap := toEnrichJob(&job)
	e.send(websocket.TypeEnrichBatchComplete, snap)
	slog.Info("enrich batch finished",
		"job_id", snap.ID, "status", snap.Status, "total", snap.Total,
		"succeeded", snap.Succeeded, "failed", snap.Failed,
	)
}

func (e *enrichmentReport) send(msgType string, payload interface{}) {
	e.notifier.SendToUser(e.userID, &websocket.Message{Type: msgType, Payload: payload})
}

// toEnrichJob shows a Job in the Enrichment API's shape.
func toEnrichJob(j *Job) *model.EnrichJob {
	return &model.EnrichJob{
		ID: j.ID, Status: j.Status, Total: j.Total, Processed: j.Processed,
		Succeeded: j.Succeeded, Failed: j.Failed, CurrentVideoID: j.Current.ID,
		StartedAt: j.StartedAt, FinishedAt: j.FinishedAt,
	}
}
