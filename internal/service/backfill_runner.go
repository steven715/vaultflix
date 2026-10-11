package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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

// BackfillRunner runs Backfill Jobs on a JobRunner, one at a time across all
// kinds (they compete for the same disks), and pushes progress over WebSocket
// in the Backfill messages. Cancellation takes effect between Videos, never
// killing an in-flight ffmpeg/ffprobe.
type BackfillRunner struct {
	notifier websocket.Notifier
	tasks    map[model.BackfillKind]BackfillTask
	jobs     jobLane
}

// NewBackfillRunner creates a BackfillRunner serving the given kinds.
func NewBackfillRunner(notifier websocket.Notifier, tasks map[model.BackfillKind]BackfillTask) *BackfillRunner {
	return &BackfillRunner{notifier: notifier, tasks: tasks, jobs: NewJobRunner()}
}

// jobKindBackfill tags Backfill Jobs on the JobRunner.
const jobKindBackfill = "backfill"

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
	job, err := r.jobs.Start(JobSpec{
		Kind: jobKindBackfill, Meta: kind, Task: &backfillItems{task: task},
		Reporter: &backfillReport{notifier: r.notifier, userID: userID},
	})
	if err != nil {
		return nil, fmt.Errorf("start backfill %s: %w", kind, err)
	}
	return toBackfillJob(job), nil
}

// Active returns a snapshot of the most recently started job (running or
// finished), or nil when none has started in this process.
func (r *BackfillRunner) Active(ctx context.Context) *model.BackfillJob {
	job := r.jobs.Latest()
	if job == nil || job.Kind != jobKindBackfill {
		return nil
	}
	return toBackfillJob(job)
}

// Cancel asks the running job jobID to stop after its current Video.
// Cancelling a finished job is a no-op. Returns model.ErrNotFound when jobID
// is not the latest job.
func (r *BackfillRunner) Cancel(ctx context.Context, jobID string) error {
	if err := r.jobs.Cancel(jobID); err != nil {
		return fmt.Errorf("cancel backfill %s: %w", jobID, err)
	}
	return nil
}

// backfillItems lists a BackfillTask's Videos as Job items. A disabled Media
// Source skips the Video.
type backfillItems struct{ task BackfillTask }

func (b *backfillItems) Items(ctx context.Context) ([]JobItem, error) {
	videos, err := b.task.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list videos to backfill: %w", err)
	}
	items := make([]JobItem, len(videos))
	for i := range videos {
		v := &videos[i]
		items[i] = JobItem{ID: v.ID, Name: v.OriginalFilename, Run: func(ctx context.Context) error {
			err := b.task.ProcessOne(ctx, v)
			switch {
			case errors.Is(err, model.ErrMediaSourceDisabled):
				return fmt.Errorf("%w: %w", model.ErrJobItemSkipped, err)
			case err != nil:
				return fmt.Errorf("backfill video %s: %w", v.ID, err)
			}
			return nil
		}}
	}
	return items, nil
}

// backfillReport sends one Backfill Job's Backfill messages.
type backfillReport struct {
	notifier websocket.Notifier
	userID   string
}

func (b *backfillReport) progress(job Job, it JobItem, current int) model.BackfillProgress {
	return model.BackfillProgress{
		JobID: job.ID, Kind: job.Meta.(model.BackfillKind), VideoID: it.ID, OriginalFilename: it.Name,
		Current: current, Total: job.Total, Status: "processing",
	}
}

func (b *backfillReport) ItemStarted(job Job, it JobItem, current int) {
	b.send(websocket.TypeBackfillProgress, b.progress(job, it, current))
}

func (b *backfillReport) ItemDone(job Job, res JobItemResult) {
	it, err := res.Item, res.Err
	p := b.progress(job, it, res.Current)
	switch {
	case errors.Is(err, model.ErrJobItemSkipped):
		slog.Info("backfill skipped video, media source disabled", "job_id", job.ID, "kind", p.Kind, "video_id", it.ID)
		p.Status = "skipped"
	case err != nil:
		slog.Warn("backfill failed for video", "job_id", job.ID, "kind", p.Kind, "video_id", it.ID, "error", err)
		p.Status, p.Error = "error", err.Error()
	default:
		p.Status = "success"
	}
	p.Succeeded, p.Skipped, p.Failed = job.Succeeded, job.Skipped, job.Failed
	b.send(websocket.TypeBackfillProgress, p)
}

func (b *backfillReport) Failed(job Job, err error) {
	b.send(websocket.TypeBackfillError, map[string]string{
		"job_id": job.ID, "kind": string(job.Meta.(model.BackfillKind)), "error": err.Error(),
	})
}

func (b *backfillReport) Finished(job Job) {
	snap := toBackfillJob(&job)
	b.send(websocket.TypeBackfillComplete, snap)
	slog.Info("backfill job finished",
		"job_id", snap.ID, "kind", snap.Kind, "status", snap.Status, "total", snap.Total,
		"succeeded", snap.Succeeded, "skipped", snap.Skipped, "failed", snap.Failed,
	)
}

func (b *backfillReport) send(msgType string, payload interface{}) {
	b.notifier.SendToUser(b.userID, &websocket.Message{Type: msgType, Payload: payload})
}

// toBackfillJob shows a Job in the Backfill API's shape.
func toBackfillJob(j *Job) *model.BackfillJob {
	errs := make([]model.BackfillError, 0, len(j.Errors))
	for _, e := range j.Errors {
		errs = append(errs, model.BackfillError{VideoID: e.ItemID, OriginalFilename: e.ItemName, Error: e.Error})
	}
	return &model.BackfillJob{
		ID: j.ID, Kind: j.Meta.(model.BackfillKind), Status: j.Status,
		Total: j.Total, Processed: j.Processed, Succeeded: j.Succeeded, Failed: j.Failed, Skipped: j.Skipped,
		CurrentVideoID: j.Current.ID, Errors: errs, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt,
	}
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
