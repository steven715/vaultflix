package service

import (
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestGetJob_Found(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	job := &model.ImportJob{
		ID:     "job-123",
		Status: "running",
	}
	svc.activeJobs.Store(job.ID, job)

	got, err := svc.GetJob("job-123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.ID != "job-123" {
		t.Errorf("expected job ID job-123, got %s", got.ID)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	_, err := svc.GetJob("nonexistent")
	if err != model.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetActiveJob_Running(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	job := &model.ImportJob{ID: "job-active", Status: "running"}
	svc.activeJobs.Store(job.ID, job)

	completed := &model.ImportJob{ID: "job-done", Status: "completed"}
	svc.activeJobs.Store(completed.ID, completed)

	got := svc.GetActiveJob()
	if got == nil {
		t.Fatal("expected active job, got nil")
	}
	if got.ID != "job-active" {
		t.Errorf("expected job-active, got %s", got.ID)
	}
}

func TestGetActiveJob_None(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	completed := &model.ImportJob{ID: "job-done", Status: "completed"}
	svc.activeJobs.Store(completed.ID, completed)

	got := svc.GetActiveJob()
	if got != nil {
		t.Errorf("expected nil, got job %s", got.ID)
	}
}

func TestStartAsync_Conflict(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	// Manually lock to simulate a running job
	svc.LockForTest()

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Test Source",
		MountPath: t.TempDir(),
	}

	_, err := svc.StartAsync(nil, source, "user-1")
	if err != model.ErrConflict {
		t.Errorf("expected ErrConflict, got %v", err)
	}

	svc.UnlockForTest()
}

func TestStartAsync_EmptyDirectory(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Empty Source",
		MountPath: t.TempDir(),
	}

	job, err := svc.StartAsync(nil, source, "user-1")
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
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Bad Source",
		MountPath: "/nonexistent/path/that/does/not/exist",
	}

	job, err := svc.StartAsync(nil, source, "user-1")
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
