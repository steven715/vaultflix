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

func strPtr(s string) *string { return &s }

func TestLookupOrProbe_MissTriggersProbeAndReportsPreparing(t *testing.T) {
	repo := newFakeKeyframeRepo()
	s := NewKeyframeService(repo)
	probedPath := make(chan string, 1)
	s.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		probedPath <- absPath
		return []float64{0, 8}, 16, nil
	}

	_, err := s.LookupOrProbe(context.Background(), "v1", "/mnt/host/D/a.avi")
	if !errors.Is(err, model.ErrStreamPreparing) {
		t.Fatalf("err = %v, want ErrStreamPreparing", err)
	}
	select {
	case got := <-probedPath:
		if got != "/mnt/host/D/a.avi" {
			t.Errorf("probed %q, want /mnt/host/D/a.avi", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("probe was not triggered on a Keyframe Index miss")
	}
}

func TestLookupOrProbe_ReturnsStoredWithoutProbing(t *testing.T) {
	repo := newFakeKeyframeRepo()
	repo.stored["v1"] = &model.KeyframeIndex{
		VideoID:  "v1",
		Segments: []model.SegmentBoundary{{Start: 0, Duration: 8}},
	}
	s := NewKeyframeService(repo)
	s.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		t.Error("probe ran although the Keyframe Index exists")
		return nil, 0, nil
	}

	segs, err := s.LookupOrProbe(context.Background(), "v1", "/mnt/host/D/a.avi")
	if err != nil || len(segs) != 1 {
		t.Errorf("segs = %v, err = %v", segs, err)
	}
}

func TestLookupOrProbe_RepoErrorIsNotPreparing(t *testing.T) {
	repo := newFakeKeyframeRepo()
	repo.getErr = errors.New("db down")
	s := NewKeyframeService(repo)

	_, err := s.LookupOrProbe(context.Background(), "v1", "/mnt/host/D/a.avi")
	if err == nil || errors.Is(err, model.ErrStreamPreparing) {
		t.Errorf("err = %v, want a non-preparing error", err)
	}
}

func TestTriggerProbe_DedupesConcurrentTriggers(t *testing.T) {
	repo := newFakeKeyframeRepo()
	s := NewKeyframeService(repo)

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

	s.triggerProbe("v1", "/in.avi")
	<-probeStarted
	s.triggerProbe("v1", "/in.avi") // 探測進行中的重複觸發應被去重
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

func TestEnsureIndex_OnlyRemuxVideosGetAnIndex(t *testing.T) {
	tests := []struct {
		name      string
		video     model.Video
		wantIndex bool
	}{
		{"remux", model.Video{ID: "v1", OriginalFilename: "a.mkv", VideoCodec: "h264", AudioCodec: "aac"}, true},
		{"direct", model.Video{ID: "v1", OriginalFilename: "a.mp4", VideoCodec: "h264", AudioCodec: "aac"}, false},
		{"transcode", model.Video{ID: "v1", OriginalFilename: "a.wmv", VideoCodec: "wmv3", AudioCodec: "wmav2"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeKeyframeRepo()
			s := NewKeyframeService(repo)
			s.probe = func(context.Context, string) ([]float64, float64, error) { return []float64{0, 8}, 16, nil }

			if err := s.EnsureIndex(context.Background(), &tt.video, "/mnt/host/D/a"); err != nil {
				t.Fatalf("EnsureIndex: %v", err)
			}
			if _, got := repo.stored["v1"]; got != tt.wantIndex {
				t.Errorf("Keyframe Index stored = %v, want %v", got, tt.wantIndex)
			}
		})
	}
}

func TestEnsureIndex_ProbeFailure(t *testing.T) {
	s := NewKeyframeService(newFakeKeyframeRepo())
	s.probe = func(context.Context, string) ([]float64, float64, error) { return nil, 0, errors.New("boom") }
	v := model.Video{ID: "v1", OriginalFilename: "a.mkv", VideoCodec: "h264", AudioCodec: "aac"}

	if err := s.EnsureIndex(context.Background(), &v, "/mnt/host/D/a.mkv"); err == nil {
		t.Error("EnsureIndex succeeded although the probe failed")
	}
}

// probeCounter is a probe that fails until told otherwise, counting its runs.
type probeCounter struct {
	mu   sync.Mutex
	runs int
	fail bool
	done chan struct{}
}

func (p *probeCounter) probe(ctx context.Context, absPath string) ([]float64, float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.runs++
	defer func() { p.done <- struct{}{} }()
	if p.fail {
		return nil, 0, errors.New("ffprobe exit 1")
	}
	return []float64{0, 8}, 16, nil
}

func (p *probeCounter) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runs
}

