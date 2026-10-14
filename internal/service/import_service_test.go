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
	b.source = &model.MediaSource{ID: "src-1", Label: "D", MountPath: mount, Enabled: true}
	b.svc = newTestImportService(repo, &mock.Notifier{}, b.source)
	b.release = func() { released.Do(func() { close(gate) }) }
	t.Cleanup(b.release)
	return b
}

// start launches the Import Job and waits until its first file is in progress.
func (b *blockedImport) start(t *testing.T) *model.ImportJob {
	t.Helper()
	job, err := b.svc.StartAsync(context.Background(), b.source.ID, "user-1")
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
	svc := newTestImportService(&mock.VideoRepository{}, notifier)

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

	_, err := svc.StartAsync(context.Background(), "src-1", "user-1")
	if !errors.Is(err, model.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestStartAsync_EmptyDirectory(t *testing.T) {
	notifier := &mock.Notifier{}

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Empty Source",
		Enabled:   true,
		MountPath: t.TempDir(),
	}
	svc := newTestImportService(&mock.VideoRepository{}, notifier, source)

	job, err := svc.StartAsync(context.Background(), source.ID, "user-1")
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

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Bad Source",
		Enabled:   true,
		MountPath: "/nonexistent/path/that/does/not/exist",
	}
	svc := newTestImportService(&mock.VideoRepository{}, notifier, source)

	job, err := svc.StartAsync(context.Background(), source.ID, "user-1")
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
	svc := newTestImportService(&mock.VideoRepository{}, &mock.Notifier{})
	if err := svc.Cancel(context.Background(), "nope"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// newTestImportService is an ImportService on a Job Lane of its own that knows
// only sources and builds no Keyframe Index.
func newTestImportService(repo *mock.VideoRepository, notifier *mock.Notifier, sources ...*model.MediaSource) *ImportService {
	return NewImportService(ImportServiceDeps{
		Videos: repo, Sources: mock.MediaSources(sources), Media: NewMediaProcessor(&mock.MediaTool{}, &mock.MinIOClient{}), Notifier: notifier,
		Lane: NewJobRunner(), Keyframes: &mock.KeyframeIndexer{},
	})
}

// An Import reads its Media Source's directory, so a disabled or unknown
// Media Source starts no Import Job.
func TestStartAsync_UnusableMediaSource(t *testing.T) {
	tests := []struct {
		name    string
		sources mock.MediaSources
		want    error
	}{
		{"disabled", mock.MediaSources{{ID: "src-1", MountPath: t.TempDir(), Enabled: false}}, model.ErrMediaSourceDisabled},
		{"unknown", nil, model.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestImportService(&mock.VideoRepository{}, &mock.Notifier{}, tt.sources...)

			_, err := svc.StartAsync(context.Background(), "src-1", "user-1")

			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if svc.GetActiveJob() != nil {
				t.Error("an Import Job started")
			}
		})
	}
}
