package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

// listTask is a JobTask over a fixed list of items.
type listTask struct {
	items []JobItem
	err   error
}

func (t listTask) Items(context.Context) ([]JobItem, error) { return t.items, t.err }

func item(id string, run func(context.Context) error) JobItem {
	return JobItem{ID: id, Name: id + ".mp4", Run: run}
}

func ok(context.Context) error { return nil }

// recorder is a JobReporter that keeps every call, and signals when the job ends.
type recorder struct {
	mu       sync.Mutex
	started  []string
	done     []Job
	failed   []error
	finished chan Job
}

func newRecorder() *recorder { return &recorder{finished: make(chan Job, 1)} }

func (r *recorder) ItemStarted(_ Job, it JobItem, _ int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = append(r.started, it.ID)
}

func (r *recorder) ItemDone(j Job, _ JobItemResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = append(r.done, j)
}

func (r *recorder) Failed(_ Job, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed = append(r.failed, err)
}

func (r *recorder) Finished(j Job) { r.finished <- j }

func (r *recorder) wait(t *testing.T) Job {
	t.Helper()
	select {
	case j := <-r.finished:
		return j
	case <-time.After(2 * time.Second):
		t.Fatal("job did not finish")
		return Job{}
	}
}

func TestJobRunner_CountsEachOutcome(t *testing.T) {
	r := NewJobRunner()
	rec := newRecorder()
	task := listTask{items: []JobItem{
		item("a", ok),
		item("b", func(context.Context) error { return fmt.Errorf("dup: %w", model.ErrJobItemSkipped) }),
		item("c", func(context.Context) error { return errors.New("boom") }),
	}}

	started, err := r.Start(JobSpec{Kind: "import", Meta: "src-1", Task: task, Reporter: rec})
	if err != nil || started.Status != "running" || started.Kind != "import" || started.Meta != "src-1" {
		t.Fatalf("Start = %+v, %v", started, err)
	}
	j := rec.wait(t)

	if j.Status != "completed" || j.Total != 3 || j.Processed != 3 ||
		j.Succeeded != 1 || j.Skipped != 1 || j.Failed != 1 || j.FinishedAt == nil {
		t.Errorf("finished job = %+v", j)
	}
	if len(j.Errors) != 1 || j.Errors[0].ItemID != "c" || j.Errors[0].ItemName != "c.mp4" || j.Errors[0].Error != "boom" {
		t.Errorf("errors = %+v", j.Errors)
	}
	if fmt.Sprint(rec.started) != "[a b c]" {
		t.Errorf("started = %v", rec.started)
	}
	// Each done report carries the job's counts after that item.
	if last := rec.done[2]; last.Processed != 3 || last.Failed != 1 || last.Current.ID != "c" {
		t.Errorf("last done = %+v", last)
	}
	if j.Current.ID != "" {
		t.Errorf("current item kept after finish: %+v", j.Current)
	}
}

func TestJobRunner_OneJobAtATime(t *testing.T) {
	r := NewJobRunner()
	release := make(chan struct{})
	rec := newRecorder()
	if _, err := r.Start(JobSpec{Kind: "backfill", Task: listTask{items: []JobItem{item("a", func(context.Context) error {
		<-release
		return nil
	})}}, Reporter: rec}); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Start(JobSpec{Kind: "import", Task: listTask{}, Reporter: newRecorder()}); !errors.Is(err, model.ErrConflict) {
		t.Fatalf("second Start err = %v, want ErrConflict", err)
	}
	close(release)
	rec.wait(t)

	again := newRecorder()
	if _, err := r.Start(JobSpec{Kind: "import", Task: listTask{}, Reporter: again}); err != nil {
		t.Fatalf("Start after finish: %v", err)
	}
	again.wait(t)
}

func TestJobRunner_CancelStopsBetweenItems(t *testing.T) {
	r := NewJobRunner()
	rec := newRecorder()
	inFirst, release := make(chan struct{}), make(chan struct{})
	job, _ := r.Start(JobSpec{Kind: "backfill", Task: listTask{items: []JobItem{
		item("a", func(context.Context) error { close(inFirst); <-release; return nil }),
		item("b", ok),
	}}, Reporter: rec})

	<-inFirst
	if err := r.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	j := rec.wait(t)

	// The item in flight finishes; the next one never starts.
	if j.Status != "cancelled" || j.Processed != 1 || j.Succeeded != 1 {
		t.Errorf("job = %+v", j)
	}
	if err := r.Cancel(job.ID); err != nil {
		t.Errorf("cancelling a finished job: %v, want nil", err)
	}
}

