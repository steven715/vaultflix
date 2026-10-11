package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

// startBlockedImport starts an Import Job that stays running, on its one
// file, until release is called.
func startBlockedImport(t *testing.T) (svc *ImportService, job *model.ImportJob, release func()) {
	t.Helper()
	mount := t.TempDir()
	if err := os.WriteFile(filepath.Join(mount, "a.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	inFile, gate := make(chan struct{}), make(chan struct{})
	repo := &mock.VideoRepository{
		FindBySourceAndPathFunc: func(context.Context, string, string) (*model.Video, error) {
			close(inFile)
			<-gate
			return &model.Video{ID: "existing"}, nil
		},
	}
	svc = NewImportService(repo, NewMediaProcessor(&mock.MediaTool{}, &mock.MinIOClient{}), &mock.Notifier{})
	job, err := svc.StartAsync(context.Background(), &model.MediaSource{ID: "src-1", Label: "D", MountPath: mount}, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	<-inFile
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	return svc, job, release
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
