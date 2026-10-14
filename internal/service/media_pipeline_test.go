package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

// The pipeline's Import and Backfill Jobs share one Media Lane (ADR-0014):
// while a Backfill runs, an Import is refused, naming the Backfill.
func TestNewMediaPipeline_ImportAndBackfillShareTheMediaLane(t *testing.T) {
	listing := make(chan struct{})
	gate := make(chan struct{})
	repo := &mock.VideoRepository{
		ListByEnrichmentStatusFunc: func(context.Context, string) ([]model.Video, error) {
			close(listing)
			<-gate
			return nil, nil
		},
	}
	p := NewMediaPipeline(MediaPipelineDeps{
		Videos:    repo,
		Files:     mock.ResolveUnder(t.TempDir()),
		Media:     NewMediaProcessor(&mock.MediaTool{}, &mock.MinIOClient{}),
		Keyframes: NewKeyframeService(newFakeKeyframeRepo()),
		Notifier:  &mock.Notifier{},
	})
	if _, err := p.Backfill.Start(context.Background(), model.BackfillCode, "user-1"); err != nil {
		t.Fatalf("Backfill Start: %v", err)
	}
	defer close(gate)
	<-listing

	_, err := p.Import.StartAsync(context.Background(), &model.MediaSource{ID: "src-1", MountPath: t.TempDir()}, "user-1")

	var busy *JobLaneBusyError
	if !errors.As(err, &busy) || busy.RunningKind != JobKindBackfill {
		t.Errorf("Import Start = %v, want a busy Media Lane naming the Backfill Job", err)
	}
}

// mediaLane puts a blocked Import and a BackfillRunner on one Job Lane.
func mediaLane(t *testing.T) (*blockedImport, *BackfillRunner) {
	t.Helper()
	b := newBlockedImport(t, 1)
	backfill := NewBackfillRunner(&mock.Notifier{}, map[model.BackfillKind]BackfillTask{model.BackfillCode: &mock.BackfillTask{}}, b.svc.jobs)
	return b, backfill
}

func TestMediaLane_ImportBlocksBackfill(t *testing.T) {
	b, backfill := mediaLane(t)
	job := b.start(t)

	_, err := backfill.Start(context.Background(), model.BackfillCode, "user-1")
	var busy *JobLaneBusyError
	if !errors.Is(err, model.ErrConflict) || !errors.As(err, &busy) || busy.RunningKind != JobKindImport {
		t.Fatalf("Backfill Start = %v, want a busy lane naming the Import Job", err)
	}
	if backfill.Active(context.Background()) != nil {
		t.Error("Backfill Active must not report the running Import Job")
	}
	if err := b.svc.Cancel(context.Background(), "not-"+job.ID); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("cancel of an unknown id = %v", err)
	}
	b.release()
	waitImportDone(t, b.svc, job.ID)
}

func TestMediaLane_BackfillBlocksImport(t *testing.T) {
	b, backfill := mediaLane(t)
	gate := make(chan struct{})
	backfill.tasks[model.BackfillCode] = &mock.BackfillTask{Videos: videosNamed("v1"), ProcessFunc: func(context.Context, *model.Video) error {
		<-gate
		return nil
	}}
	bf, err := backfill.Start(context.Background(), model.BackfillCode, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	defer close(gate)

	_, err = b.svc.StartAsync(context.Background(), b.source, "user-1")
	var busy *JobLaneBusyError
	if !errors.As(err, &busy) || busy.RunningKind != JobKindBackfill {
		t.Fatalf("Import Start = %v, want a busy lane naming the Backfill Job", err)
	}
	if b.svc.GetActiveJob() != nil {
		t.Error("Import GetActiveJob must not report the running Backfill Job")
	}
	if err := b.svc.Cancel(context.Background(), bf.ID); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("Import Cancel of a Backfill Job = %v, want ErrNotFound", err)
	}
}
