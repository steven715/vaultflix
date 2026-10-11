package service

import (
	"context"
	"errors"
	"fmt"
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
// calls it from the Job's goroutine with snapshots; the per-item ones carry
// counts but not Errors.
type JobReporter interface {
	ItemStarted(job Job, item JobItem, current int)
	ItemDone(job Job, result JobItemResult)
	// Failed reports that listing the items failed; Finished still follows.
	Failed(job Job, err error)
	// Finished reports the final Job; no other Job starts until it returns.
	Finished(job Job)
}

// JobItemResult is how one item ended.
type JobItemResult struct {
	Item    JobItem
	Current int   // the item's 1-based position
	Err     error // nil on success; wraps model.ErrJobItemSkipped when skipped
}

// JobSpec describes a Job to start.
type JobSpec struct {
	Kind     string // which service starts it, e.g. "import"
	Meta     any    // the starter's own context, read back by its reporter
	Task     JobTask
	Reporter JobReporter
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

// jobLane is what a service needs from the runner of its Job Lane.
type jobLane interface {
	// Start returns model.ErrConflict while another Job runs on the lane.
	Start(spec JobSpec) (*Job, error)
	// Latest returns nil when no Job has started.
	Latest() *Job
	// Cancel returns model.ErrNotFound when jobID is not the latest Job.
	Cancel(jobID string) error
}

// JobLaneBusyError is why a Job did not start: another Job, of RunningKind,
// holds the lane. It matches model.ErrConflict.
type JobLaneBusyError struct {
	RunningKind string
}

func (e *JobLaneBusyError) Error() string {
	return fmt.Sprintf("a %s job is running on this lane", e.RunningKind)
}

func (e *JobLaneBusyError) Unwrap() error { return model.ErrConflict }

// JobRunner runs one Job at a time in the background, keeping only the latest
// in memory (ADR-0003). Cancellation takes effect between items, never
// interrupting one in flight.
type JobRunner struct {
	mu       sync.Mutex
	latest   *Job
	busy     bool // from Start until the Job's Finished report returns
	cancelCh chan struct{}
}

// NewJobRunner creates an idle JobRunner.
func NewJobRunner() *JobRunner {
	return &JobRunner{}
}

// Start launches a Job and returns a snapshot of it. The Job runs detached
// from any request. Until the previous Job has reported Finished, returns a
// *JobLaneBusyError, which matches model.ErrConflict.
func (r *JobRunner) Start(spec JobSpec) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busy {
		return nil, &JobLaneBusyError{RunningKind: r.latest.Kind}
	}
	job := &Job{ID: uuid.NewString(), Kind: spec.Kind, Meta: spec.Meta, Status: "running", StartedAt: time.Now()}
	r.latest, r.busy, r.cancelCh = job, true, make(chan struct{})
	run := &jobRun{runner: r, job: job, spec: spec}
	go run.execute(r.cancelCh)
	return cloneJob(job, true), nil
}

// Latest returns a snapshot of the most recently started Job (running or
// finished), or nil when none has started.
func (r *JobRunner) Latest() *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneJob(r.latest, true)
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

// jobRun is the state one Job's goroutine carries.
type jobRun struct {
	runner *JobRunner
	job    *Job
	spec   JobSpec
}

func (j *jobRun) execute(cancelCh <-chan struct{}) {
	defer j.finish()

	// Background, not a request context: the Job outlives the HTTP call.
	ctx := context.Background()
	items, err := j.spec.Task.Items(ctx)
	if err != nil {
		j.runner.update(j.job, func(job *Job) { job.Status, job.ListError = "failed", err.Error() })
		j.spec.Reporter.Failed(j.runner.snapshot(j.job, false), err)
		return
	}
	j.runner.update(j.job, func(job *Job) { job.Total = len(items) })

	for i, it := range items {
		select {
		case <-cancelCh:
			j.runner.update(j.job, func(job *Job) { job.Status = "cancelled" })
			return
		default:
		}
		j.runItem(ctx, it, i+1)
	}
}

// runItem processes one item and records its outcome.
func (j *jobRun) runItem(ctx context.Context, it JobItem, current int) {
	j.runner.update(j.job, func(job *Job) { job.Current = it })
	j.spec.Reporter.ItemStarted(j.runner.snapshot(j.job, false), it, current)

	err := it.Run(ctx)
	j.runner.update(j.job, func(job *Job) {
		job.Processed = current
		switch {
		case errors.Is(err, model.ErrJobItemSkipped):
			job.Skipped++
		case err != nil:
			job.Failed++
			job.Errors = append(job.Errors, JobItemError{ItemID: it.ID, ItemName: it.Name, Error: err.Error()})
		default:
			job.Succeeded++
		}
	})
	j.spec.Reporter.ItemDone(j.runner.snapshot(j.job, false), JobItemResult{Item: it, Current: current, Err: err})
}

// finish stamps the end of the Job and reports it; only then may the next
// Job start, so a kind's complete message always precedes the next Job's.
func (j *jobRun) finish() {
	finished := time.Now()
	j.runner.update(j.job, func(job *Job) {
		job.FinishedAt = &finished
		job.Current = JobItem{}
		if job.Status == "running" {
			job.Status = "completed"
		}
	})
	j.spec.Reporter.Finished(j.runner.snapshot(j.job, true))

	j.runner.mu.Lock()
	defer j.runner.mu.Unlock()
	j.runner.busy = false
}

// update applies fn to job under r.mu, serialising it against snapshots.
func (r *JobRunner) update(job *Job, fn func(*Job)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(job)
}

// snapshot copies job; without errors, it leaves Errors out, keeping
// per-item reports cheap however many items failed.
func (r *JobRunner) snapshot(job *Job, withErrors bool) Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *cloneJob(job, withErrors)
}

// cloneJob deep-copies j; the caller holds the runner's lock. Nil in, nil out.
func cloneJob(j *Job, withErrors bool) *Job {
	if j == nil {
		return nil
	}
	cp := *j
	cp.Errors = nil
	if withErrors {
		cp.Errors = append([]JobItemError(nil), j.Errors...)
	}
	if j.FinishedAt != nil {
		f := *j.FinishedAt
		cp.FinishedAt = &f
	}
	return &cp
}
