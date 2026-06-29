package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/scraper"
	"github.com/steven/vaultflix/internal/websocket"
)

// fakeEnrichVideos returns n stub videos whose OriginalFilename contains an
// extractable AV code so EnrichVideo succeeds without real network calls.
func fakeEnrichVideos(n int) []model.Video {
	filenames := []string{
		"DASD-626.mp4",
		"SSIS-001.mp4",
		"ABW-123.mp4",
		"MIDE-456.mp4",
	}
	videos := make([]model.Video, n)
	for i := 0; i < n; i++ {
		fname := filenames[i%len(filenames)]
		videos[i] = model.Video{
			ID:               "vid-enrich-" + string(rune('A'+i)),
			OriginalFilename: fname,
			DurationSeconds:  90,
		}
	}
	return videos
}

// newTestEnrichmentService builds an EnrichmentService wired with controllable
// mocks suitable for batch tests. It overrides downloadImage so no real HTTP
// is attempted.
func newTestEnrichmentService(
	t *testing.T,
	videos []model.Video,
	scraperFn func(ctx context.Context, code string) (*model.EnrichedMetadata, error),
) *EnrichmentService {
	t.Helper()

	videoRepo := &mock.VideoRepository{
		ListByEnrichmentStatusFunc: func(_ context.Context, status string) ([]model.Video, error) {
			return videos, nil
		},
		GetByIDFunc: func(_ context.Context, id string) (*model.Video, error) {
			for _, v := range videos {
				if v.ID == id {
					return &v, nil
				}
			}
			return nil, model.ErrNotFound
		},
		SetEnrichmentStatusFunc: func(_ context.Context, id, status string) error {
			return nil
		},
	}

	sc := &mock.Scraper{
		SourceValue:      "javbus",
		ScrapeByCodeFunc: scraperFn,
	}

	sugRepo := &mock.SuggestionRepository{
		CreateFunc: func(_ context.Context, s *model.MetadataSuggestion) error {
			return nil
		},
	}

	svc := NewEnrichmentService(
		[]scraper.MetadataScraper{sc},
		videoRepo,
		&mock.ActressRepository{},
		sugRepo,
		&mock.TagRepository{},
		&mock.MinIOClient{},
		&mock.Notifier{},
	)
	// No real HTTP downloads needed in tests.
	svc.downloadImage = func(_ context.Context, url string) (string, error) {
		return "", errors.New("no real HTTP in tests")
	}
	return svc
}

// waitForEnrichJobStatus polls svc.ActiveJob() until the job status matches
// want or the 3-second deadline is exceeded.
func waitForEnrichJobStatus(t *testing.T, svc *EnrichmentService, jobID, want string) *model.EnrichJob {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for enrich job %s to reach status %q", jobID, want)
		default:
			job := svc.ActiveJob()
			if job != nil && job.ID == jobID && job.Status == want {
				return job
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func TestStartBatchAsync_RunsAllAndCompletes(t *testing.T) {
	videos := fakeEnrichVideos(2)

	svc := newTestEnrichmentService(t, videos, func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
		return &model.EnrichedMetadata{Code: code, Title: "Test Title"}, nil
	})

	job, err := svc.StartBatchAsync(context.Background(), "", "user-1")
	if err != nil {
		t.Fatalf("StartBatchAsync failed: %v", err)
	}
	if job.Status != "running" {
		t.Errorf("initial status = %q, want running", job.Status)
	}

	final := waitForEnrichJobStatus(t, svc, job.ID, "completed")
	if final.Total != 2 {
		t.Errorf("Total = %d, want 2", final.Total)
	}
	if final.Succeeded != 2 {
		t.Errorf("Succeeded = %d, want 2", final.Succeeded)
	}
	if final.Failed != 0 {
		t.Errorf("Failed = %d, want 0", final.Failed)
	}
	if final.Processed != 2 {
		t.Errorf("Processed = %d, want 2", final.Processed)
	}
}

func TestStartBatchAsync_ConflictWhenAlreadyRunning(t *testing.T) {
	// Control channel: the scraper blocks until we release it, so the first
	// batch stays in "running" while we attempt the second call.
	block := make(chan struct{})

	videos := fakeEnrichVideos(1)
	svc := newTestEnrichmentService(t, videos, func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
		<-block
		return &model.EnrichedMetadata{Code: code, Title: "T"}, nil
	})

	job, err := svc.StartBatchAsync(context.Background(), "", "user-1")
	if err != nil {
		t.Fatalf("first StartBatchAsync failed: %v", err)
	}

	// Second call while the first is still blocked inside the scraper.
	_, err2 := svc.StartBatchAsync(context.Background(), "", "user-1")
	if !errors.Is(err2, model.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err2)
	}

	// Unblock the first job so it can finish cleanly.
	close(block)
	waitForEnrichJobStatus(t, svc, job.ID, "completed")
}

