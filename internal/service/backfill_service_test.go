package service

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/websocket"
)

// fakeVideos returns n stub videos with source_id + file_path filled in so
// processOneVideo can resolve a media source path. Preview generation itself
// is stubbed via the generatePreview field on BackfillService.
func fakeVideos(n int) []model.Video {
	videos := make([]model.Video, n)
	srcID := "src-1"
	for i := 0; i < n; i++ {
		fp := "movie.mp4"
		videos[i] = model.Video{
			ID:               "vid-" + string(rune('A'+i)),
			OriginalFilename: "movie.mp4",
			DurationSeconds:  100,
			SourceID:         &srcID,
			FilePath:         &fp,
		}
	}
	return videos
}

// stubPreviewGenerator returns a generator that creates a real (empty) temp
// file, so the caller's defer os.Remove succeeds. The optional fail predicate
// makes specific videos fail; when fail is nil, all succeed.
func stubPreviewGenerator(fail func(srcPath string) error) func(ctx context.Context, srcPath string, dur int) (string, error) {
	return func(ctx context.Context, srcPath string, dur int) (string, error) {
		if fail != nil {
			if err := fail(srcPath); err != nil {
				return "", err
			}
		}
		f, err := os.CreateTemp("", "backfill-stub-*.mp4")
		if err != nil {
			return "", err
		}
		path := f.Name()
		f.Close()
		return path, nil
	}
}

func newTestBackfillService(t *testing.T, videos []model.Video, gen func(ctx context.Context, srcPath string, dur int) (string, error)) (*BackfillService, *mock.Notifier, *atomic.Int32, *atomic.Int32) {
	t.Helper()

	uploadCount := &atomic.Int32{}
	updateCount := &atomic.Int32{}

	videoRepo := &mock.VideoRepository{
		ListMissingPreviewsFunc: func(ctx context.Context) ([]model.Video, error) {
			return videos, nil
		},
		UpdatePreviewKeyFunc: func(ctx context.Context, id string, key string) error {
			updateCount.Add(1)
			return nil
		},
	}
	sourceRepo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return &model.MediaSource{ID: id, Label: "test", MountPath: "/mnt/host/test"}, nil
		},
	}
	minioSvc := &mock.MinIOClient{
		UploadPreviewFunc: func(ctx context.Context, key, path string) error {
			uploadCount.Add(1)
			return nil
		},
	}
	notifier := &mock.Notifier{}

	svc := NewBackfillService(videoRepo, sourceRepo, minioSvc, notifier)
	svc.generatePreview = gen
	return svc, notifier, uploadCount, updateCount
}