// lookupUntilSettled calls LookupOrProbe once, then waits for the probe it
// triggered (if any) to finish.
func lookupUntilSettled(t *testing.T, s *KeyframeService, p *probeCounter) error {
	t.Helper()
	_, err := s.LookupOrProbe(context.Background(), "v1", "/mnt/host/D/a.mkv")
	if errors.Is(err, model.ErrStreamPreparing) {
		select {
		case <-p.done:
		case <-time.After(2 * time.Second):
			t.Fatal("triggered probe never finished")
		}
		// The probe's bookkeeping runs after it returns.
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			s.mu.Lock()
			_, busy := s.inflight["v1"]
			s.mu.Unlock()
			if !busy {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
	return err
}

// A failed probe is remembered: polling the manifest must not rerun ffprobe
// every few seconds, and the player must hear it failed, not "preparing".
func TestLookupOrProbe_RemembersAFailedProbe(t *testing.T) {
	s := NewKeyframeService(newFakeKeyframeRepo())
	p := &probeCounter{fail: true, done: make(chan struct{}, 4)}
	s.probe = p.probe

	if err := lookupUntilSettled(t, s, p); !errors.Is(err, model.ErrStreamPreparing) {
		t.Fatalf("first lookup = %v, want preparing", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.LookupOrProbe(context.Background(), "v1", "/mnt/host/D/a.mkv"); !errors.Is(err, model.ErrKeyframeIndexFailed) {
			t.Fatalf("lookup after failure = %v, want ErrKeyframeIndexFailed", err)
		}
	}
	if p.count() != 1 {
		t.Errorf("probe ran %d times, want 1", p.count())
	}
}

// A failure may be passing (a disk mounted late, a file replaced): after the
// cooldown, the next play probes again.
func TestLookupOrProbe_ProbesAgainAfterTheCooldown(t *testing.T) {
	s := NewKeyframeService(newFakeKeyframeRepo())
	p := &probeCounter{fail: true, done: make(chan struct{}, 4)}
	s.probe = p.probe
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := lookupUntilSettled(t, s, p); !errors.Is(err, model.ErrStreamPreparing) {
		t.Fatalf("first lookup = %v", err)
	}

	now = now.Add(probeFailureCooldown + time.Second)
	p.mu.Lock()
	p.fail = false
	p.mu.Unlock()

	if err := lookupUntilSettled(t, s, p); !errors.Is(err, model.ErrStreamPreparing) {
		t.Fatalf("lookup after cooldown = %v, want preparing (a new probe)", err)
	}
	if segs, err := s.LookupOrProbe(context.Background(), "v1", "/mnt/host/D/a.mkv"); err != nil || len(segs) == 0 {
		t.Errorf("after a successful probe: segs %v, err %v", segs, err)
	}
}

// Building the index on purpose (Import, Keyframe Backfill) forgets an
// earlier failure.
func TestEnsureIndex_ForgetsAnEarlierFailure(t *testing.T) {
	repo := newFakeKeyframeRepo()
	s := NewKeyframeService(repo)
	p := &probeCounter{fail: true, done: make(chan struct{}, 4)}
	s.probe = p.probe
	if err := lookupUntilSettled(t, s, p); !errors.Is(err, model.ErrStreamPreparing) {
		t.Fatalf("first lookup = %v", err)
	}

	p.mu.Lock()
	p.fail = false
	p.mu.Unlock()
	v := model.Video{ID: "v1", OriginalFilename: "a.mkv", VideoCodec: "h264", AudioCodec: "aac"}
	if err := s.EnsureIndex(context.Background(), &v, "/mnt/host/D/a.mkv"); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	<-p.done
	repo.mu.Lock()
	delete(repo.stored, "v1") // the index goes away again (e.g. deleted by hand)
	repo.mu.Unlock()

	if err := lookupUntilSettled(t, s, p); !errors.Is(err, model.ErrStreamPreparing) {
		t.Errorf("lookup = %v, want preparing: the old failure must be forgotten", err)
	}
}
