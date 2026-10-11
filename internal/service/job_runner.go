package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/steven/vaultflix/internal/model"
)

// JobItem is one unit of a Job: a file for an Import Job, a Video for a
// Backfill or Enrichment Job. Run does the work for this item.
type JobItem struct {
	ID   string // a file path or a Video id
	Name string // shown in progress messages and the failure list
	Run  func(ctx context.Context) error
}

// JobTask lists what one Job will process. A Run error wrapping
// model.ErrJobItemSkipped counts the item as skipped; any other error as
// failed.
type JobTask interface {
	// Items returns the items to process, or an error that fails the Job.
	Items(ctx context.Context) ([]JobItem, error)
}

// JobReporter turns a Job's lifecycle into its kind's own messages. The runner
// calls it from the Job's goroutine, always with a snapshot.
type JobReporter interface {
	ItemStarted(job Job, item JobItem, current int)
	ItemDone(job Job, item JobItem, current int, err error)
	// Failed reports that listing the items failed; Finished still follows.
	Failed(job Job, err error)
	Finished(job Job)
}

// JobItemError records one failed item.
type JobItemError struct {
	ItemID   string
	ItemName string
	Error    string
}

// Job is a snapshot of one Job's state.
type Job struct {
	ID        string
	Kind      string // which service started it, e.g. "import"
	Meta      any    // the starter's own context, e.g. the Media Source
	Status    string // running | completed | failed | cancelled
	Total     int
	Processed int
	Succeeded int
	Skipped   int
	Failed    int
	Current   JobItem // the item in progress; zero when idle
	Errors    []JobItemError
	ListError string // why listing the items failed, if it did
	StartedAt time.Time
	// FinishedAt is nil while the Job runs.
	FinishedAt *time.Time
}

// JobRunner runs one Job at a time in the background, keeping only the latest
// in memory (ADR-0003). Cancellation takes effect between items, never
// interrupting one in flight.
type JobRunner struct {
	mu       sync.Mutex
	latest   *Job
	cancelCh chan struct{}
}

// NewJobRunner creates an idle JobRunner.
func NewJobRunner() *JobRunner {
	return &JobRunner{}
}

// Start launches a Job over task and returns a snapshot of it. The Job runs
// detached from ctx-scoped requests. Returns model.ErrConflict while another
// Job is running.
func (r *JobRunner) Start(kind string, meta any, task JobTask, rep JobReporter) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.latest != nil && r.latest.Status == "running" {
		return nil, model.ErrConflict
	}
	job := &Job{ID: uuid.NewString(), Kind: kind, Meta: meta, Status: "running", StartedAt: time.Now()}
	r.latest, r.cancelCh = job, make(chan struct{})
	go r.run(job, task, rep, r.cancelCh)
	return cloneJob(job), nil
}

// Latest returns a snapshot of the most recently started Job (running or
// finished), or nil when none has started.
func (r *JobRunner) Latest() *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneJob(r.latest)
}

// Cancel asks the running Job jobID to stop after its current item.
// Cancelling a finished Job is a no-op. Returns model.ErrNotFound when jobID
// is not the latest Job.
func (r *JobRunner) Cancel(jobID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.latest == nil || r.latest.ID != jobID {
		return model.ErrNotFound
	}
	if r.latest.Status != "running" {
		return nil
	}
	select {
	case <-r.cancelCh: // already closed
	default:
		close(r.cancelCh)
	}
	return nil
}

func (r *JobRunner) run(job *Job, task JobTask, rep JobReporter, cancelCh <-chan struct{}) {
	defer r.finish(job, rep)

	// Background, not a request context: the Job outlives the HTTP call.
	ctx := context.Background()
	items, err := task.Items(ctx)
	if err != nil {
		r.update(job, func(j *Job) { j.Status, j.ListError = "failed", err.Error() })
		rep.Failed(r.snapshot(job), err)
		return
	}
	r.update(job, func(j *Job) { j.Total = len(items) })

	for i, it := range items {
		select {
		case <-cancelCh:
			r.update(job, func(j *Job) { j.Status = "cancelled" })
			return
		default:
		}
		r.runItem(ctx, job, rep, it, i+1)
	}
}

// runItem processes one item and records its outcome.
func (r *JobRunner) runItem(ctx context.Context, job *Job, rep JobReporter, it JobItem, current int) {
	r.update(job, func(j *Job) { j.Current = it })
	rep.ItemStarted(r.snapshot(job), it, current)

	err := it.Run(ctx)
	r.update(job, func(j *Job) {
		j.Processed = current
		switch {
		case errors.Is(err, model.ErrJobItemSkipped):
			j.Skipped++
		case err != nil:
			j.Failed++
			j.Errors = append(j.Errors, JobItemError{ItemID: it.ID, ItemName: it.Name, Error: err.Error()})
		default:
			j.Succeeded++
		}
	})
	rep.ItemDone(r.snapshot(job), it, current, err)
}

// finish stamps the end of the Job and reports it.
func (r *JobRunner) finish(job *Job, rep JobReporter) {
	finished := time.Now()
	r.update(job, func(j *Job) {
		j.FinishedAt = &finished
		j.Current = JobItem{}
		if j.Status == "running" {
			j.Status = "completed"
		}
	})
	rep.Finished(r.snapshot(job))
}

// update applies fn to job under r.mu, serialising it against snapshots.
func (r *JobRunner) update(job *Job, fn func(*Job)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(job)
}

func (r *JobRunner) snapshot(job *Job) Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *cloneJob(job)
}

// cloneJob deep-copies j; the caller holds the runner's lock. Nil in, nil out.
func cloneJob(j *Job) *Job {
	if j == nil {
		return nil
	}
	cp := *j
	cp.Errors = append([]JobItemError(nil), j.Errors...)
	if j.FinishedAt != nil {
		f := *j.FinishedAt
		cp.FinishedAt = &f
	}
	return &cp
}
