package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/websocket"
)

func videosNamed(ids ...string) []model.Video {
	vs := make([]model.Video, len(ids))
	for i, id := range ids {
		vs[i] = model.Video{ID: id, OriginalFilename: id + ".mkv"}
	}
	return vs
}

// waitForBackfill polls until the active job reaches want or the test fails.
func waitForBackfill(t *testing.T, r *BackfillRunner, jobID, want string) *model.BackfillJob {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if job := r.Active(); job != nil && job.ID == jobID && job.Status == want {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s never reached status %q (now %+v)", jobID, want, r.Active())
	return nil
}

func TestBackfillRunner_Start_CountsEachOutcome(t *testing.T) {
	task := &mock.BackfillTask{
		Videos: videosNamed("ok", "disabled", "broken"),
		ProcessFunc: func(_ context.Context, v *model.Video) error {
			switch v.ID {
			case "disabled":
				return fmt.Errorf("resolve: %w", model.ErrMediaSourceDisabled)
			case "broken":
				return errors.New("ffprobe exploded")
			}
			return nil
		},
	}
	r := NewBackfillRunner(&mock.Notifier{}, map[model.BackfillKind]backfillTask{model.BackfillCodec: task})

	job, err := r.Start(model.BackfillCodec, "user-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	final := waitForBackfill(t, r, job.ID, "completed")

	if final.Kind != model.BackfillCodec {
		t.Errorf("kind = %q, want codec", final.Kind)
	}
	if final.Total != 3 || final.Processed != 3 || final.Succeeded != 1 || final.Skipped != 1 || final.Failed != 1 {
		t.Errorf("counts total=%d processed=%d succeeded=%d skipped=%d failed=%d, want 3/3/1/1/1",
			final.Total, final.Processed, final.Succeeded, final.Skipped, final.Failed)
	}
	if len(final.Errors) != 1 || final.Errors[0].VideoID != "broken" {
		t.Errorf("errors = %+v, want one for broken", final.Errors)
	}
}

func TestBackfillRunner_Start_UnknownKind(t *testing.T) {
	r := NewBackfillRunner(&mock.Notifier{}, map[model.BackfillKind]backfillTask{})
	if _, err := r.Start("thumbnail", "user-1"); !errors.Is(err, model.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

// Only one Backfill Job runs at a time, whatever its kind.
func TestBackfillRunner_Start_ConflictWhileAnyKindRuns(t *testing.T) {
	release := make(chan struct{})
	slow := &mock.BackfillTask{Videos: videosNamed("a"), ProcessFunc: func(context.Context, *model.Video) error {
		<-release
		return nil
	}}
	r := NewBackfillRunner(&mock.Notifier{}, map[model.BackfillKind]backfillTask{
		model.BackfillKeyframe: slow,
		model.BackfillCode:     &mock.BackfillTask{},
	})
	job, err := r.Start(model.BackfillKeyframe, "user-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if _, err := r.Start(model.BackfillCode, "user-1"); !errors.Is(err, model.ErrConflict) {
		t.Errorf("second Start err = %v, want ErrConflict", err)
	}
	close(release)
	waitForBackfill(t, r, job.ID, "completed")
}

func TestBackfillRunner_Cancel_StopsBetweenVideos(t *testing.T) {
	started, release := make(chan struct{}, 3), make(chan struct{})
	task := &mock.BackfillTask{Videos: videosNamed("a", "b", "c"), ProcessFunc: func(context.Context, *model.Video) error {
		started <- struct{}{}
		<-release
		return nil
	}}
	r := NewBackfillRunner(&mock.Notifier{}, map[model.BackfillKind]backfillTask{model.BackfillPreview: task})
	job, err := r.Start(model.BackfillPreview, "user-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started
	if err := r.Cancel(job.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	close(release) // the in-flight Video finishes normally

	final := waitForBackfill(t, r, job.ID, "cancelled")
	if got := task.Processed(); len(got) != 1 || final.Succeeded != 1 {
		t.Errorf("processed %v (succeeded=%d), want only the in-flight Video", got, final.Succeeded)
	}
}

func TestBackfillRunner_Cancel_UnknownJob(t *testing.T) {
	r := NewBackfillRunner(&mock.Notifier{}, map[model.BackfillKind]backfillTask{})
	if err := r.Cancel("nope"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestBackfillRunner_Start_ListFailureFailsJob(t *testing.T) {
	notifier := &mock.Notifier{}
	r := NewBackfillRunner(notifier, map[model.BackfillKind]backfillTask{
		model.BackfillCode: &mock.BackfillTask{ListErr: errors.New("db down")},
	})
	job, err := r.Start(model.BackfillCode, "user-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForBackfill(t, r, job.ID, "failed")
	if !hasMessage(notifier, websocket.TypeBackfillError) {
		t.Error("no backfill_error message sent")
	}
}

func TestBackfillRunner_Start_ProgressCarriesKind(t *testing.T) {
	notifier := &mock.Notifier{}
	r := NewBackfillRunner(notifier, map[model.BackfillKind]backfillTask{
		model.BackfillKeyframe: &mock.BackfillTask{Videos: videosNamed("a", "b")},
	})
	job, err := r.Start(model.BackfillKeyframe, "user-1")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForBackfill(t, r, job.ID, "completed")

	var progress int
	for _, m := range notifier.GetMessages() {
		if p, ok := m.Payload.(model.BackfillProgress); ok {
			progress++
			if p.Kind != model.BackfillKeyframe {
				t.Errorf("progress kind = %q, want keyframe", p.Kind)
			}
		}
	}
	if progress != 4 { // processing + success per Video
		t.Errorf("progress messages = %d, want 4", progress)
	}
	if !hasMessage(notifier, websocket.TypeBackfillComplete) {
		t.Error("no backfill_complete message sent")
	}
}

func hasMessage(n *mock.Notifier, msgType string) bool {
	for _, m := range n.GetMessages() {
		if m.Type == msgType {
			return true
		}
	}
	return false
}
