package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

// blockedImport is an ImportService over a Media Source of n files, where
// every file waits in the duplicate check until release is called.
type blockedImport struct {
	svc     *ImportService
	source  *model.MediaSource
	inFile  chan struct{} // closed once the first file is being processed
	release func()
}

func newBlockedImport(t *testing.T, n int) *blockedImport {
	t.Helper()
	mount := t.TempDir()
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(mount, fmt.Sprintf("v%d.mp4", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b := &blockedImport{inFile: make(chan struct{})}
	gate := make(chan struct{})
	var entered, released sync.Once
	repo := &mock.VideoRepository{
		FindBySourceAndPathFunc: func(context.Context, string, string) (*model.Video, error) {
			entered.Do(func() { close(b.inFile) })
			<-gate
			return &model.Video{ID: "existing"}, nil
		},
	}
	b.svc = NewImportService(repo, NewMediaProcessor(&mock.MediaTool{}, &mock.MinIOClient{}), &mock.Notifier{})
	b.source = &model.MediaSource{ID: "src-1", Label: "D", MountPath: mount}
	b.release = func() { released.Do(func() { close(gate) }) }
	t.Cleanup(b.release)
	return b
}

// start launches the Import Job and waits until its first file is in progress.
func (b *blockedImport) start(t *testing.T) *model.ImportJob {
	t.Helper()
	job, err := b.svc.StartAsync(context.Background(), b.source, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	<-b.inFile
	return job
}

// startBlockedImport starts an Import Job that stays running, on its one
// file, until release is called.
func startBlockedImport(t *testing.T) (svc *ImportService, job *model.ImportJob, release func()) {
	t.Helper()
	b := newBlockedImport(t, 1)
	return b.svc, b.start(t), b.release
}

// waitImportDone polls until the Import Job jobID is no longer running.
func waitImportDone(t *testing.T, svc *ImportService, jobID string) *model.ImportJob {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got, err := svc.GetJob(jobID); err == nil && got.Status != "running" {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("import never finished")
	return nil
}

func TestGetJob_Found(t *testing.T) {
	svc, job, _ := startBlockedImport(t)

	got, err := svc.GetJob(job.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.ID != job.ID || got.Status != "running" || got.SourceID != "src-1" {
		t.Errorf("got %+v", got)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, NewMediaProcessor(&mock.MediaTool{}, &mock.MinIOClient{}), notifier)

	_, err := svc.GetJob("nonexistent")
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetActiveJob_Running(t *testing.T) {
	svc, job, _ := startBlockedImport(t)

	got := svc.GetActiveJob()
	if got == nil || got.ID != job.ID {
		t.Fatalf("GetActiveJob = %+v, want %s", got, job.ID)
	}
}

func TestGetActiveJob_None(t *testing.T) {
	svc, job, release := startBlockedImport(t)
	release()
	waitImportDone(t, svc, job.ID)

	if got := svc.GetActiveJob(); got != nil {
		t.Errorf("expected nil once the job finished, got job %s", got.ID)
	}
}

func TestStartAsync_Conflict(t *testing.T) {
	svc, _, _ := startBlockedImport(t)

	_, err := svc.StartAsync(context.Background(), &model.MediaSource{ID: "src-2", MountPath: t.TempDir()}, "user-1")
	if !errors.Is(err, model.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestStartAsync_EmptyDirectory(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, NewMediaProcessor(&mock.MediaTool{}, &mock.MinIOClient{}), notifier)

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Empty Source",
		MountPath: t.TempDir(),
	}

	job, err := svc.StartAsync(context.Background(), source, "user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	// Wait for background goroutine to complete
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job to complete")
		default:
			got, _ := svc.GetJob(job.ID)
			if got.Status != "running" {
				if got.Status != "completed" {
					t.Errorf("expected completed, got %s", got.Status)
				}
				if got.Total != 0 {
					t.Errorf("expected total 0, got %d", got.Total)
				}
				if got.FinishedAt == nil {
					t.Error("expected FinishedAt to be set")
				}

				msgs := notifier.GetMessages()
				if len(msgs) == 0 {
					t.Fatal("expected at least 1 notifier message")
				}
				lastMsg := msgs[len(msgs)-1]
				if lastMsg.Type != "import_complete" {
					t.Errorf("expected import_complete, got %s", lastMsg.Type)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestStartAsync_ScanError(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, NewMediaProcessor(&mock.MediaTool{}, &mock.MinIOClient{}), notifier)

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Bad Source",
		MountPath: "/nonexistent/path/that/does/not/exist",
	}

	job, err := svc.StartAsync(context.Background(), source, "user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job to complete")
		default:
			got, _ := svc.GetJob(job.ID)
			if got.Status != "running" {
				if got.Status != "failed" {
					t.Errorf("expected failed, got %s", got.Status)
				}
				if len(got.Errors) == 0 {
					t.Error("expected at least 1 error")
				}

				msgs := notifier.GetMessages()
				hasImportError := false
				for _, msg := range msgs {
					if msg.Type == "import_error" {
						hasImportError = true
					}
				}
				if !hasImportError {
					t.Error("expected import_error message")
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestCancel_StopsTheImportBetweenFiles(t *testing.T) {
	b := newBlockedImport(t, 2)
	job := b.start(t)

	if err := b.svc.Cancel(context.Background(), job.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	b.release()
	got := waitImportDone(t, b.svc, job.ID)

	if got.Status != "cancelled" || got.Processed != 1 {
		t.Errorf("job = %+v, want cancelled after its first file", got)
	}
}

func TestCancel_UnknownImportJob(t *testing.T) {
	svc := NewImportService(&mock.VideoRepository{}, NewMediaProcessor(&mock.MediaTool{}, &mock.MinIOClient{}), &mock.Notifier{})
	if err := svc.Cancel(context.Background(), "nope"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// mediaLane puts a blocked Import and a BackfillRunner on one Job Lane.
func mediaLane(t *testing.T) (*blockedImport, *BackfillRunner) {
	t.Helper()
	lane := NewJobRunner()
	b := newBlockedImport(t, 1)
	b.svc.UseJobRunner(lane)
	backfill := NewBackfillRunner(&mock.Notifier{}, map[model.BackfillKind]BackfillTask{model.BackfillCode: &mock.BackfillTask{}})
	backfill.UseJobRunner(lane)
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