func TestJobRunner_CancelUnknownJob(t *testing.T) {
	if err := NewJobRunner().Cancel("nope"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestJobRunner_ListFailureFailsTheJob(t *testing.T) {
	r := NewJobRunner()
	rec := newRecorder()
	_, _ = r.Start(JobSpec{Kind: "import", Task: listTask{err: errors.New("walk failed")}, Reporter: rec})
	j := rec.wait(t)

	if j.Status != "failed" || j.ListError != "walk failed" {
		t.Errorf("job = %+v", j)
	}
	if len(rec.failed) != 1 {
		t.Errorf("Failed reports = %v", rec.failed)
	}
}

func TestJobRunner_LatestIsASnapshot(t *testing.T) {
	r := NewJobRunner()
	if r.Latest() != nil {
		t.Fatal("Latest before any job should be nil")
	}
	rec := newRecorder()
	_, _ = r.Start(JobSpec{Kind: "import", Task: listTask{items: []JobItem{item("a", func(context.Context) error { return errors.New("x") })}}, Reporter: rec})
	rec.wait(t)

	got := r.Latest()
	got.Errors[0].Error = "changed"
	got.Failed = 99
	if again := r.Latest(); again.Errors[0].Error != "x" || again.Failed != 1 {
		t.Errorf("Latest leaked the live job: %+v", again)
	}
}

// blockingFinish holds the Finished report until released.
type blockingFinish struct {
	recorder
	inFinish, release chan struct{}
}

func (b *blockingFinish) Finished(Job) {
	close(b.inFinish)
	<-b.release
}

func TestJobRunner_NextJobWaitsForTheFinishedReport(t *testing.T) {
	r := NewJobRunner()
	rep := &blockingFinish{inFinish: make(chan struct{}), release: make(chan struct{})}
	_, _ = r.Start(JobSpec{Kind: "import", Task: listTask{}, Reporter: rep})
	<-rep.inFinish

	// The Job is already completed, but its complete message is still going out.
	if _, err := r.Start(JobSpec{Kind: "import", Task: listTask{}, Reporter: newRecorder()}); !errors.Is(err, model.ErrConflict) {
		t.Errorf("Start during Finished = %v, want ErrConflict", err)
	}
	close(rep.release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		next := newRecorder()
		if _, err := r.Start(JobSpec{Kind: "import", Task: listTask{}, Reporter: next}); err == nil {
			next.wait(t)
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("runner stayed busy after Finished returned")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestJobRunner_BusyErrorNamesTheRunningKind(t *testing.T) {
	r := NewJobRunner()
	release := make(chan struct{})
	rec := newRecorder()
	_, _ = r.Start(JobSpec{Kind: "import", Task: listTask{items: []JobItem{item("a", func(context.Context) error {
		<-release
		return nil
	})}}, Reporter: rec})

	_, err := r.Start(JobSpec{Kind: "backfill", Task: listTask{}, Reporter: newRecorder()})
	var busy *JobLaneBusyError
	if !errors.Is(err, model.ErrConflict) || !errors.As(err, &busy) || busy.RunningKind != "import" {
		t.Errorf("err = %v, want a JobLaneBusyError naming import", err)
	}
	close(release)
	rec.wait(t)
}

func TestJobRunner_ShowsTheEndOnlyOnceTheLaneIsFree(t *testing.T) {
	r := NewJobRunner()
	rep := &blockingFinish{inFinish: make(chan struct{}), release: make(chan struct{})}
	_, _ = r.Start(JobSpec{Kind: "backfill", Task: listTask{}, Reporter: rep})
	<-rep.inFinish

	// While the complete message is still going out, pollers must not see a
	// finished Job they cannot yet follow with another.
	if got := r.Latest(); got.Status != "running" {
		t.Errorf("Latest during Finished = %q, want running until the lane is free", got.Status)
	}
	close(rep.release)
	deadline := time.Now().Add(2 * time.Second)
	for r.Latest().Status == "running" {
		if time.Now().After(deadline) {
			t.Fatal("job never showed as finished")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := r.Start(JobSpec{Kind: "import", Task: listTask{}, Reporter: newRecorder()}); err != nil {
		t.Errorf("Start once the Job shows finished = %v, want the lane free", err)
	}
}