// waitForJobStatus polls until job.Status != "running" or t fails the test.
func waitForJobStatus(t *testing.T, svc *BackfillService, jobID, want string) *model.BackfillJob {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for job %s status %q", jobID, want)
		default:
			job := svc.GetActiveJob()
			if job != nil && job.ID == jobID && job.Status == want {
				return job
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func TestBackfillService_StartAsync_Success(t *testing.T) {
	videos := fakeVideos(3)
	svc, notifier, uploads, updates := newTestBackfillService(t, videos, stubPreviewGenerator(nil))

	job, err := svc.StartAsync("user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	if job.Status != "running" {
		t.Errorf("expected status running, got %s", job.Status)
	}

	final := waitForJobStatus(t, svc, job.ID, "completed")
	if final.Total != 3 || final.Succeeded != 3 || final.Failed != 0 {
		t.Errorf("unexpected counts: total=%d succeeded=%d failed=%d", final.Total, final.Succeeded, final.Failed)
	}
	if uploads.Load() != 3 {
		t.Errorf("expected 3 uploads, got %d", uploads.Load())
	}
	if updates.Load() != 3 {
		t.Errorf("expected 3 UpdatePreviewKey calls, got %d", updates.Load())
	}

	// Last message must be backfill_complete carrying the final job.
	msgs := notifier.GetMessages()
	if len(msgs) == 0 {
		t.Fatal("expected at least one notification")
	}
	last := msgs[len(msgs)-1]
	if last.Type != websocket.TypeBackfillComplete {
		t.Errorf("last message expected backfill_complete, got %s", last.Type)
	}
}

func TestBackfillService_StartAsync_Conflict(t *testing.T) {
	// First job blocks on this channel inside its preview generator so we have
	// time to attempt a second start while it's still running.
	block := make(chan struct{})
	gen := func(ctx context.Context, srcPath string, dur int) (string, error) {
		<-block
		f, _ := os.CreateTemp("", "backfill-stub-*.mp4")
		f.Close()
		return f.Name(), nil
	}

	videos := fakeVideos(1)
	svc, _, _, _ := newTestBackfillService(t, videos, gen)

	job, err := svc.StartAsync("user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	// Second StartAsync while the first is still inside generatePreview.
	if _, err := svc.StartAsync("user-1"); !errors.Is(err, model.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}

	close(block)
	waitForJobStatus(t, svc, job.ID, "completed")
}

func TestBackfillService_StartAsync_FailureCounts(t *testing.T) {
	videos := fakeVideos(3)

	// Fail the second video; first and third succeed.
	gen := stubPreviewGenerator(func(srcPath string) error {
		// All videos use the same srcPath in fakeVideos, so we have to fail
		// based on call order. Use a counter via closure instead.
		return nil
	})

	var calls atomic.Int32
	failingGen := func(ctx context.Context, srcPath string, dur int) (string, error) {
		n := calls.Add(1)
		if n == 2 {
			return "", errors.New("boom")
		}
		return gen(ctx, srcPath, dur)
	}

	svc, _, _, _ := newTestBackfillService(t, videos, failingGen)

	job, err := svc.StartAsync("user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	final := waitForJobStatus(t, svc, job.ID, "completed")
	if final.Total != 3 || final.Succeeded != 2 || final.Failed != 1 {
		t.Errorf("unexpected counts: total=%d succeeded=%d failed=%d", final.Total, final.Succeeded, final.Failed)
	}
	if len(final.Errors) != 1 || final.Errors[0].Error == "" {
		t.Errorf("expected one populated error, got %+v", final.Errors)
	}
}

func TestBackfillService_Cancel_StopsAfterCurrentVideo(t *testing.T) {
	videos := fakeVideos(5)

	// Each video: signal that we've started, then block until released, then return.
	started := make(chan int, 10)
	release := make(chan struct{}, 10)
	var calls atomic.Int32

	gen := func(ctx context.Context, srcPath string, dur int) (string, error) {
		n := calls.Add(1)
		started <- int(n)
		<-release
		f, _ := os.CreateTemp("", "backfill-stub-*.mp4")
		f.Close()
		return f.Name(), nil
	}

	svc, _, _, _ := newTestBackfillService(t, videos, gen)

	job, err := svc.StartAsync("user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	// Wait for the first video to enter generation, then cancel.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first video did not start in time")
	}
	if err := svc.Cancel(job.ID); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	// Release the in-flight first video; it must complete normally.
	release <- struct{}{}

	// The worker should now exit the loop without starting video 2+.
	final := waitForJobStatus(t, svc, job.ID, "cancelled")
	if final.Processed != 1 {
		t.Errorf("expected exactly 1 processed video, got %d", final.Processed)
	}
	if final.Succeeded != 1 {
		t.Errorf("expected first video to count as succeeded, got %d", final.Succeeded)
	}

	// Make sure no further calls happened.
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 generator call, got %d", calls.Load())
	}
}

func TestBackfillService_Cancel_UnknownJob(t *testing.T) {
	svc, _, _, _ := newTestBackfillService(t, nil, stubPreviewGenerator(nil))
	if err := svc.Cancel("nonexistent"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestBackfillService_StartAsync_PropagatesProgressMessages(t *testing.T) {
	videos := fakeVideos(2)
	svc, notifier, _, _ := newTestBackfillService(t, videos, stubPreviewGenerator(nil))

	job, err := svc.StartAsync("user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}
	waitForJobStatus(t, svc, job.ID, "completed")

	var progressCount, completeCount int
	for _, m := range notifier.GetMessages() {
		switch m.Type {
		case websocket.TypeBackfillProgress:
			progressCount++
		case websocket.TypeBackfillComplete:
			completeCount++
		}
	}
	// Each video produces one "processing" + one "success" progress message.
	if progressCount != 4 {
		t.Errorf("expected 4 progress messages, got %d", progressCount)
	}
	if completeCount != 1 {
		t.Errorf("expected exactly 1 complete message, got %d", completeCount)
	}
}

// Compile-time assertion that BackfillService satisfies its expected shape.
var _ interface {
	StartAsync(string) (*model.BackfillJob, error)
	GetActiveJob() *model.BackfillJob
	Cancel(string) error
} = (*BackfillService)(nil)

// silence unused import in test helpers if structure changes.
var _ = sync.Mutex{}