func TestStartBatchAsync_DefaultStatusIsPending(t *testing.T) {
	var capturedStatus string
	videos := fakeEnrichVideos(1)

	videoRepo := &mock.VideoRepository{
		ListByEnrichmentStatusFunc: func(_ context.Context, status string) ([]model.Video, error) {
			capturedStatus = status
			return videos, nil
		},
		GetByIDFunc: func(_ context.Context, id string) (*model.Video, error) {
			return &videos[0], nil
		},
		SetEnrichmentStatusFunc: func(_ context.Context, id, status string) error { return nil },
	}
	sugRepo := &mock.SuggestionRepository{
		CreateFunc: func(_ context.Context, s *model.MetadataSuggestion) error { return nil },
	}
	sc := &mock.Scraper{
		SourceValue: "javbus",
		ScrapeByCodeFunc: func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
			return &model.EnrichedMetadata{Code: code, Title: "T"}, nil
		},
	}

	svc := NewEnrichmentService(
		[]scraper.MetadataScraper{sc},
		videoRepo,
		&mock.ActressRepository{},
		sugRepo,
		&mock.TagRepository{},
		&mock.MinIOClient{},
		&mock.Notifier{},
	)
	svc.downloadImage = func(_ context.Context, url string) (string, error) {
		return "", errors.New("no real HTTP in tests")
	}

	job, err := svc.StartBatchAsync(context.Background(), "", "user-1")
	if err != nil {
		t.Fatalf("StartBatchAsync failed: %v", err)
	}
	waitForEnrichJobStatus(t, svc, job.ID, "completed")

	if capturedStatus != model.EnrichmentPending {
		t.Errorf("status passed to ListByEnrichmentStatus = %q, want %q", capturedStatus, model.EnrichmentPending)
	}
}

func TestStartBatchAsync_PropagatesProgressAndCompleteMessages(t *testing.T) {
	videos := fakeEnrichVideos(2)

	notifier := &mock.Notifier{}
	videoRepo := &mock.VideoRepository{
		ListByEnrichmentStatusFunc: func(_ context.Context, status string) ([]model.Video, error) {
			return videos, nil
		},
		GetByIDFunc: func(_ context.Context, id string) (*model.Video, error) {
			for _, v := range videos {
				if v.ID == id {
					return &v, nil
				}
			}
			return nil, model.ErrNotFound
		},
		SetEnrichmentStatusFunc: func(_ context.Context, id, status string) error { return nil },
	}
	sugRepo := &mock.SuggestionRepository{
		CreateFunc: func(_ context.Context, s *model.MetadataSuggestion) error { return nil },
	}
	sc := &mock.Scraper{
		SourceValue: "javbus",
		ScrapeByCodeFunc: func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
			return &model.EnrichedMetadata{Code: code, Title: "T"}, nil
		},
	}
	svc := NewEnrichmentService(
		[]scraper.MetadataScraper{sc},
		videoRepo,
		&mock.ActressRepository{},
		sugRepo,
		&mock.TagRepository{},
		&mock.MinIOClient{},
		notifier,
	)
	svc.downloadImage = func(_ context.Context, url string) (string, error) {
		return "", errors.New("no real HTTP in tests")
	}

	job, err := svc.StartBatchAsync(context.Background(), "", "user-1")
	if err != nil {
		t.Fatalf("StartBatchAsync failed: %v", err)
	}
	waitForEnrichJobStatus(t, svc, job.ID, "completed")

	msgs := notifier.GetMessages()
	var progressCount, perVideoCompleteCount, batchCompleteCount int
	for _, m := range msgs {
		switch m.Type {
		case websocket.TypeEnrichProgress:
			progressCount++
		case websocket.TypeEnrichComplete:
			perVideoCompleteCount++
		case websocket.TypeEnrichBatchComplete:
			batchCompleteCount++
		}
	}
	if progressCount < 2 {
		t.Errorf("expected at least 2 enrich_progress messages (one per video), got %d", progressCount)
	}
	// EnrichVideo emits one enrich_complete per successful video.
	if perVideoCompleteCount < 2 {
		t.Errorf("expected at least 2 per-video enrich_complete messages, got %d", perVideoCompleteCount)
	}
	// The batch runner emits exactly one enrich_batch_complete at the end.
	if batchCompleteCount != 1 {
		t.Errorf("expected exactly 1 enrich_batch_complete message, got %d", batchCompleteCount)
	}
	// Verify the last message is the batch-level complete carrying job state.
	last := msgs[len(msgs)-1]
	if last.Type != websocket.TypeEnrichBatchComplete {
		t.Errorf("last message type = %q, want enrich_batch_complete", last.Type)
	}
}

func TestCancelBatch_StopsProcessing(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	videos := fakeEnrichVideos(2)

	svc := newTestEnrichmentService(t, videos, func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
		// Signal that the first call has entered the scraper, then block.
		once.Do(func() { close(started) })
		<-block
		return &model.EnrichedMetadata{Code: code, Title: "T"}, nil
	})

	job, err := svc.StartBatchAsync(context.Background(), "", "user-1")
	if err != nil {
		t.Fatalf("StartBatchAsync failed: %v", err)
	}

	// Wait deterministically for the worker to enter the blocking scraper.
	<-started
	if err := svc.CancelBatch(job.ID); err != nil {
		t.Fatalf("CancelBatch failed: %v", err)
	}
	close(block)

	final := waitForEnrichJobStatus(t, svc, job.ID, "cancelled")
	if final.Processed >= 2 {
		t.Errorf("expected <2 processed after cancel, got %d", final.Processed)
	}
}

func TestCancelBatch_UnknownJob(t *testing.T) {
	svc := newTestEnrichmentService(t, nil, nil)
	if err := svc.CancelBatch("nonexistent"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// Compile-time assertion that EnrichmentService exposes the batch API.
var _ interface {
	StartBatchAsync(ctx context.Context, status, userID string) (*model.EnrichJob, error)
	ActiveJob() *model.EnrichJob
	CancelBatch(jobID string) error
} = (*EnrichmentService)(nil)
