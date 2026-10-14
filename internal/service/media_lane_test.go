package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

// laneSources is a Media Source lookup plus a file resolver, as
// MediaSourceService is both.
type laneSources struct {
	mock.MediaSources
	*mock.MediaFileResolver
}

func mediaLaneDeps(t *testing.T, repo *mock.VideoRepository, sources ...*model.MediaSource) MediaLaneDeps {
	t.Helper()
	return MediaLaneDeps{
		Videos:    repo,
		Sources:   laneSources{MediaSources: sources, MediaFileResolver: mock.ResolveUnder(t.TempDir())},
		Media:     NewMediaProcessor(&mock.MediaTool{}, &mock.MinIOClient{}),
		Keyframes: NewKeyframeService(newFakeKeyframeRepo()),
		Notifier:  &mock.Notifier{},
	}
}

// The Media Lane's Import and Backfill Jobs share one lane (ADR-0014): while
// a Backfill runs, an Import is refused, naming the Backfill.
func TestNewMediaLane_ImportAndBackfillShareTheLane(t *testing.T) {
	listing := make(chan struct{})
	gate := make(chan struct{})
	repo := &mock.VideoRepository{
		ListByEnrichmentStatusFunc: func(context.Context, string) ([]model.Video, error) {
			close(listing)
			<-gate
			return nil, nil
		},
	}
	source := &model.MediaSource{ID: "src-1", MountPath: t.TempDir(), Enabled: true}
	lane := NewMediaLane(mediaLaneDeps(t, repo, source))
	if _, err := lane.Backfill.Start(context.Background(), model.BackfillCode, "user-1"); err != nil {
		t.Fatalf("Backfill Start: %v", err)
	}
	defer close(gate)
	<-listing

	_, err := lane.Import.StartAsync(context.Background(), source.ID, "user-1")

	var busy *JobLaneBusyError
	if !errors.As(err, &busy) || busy.RunningKind != JobKindBackfill {
		t.Errorf("Import Start = %v, want a busy Media Lane naming the Backfill Job", err)
	}
}

func TestNewMediaLane_ServesEveryBackfillKind(t *testing.T) {
	lane := NewMediaLane(mediaLaneDeps(t, &mock.VideoRepository{}))

	for _, kind := range []model.BackfillKind{model.BackfillPreview, model.BackfillCodec, model.BackfillKeyframe, model.BackfillCode} {
		if lane.Backfill.tasks[kind] == nil {
			t.Errorf("Backfill kind %q is not served", kind)
		}
	}
}

// A missing dependency is a wiring mistake: it stops the server at startup,
// instead of degrading silently or crashing in the middle of a Job.
func TestNewImportService_MissingDependencyPanics(t *testing.T) {
	full := ImportServiceDeps{
		Videos: &mock.VideoRepository{}, Sources: mock.MediaSources{}, Media: NewMediaProcessor(&mock.MediaTool{}, &mock.MinIOClient{}),
		Notifier: &mock.Notifier{}, Lane: NewJobRunner(), Keyframes: &mock.KeyframeIndexer{},
	}
	tests := []struct {
		name string
		omit func(*ImportServiceDeps)
	}{
		{"no Lane", func(d *ImportServiceDeps) { d.Lane = nil }},
		{"no Keyframes", func(d *ImportServiceDeps) { d.Keyframes = nil }},
		{"no Videos", func(d *ImportServiceDeps) { d.Videos = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := full
			tt.omit(&d)
			defer func() {
				if recover() == nil {
					t.Error("NewImportService accepted a missing dependency")
				}
			}()
			NewImportService(d)
		})
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

	_, err = b.svc.StartAsync(context.Background(), b.source.ID, "user-1")
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
