package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/websocket"
)

// backfillTask is one kind of Backfill: which Videos still need the derived
// data, and how to derive it for one Video. The runner owns everything else.
type backfillTask interface {
	// List returns the Videos this Backfill still has to process, or a
	// wrapped repository error.
	List(ctx context.Context) ([]model.Video, error)
	// ProcessOne derives the data for v. A model.ErrMediaSourceDisabled
	// (wrapped) result counts the Video as skipped; any other error as failed.
	ProcessOne(ctx context.Context, v *model.Video) error
}

// BackfillTasks names the production task of each Backfill kind, so wiring
// code outside this package can build the runner's kind → task table.
type BackfillTasks struct {
	Preview  *PreviewBackfill
	Codec    *CodecBackfillService
	Keyframe *KeyframeBackfill
	Code     *CodeBackfill
}

// ByKind returns the kind → task table NewBackfillRunner takes.
func (t BackfillTasks) ByKind() map[model.BackfillKind]backfillTask {
	return map[model.BackfillKind]backfillTask{
		model.BackfillPreview:  t.Preview,
		model.BackfillCodec:    t.Codec,
		model.BackfillKeyframe: t.Keyframe,
		model.BackfillCode:     t.Code,
	}
}

// BackfillRunner runs Backfill Jobs in the background, one at a time across
// all kinds (they compete for the same disks), keeping the latest job in
// memory (ADR-0003) and pushing progress over WebSocket. Cancellation takes
// effect between Videos, never killing an in-flight ffmpeg/ffprobe.
type BackfillRunner struct {
	notifier websocket.Notifier
	tasks    map[model.BackfillKind]backfillTask

	mu       sync.Mutex
	active   *model.BackfillJob
	cancelCh chan struct{}
}

// NewBackfillRunner creates a BackfillRunner serving the given kinds.
func NewBackfillRunner(notifier websocket.Notifier, tasks map[model.BackfillKind]backfillTask) *BackfillRunner {
	return &BackfillRunner{notifier: notifier, tasks: tasks}
}

// Start launches a Backfill Job of kind and returns a snapshot of it. Progress
// goes to userID's WebSocket connections.
//
// Returns model.ErrInvalidInput (wrapped) for an unknown kind and
// model.ErrConflict while any Backfill Job is running.
func (r *BackfillRunner) Start(kind model.BackfillKind, userID string) (*model.BackfillJob, error) {
	task, ok := r.tasks[kind]
	if !ok {
		return nil, fmt.Errorf("backfill kind %q: %w", kind, model.ErrInvalidInput)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active != nil && r.active.Status == "running" {
		return nil, model.ErrConflict
	}
	job := &model.BackfillJob{
		ID:        uuid.NewString(),
		Kind:      kind,
		Status:    "running",
		Errors:    []model.BackfillError{},
		StartedAt: time.Now(),
	}
	r.active, r.cancelCh = job, make(chan struct{})
	go r.run(job, task, r.cancelCh, userID)

	// A snapshot, never the live pointer: the worker mutates job under r.mu.
	return cloneBackfillJob(job), nil
}

// Active returns a snapshot of the most recently started job (running or
// finished), or nil when none has started in this process.
func (r *BackfillRunner) Active() *model.BackfillJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneBackfillJob(r.active)
}

// Cancel asks the running job jobID to stop after its current Video.
// Cancelling a finished job is a no-op. Returns model.ErrNotFound when jobID
// is not the latest job.
func (r *BackfillRunner) Cancel(jobID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active == nil || r.active.ID != jobID {
		return model.ErrNotFound
	}
	if r.active.Status != "running" {
		return nil
	}
	select {
	case <-r.cancelCh: // already closed
	default:
		close(r.cancelCh)
	}
	return nil
}

func (r *BackfillRunner) run(job *model.BackfillJob, task backfillTask, cancelCh chan struct{}, userID string) {
	defer r.finish(job, userID)

	// Background, not a request context: the job outlives the HTTP call, and
	// cancellation is checked between Videos only.
	ctx := context.Background()
	videos, err := task.List(ctx)
	if err != nil {
		r.update(job, func(j *model.BackfillJob) { j.Status = "failed" })
		r.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeBackfillError,
			Payload: map[string]string{"job_id": job.ID, "error": err.Error()},
		})
		return
	}
	total := len(videos)
	r.update(job, func(j *model.BackfillJob) { j.Total = total })

	for i := range videos {
		select {
		case <-cancelCh:
			r.update(job, func(j *model.BackfillJob) { j.Status = "cancelled" })
			return
		default:
		}
		r.processOne(ctx, job, task, &videos[i], i+1, total, userID)
	}
}

// processOne runs one Video and records its outcome on the job and over WS.
func (r *BackfillRunner) processOne(ctx context.Context, job *model.BackfillJob, task backfillTask, v *model.Video, current, total int, userID string) {
	r.update(job, func(j *model.BackfillJob) { j.CurrentVideoID = v.ID })
	progress := model.BackfillProgress{
		JobID: job.ID, Kind: job.Kind, VideoID: v.ID, OriginalFilename: v.OriginalFilename,
		Current: current, Total: total, Status: "processing",
	}
	r.sendProgress(userID, progress)

	err := task.ProcessOne(ctx, v)
	switch {
	case errors.Is(err, model.ErrMediaSourceDisabled):
		slog.Info("backfill skipped video, media source disabled", "job_id", job.ID, "kind", job.Kind, "video_id", v.ID)
		progress.Status = "skipped"
	case err != nil:
		slog.Warn("backfill failed for video", "job_id", job.ID, "kind", job.Kind, "video_id", v.ID, "error", err)
		progress.Status, progress.Error = "error", err.Error()
	default:
		progress.Status = "success"
	}
	r.update(job, func(j *model.BackfillJob) {
		j.Processed = current
		switch progress.Status {
		case "skipped":
			j.Skipped++
		case "error":
			j.Failed++
			j.Errors = append(j.Errors, model.BackfillError{VideoID: v.ID, OriginalFilename: v.OriginalFilename, Error: progress.Error})
		default:
			j.Succeeded++
		}
	})
	r.sendProgress(userID, progress)
}

// finish stamps the end of job and announces it.
func (r *BackfillRunner) finish(job *model.BackfillJob, userID string) {
	finished := time.Now()
	r.update(job, func(j *model.BackfillJob) {
		j.FinishedAt = &finished
		j.CurrentVideoID = ""
		if j.Status == "running" {
			j.Status = "completed"
		}
	})
	snap := r.snapshot(job)
	r.notifier.SendToUser(userID, &websocket.Message{Type: websocket.TypeBackfillComplete, Payload: snap})
	slog.Info("backfill job finished",
		"job_id", snap.ID, "kind", snap.Kind, "status", snap.Status, "total", snap.Total,
		"succeeded", snap.Succeeded, "skipped", snap.Skipped, "failed", snap.Failed,
	)
}

func (r *BackfillRunner) sendProgress(userID string, p model.BackfillProgress) {
	r.notifier.SendToUser(userID, &websocket.Message{Type: websocket.TypeBackfillProgress, Payload: p})
}

// update applies fn to job under r.mu, serialising it against snapshots.
func (r *BackfillRunner) update(job *model.BackfillJob, fn func(*model.BackfillJob)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(job)
}

func (r *BackfillRunner) snapshot(job *model.BackfillJob) *model.BackfillJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneBackfillJob(job)
}

// cloneBackfillJob deep-copies j; the caller holds the runner's lock. Nil in,
// nil out.
func cloneBackfillJob(j *model.BackfillJob) *model.BackfillJob {
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
