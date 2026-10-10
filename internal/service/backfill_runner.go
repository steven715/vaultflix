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

// BackfillTask is one kind of Backfill: which Videos still need the derived
// data, and how to derive it for one Video. The runner owns everything else.
type BackfillTask interface {
	// List returns the Videos this Backfill still has to process, or a
	// wrapped repository error.
	List(ctx context.Context) ([]model.Video, error)
	// ProcessOne derives the data for v. A model.ErrMediaSourceDisabled
	// (wrapped) result counts the Video as skipped; any other error as failed.
	ProcessOne(ctx context.Context, v *model.Video) error
}

// BackfillRunner runs Backfill Jobs in the background, one at a time across
// all kinds (they compete for the same disks), keeping the latest job in
// memory (ADR-0003) and pushing progress over WebSocket. Cancellation takes
// effect between Videos, never killing an in-flight ffmpeg/ffprobe.
type BackfillRunner struct {
	notifier websocket.Notifier
	tasks    map[model.BackfillKind]BackfillTask

	mu       sync.Mutex
	active   *model.BackfillJob
	cancelCh chan struct{}
}

// NewBackfillRunner creates a BackfillRunner serving the given kinds.
func NewBackfillRunner(notifier websocket.Notifier, tasks map[model.BackfillKind]BackfillTask) *BackfillRunner {
	return &BackfillRunner{notifier: notifier, tasks: tasks}
}

// Start launches a Backfill Job of kind and returns a snapshot of it. Progress
// goes to userID's WebSocket connections. ctx scopes only the call: the job
// itself runs detached from the request.
//
// Returns model.ErrInvalidInput (wrapped) for an unknown kind and
// model.ErrConflict while any Backfill Job is running.
func (r *BackfillRunner) Start(ctx context.Context, kind model.BackfillKind, userID string) (*model.BackfillJob, error) {
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
	run := &backfillRun{runner: r, job: job, task: task, userID: userID}
	go run.execute(r.cancelCh)

	// A snapshot, never the live pointer: the worker mutates job under r.mu.
	return cloneBackfillJob(job), nil
}

// Active returns a snapshot of the most recently started job (running or
// finished), or nil when none has started in this process.
func (r *BackfillRunner) Active(ctx context.Context) *model.BackfillJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneBackfillJob(r.active)
}

// Cancel asks the running job jobID to stop after its current Video.
// Cancelling a finished job is a no-op. Returns model.ErrNotFound when jobID
// is not the latest job.
func (r *BackfillRunner) Cancel(ctx context.Context, jobID string) error {
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

// backfillRun is the state one Backfill Job's worker goroutine carries.
type backfillRun struct {
	runner *BackfillRunner
	job    *model.BackfillJob
	task   BackfillTask
	userID string
}

// position is a Video's place in the job, for progress messages.
type position struct{ current, total int }

func (b *backfillRun) execute(cancelCh <-chan struct{}) {
	defer b.finish()

	// Background, not a request context: the job outlives the HTTP call, and
	// cancellation is checked between Videos only.
	ctx := context.Background()
	videos, err := b.task.List(ctx)
	if err != nil {
		b.runner.update(b.job, func(j *model.BackfillJob) { j.Status = "failed" })
		b.runner.notifier.SendToUser(b.userID, &websocket.Message{
			Type:    websocket.TypeBackfillError,
			Payload: map[string]string{"job_id": b.job.ID, "kind": string(b.job.Kind), "error": err.Error()},
		})
		return
	}
	total := len(videos)
	b.runner.update(b.job, func(j *model.BackfillJob) { j.Total = total })

	for i := range videos {
		select {
		case <-cancelCh:
			b.runner.update(b.job, func(j *model.BackfillJob) { j.Status = "cancelled" })
			return
		default:
		}
		b.processOne(ctx, &videos[i], position{current: i + 1, total: total})
	}
}

// processOne runs one Video and records its outcome on the job and over WS.
func (b *backfillRun) processOne(ctx context.Context, v *model.Video, pos position) {
	job := b.job
	b.runner.update(job, func(j *model.BackfillJob) { j.CurrentVideoID = v.ID })
	progress := model.BackfillProgress{
		JobID: job.ID, Kind: job.Kind, VideoID: v.ID, OriginalFilename: v.OriginalFilename,
		Current: pos.current, Total: pos.total, Status: "processing",
	}
	b.runner.sendProgress(b.userID, progress)

	err := b.task.ProcessOne(ctx, v)
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
	b.runner.update(job, func(j *model.BackfillJob) {
		j.Processed = pos.current
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
	b.runner.sendProgress(b.userID, progress)
}

// finish stamps the end of the job and announces it.
func (b *backfillRun) finish() {
	finished := time.Now()
	b.runner.update(b.job, func(j *model.BackfillJob) {
		j.FinishedAt = &finished
		j.CurrentVideoID = ""
		if j.Status == "running" {
			j.Status = "completed"
		}
	})
	snap := b.runner.snapshot(b.job)
	b.runner.notifier.SendToUser(b.userID, &websocket.Message{Type: websocket.TypeBackfillComplete, Payload: snap})
	slog.Info("backfill job finished",
		"job_id", snap.ID, "kind", snap.Kind, "status", snap.Status, "total", snap.Total,
		"succeeded", snap.Succeeded, "skipped", snap.Skipped, "failed", snap.Failed,
	)
}

// resolveVideoFile turns v into its on-disk path for a Backfill, failing (not
// panicking) for a Video without a Media Source. Errors carry the Video id and
// keep model.ErrMediaSourceDisabled visible to errors.Is.
func resolveVideoFile(ctx context.Context, files mediaFileResolver, v *model.Video) (string, error) {
	if v.SourceID == nil || v.FilePath == nil {
		return "", fmt.Errorf("video %s has no media source (legacy MinIO-stored video)", v.ID)
	}
	abs, err := files.ResolveFile(ctx, *v.SourceID, *v.FilePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file of video %s: %w", v.ID, err)
	}
	return abs, nil
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
