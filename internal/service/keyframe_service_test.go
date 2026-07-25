package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

type fakeKeyframeRepo struct {
	mu      sync.Mutex
	stored  map[string]*model.KeyframeIndex
	getErr  error
	upserts int
}

func newFakeKeyframeRepo() *fakeKeyframeRepo {
	return &fakeKeyframeRepo{stored: make(map[string]*model.KeyframeIndex)}
}

func (f *fakeKeyframeRepo) Get(ctx context.Context, videoID string) (*model.KeyframeIndex, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, f.getErr
	}
	idx, ok := f.stored[videoID]
	if !ok {
		return nil, model.ErrNotFound
	}
	return idx, nil
}

func (f *fakeKeyframeRepo) Upsert(ctx context.Context, idx *model.KeyframeIndex) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts++
	f.stored[idx.VideoID] = idx
	return nil
}

type fakeKfVideoRepo struct{ videos []model.Video }

func (f *fakeKfVideoRepo) ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error) {
	return f.videos, nil
}

type fakeKfSourceRepo struct{ source *model.MediaSource }

func (f *fakeKfSourceRepo) FindByID(ctx context.Context, id string) (*model.MediaSource, error) {
	if f.source == nil {
		return nil, model.ErrNotFound
	}
	return f.source, nil
}

func strPtr(s string) *string { return &s }

func TestGetSegments_NotFoundPassthrough(t *testing.T) {
	s := NewKeyframeService(newFakeKeyframeRepo(), &fakeKfVideoRepo{}, &fakeKfSourceRepo{})
	_, err := s.GetSegments(context.Background(), "missing")
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGetSegments_ReturnsStored(t *testing.T) {
	repo := newFakeKeyframeRepo()
	repo.stored["v1"] = &model.KeyframeIndex{
		VideoID:  "v1",
		Segments: []model.SegmentBoundary{{Start: 0, Duration: 8}},
	}
	s := NewKeyframeService(repo, &fakeKfVideoRepo{}, &fakeKfSourceRepo{})
	segs, err := s.GetSegments(context.Background(), "v1")
	if err != nil || len(segs) != 1 {
		t.Errorf("segs = %v, err = %v", segs, err)
	}
}

func TestTriggerProbe_DedupesConcurrentTriggers(t *testing.T) {
	repo := newFakeKeyframeRepo()
	s := NewKeyframeService(repo, &fakeKfVideoRepo{}, &fakeKfSourceRepo{})

	probeStarted := make(chan struct{})
	probeRelease := make(chan struct{})
	var probeCalls int
	var mu sync.Mutex
	s.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		mu.Lock()
		probeCalls++
		mu.Unlock()
		close(probeStarted)
		<-probeRelease
		return []float64{0, 8}, 16, nil
	}

	s.TriggerProbe("v1", "/in.avi")
	<-probeStarted
	s.TriggerProbe("v1", "/in.avi") // 探測進行中的重複觸發應被去重
	close(probeRelease)

	deadline := time.After(2 * time.Second)
	for {
		repo.mu.Lock()
		done := repo.upserts > 0
		repo.mu.Unlock()
		if done {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for probe result upsert")
		case <-time.After(10 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if probeCalls != 1 {
		t.Errorf("probe calls = %d, want 1 (dedupe)", probeCalls)
	}
	if repo.upserts != 1 {
		t.Errorf("upserts = %d, want 1", repo.upserts)
	}
}

func TestRunBackfill_FiltersNonRemuxAndCounts(t *testing.T) {
	repo := newFakeKeyframeRepo()
	videoRepo := &fakeKfVideoRepo{videos: []model.Video{
		{ID: "remux1", OriginalFilename: "a.avi", VideoCodec: "h264", AudioCodec: "aac",
			SourceID: strPtr("s1"), FilePath: strPtr("a.avi")},
		{ID: "direct1", OriginalFilename: "b.mp4", VideoCodec: "h264", AudioCodec: "aac",
			SourceID: strPtr("s1"), FilePath: strPtr("b.mp4")}, // direct → 跳過
		{ID: "transcode1", OriginalFilename: "c.wmv", VideoCodec: "wmv3", AudioCodec: "wmav2",
			SourceID: strPtr("s1"), FilePath: strPtr("c.wmv")}, // transcode → 跳過
	}}
	s := NewKeyframeService(repo, videoRepo, &fakeKfSourceRepo{
		source: &model.MediaSource{ID: "s1", MountPath: "/mnt/host/D"},
	})
	s.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		return []float64{0, 8}, 16, nil
	}

	processed, failed, err := s.RunBackfill(context.Background())
	if err != nil {
		t.Fatalf("RunBackfill: %v", err)
	}
	if processed != 1 || failed != 0 {
		t.Errorf("processed=%d failed=%d, want 1/0", processed, failed)
	}
	if _, ok := repo.stored["remux1"]; !ok {
		t.Error("remux1 index not stored")
	}
}

func TestRunBackfill_ProbeFailureCountsFailed(t *testing.T) {
	repo := newFakeKeyframeRepo()
	videoRepo := &fakeKfVideoRepo{videos: []model.Video{
		{ID: "remux1", OriginalFilename: "a.avi", VideoCodec: "h264", AudioCodec: "aac",
			SourceID: strPtr("s1"), FilePath: strPtr("a.avi")},
	}}
	s := NewKeyframeService(repo, videoRepo, &fakeKfSourceRepo{
		source: &model.MediaSource{ID: "s1", MountPath: "/mnt/host/D"},
	})
	s.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		return nil, 0, errors.New("boom")
	}

	processed, failed, err := s.RunBackfill(context.Background())
	if err != nil {
		t.Fatalf("RunBackfill: %v", err)
	}
	if processed != 0 || failed != 1 {
		t.Errorf("processed=%d failed=%d, want 0/1", processed, failed)
	}
}
