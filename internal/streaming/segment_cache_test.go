package streaming

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

// fakeGenerator 寫入固定大小的假分段,並計數呼叫次數。
type fakeGenerator struct {
	calls atomic.Int64
	size  int
	delay time.Duration
	fail  bool
}

func (g *fakeGenerator) Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error {
	g.calls.Add(1)
	if g.delay > 0 {
		select {
		case <-time.After(g.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if g.fail {
		return os.ErrInvalid
	}
	return os.WriteFile(outPath, make([]byte, g.size), 0o644)
}

func newTestCache(t *testing.T, gen SegmentGenerator, maxBytes int64) *SegmentCache {
	t.Helper()
	c, err := NewSegmentCache(gen, t.TempDir(), time.Minute, maxBytes)
	if err != nil {
		t.Fatalf("NewSegmentCache: %v", err)
	}
	return c
}

var seg0 = model.SegmentBoundary{Start: 0, Duration: 6}

func TestEnsureSegment_CacheHit(t *testing.T) {
	gen := &fakeGenerator{size: 10}
	c := newTestCache(t, gen, 0)

	p1, err := c.EnsureSegment(context.Background(), "v1", "/in.avi", 0, seg0)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	p2, err := c.EnsureSegment(context.Background(), "v1", "/in.avi", 0, seg0)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if p1 != p2 {
		t.Errorf("paths differ: %s vs %s", p1, p2)
	}
	if got := gen.calls.Load(); got != 1 {
		t.Errorf("generator calls = %d, want 1 (cache hit)", got)
	}
}

func TestEnsureSegment_SingleflightConcurrent(t *testing.T) {
	gen := &fakeGenerator{size: 10, delay: 50 * time.Millisecond}
	c := newTestCache(t, gen, 0)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = c.EnsureSegment(context.Background(), "v1", "/in.avi", 3, model.SegmentBoundary{Start: 18, Duration: 6})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
	if got := gen.calls.Load(); got != 1 {
		t.Errorf("generator calls = %d, want 1 (singleflight)", got)
	}
}

func TestEnsureSegment_GeneratorFailure(t *testing.T) {
	gen := &fakeGenerator{fail: true}
	c := newTestCache(t, gen, 0)

	if _, err := c.EnsureSegment(context.Background(), "v1", "/in.avi", 0, seg0); err == nil {
		t.Fatal("expected error from failing generator")
	}
	// 失敗不得留下半成品分段檔
	entries, _ := os.ReadDir(filepath.Join(c.cacheDir, "v1"))
	for _, e := range entries {
		t.Errorf("unexpected leftover file: %s", e.Name())
	}
}

func TestEnsureSegment_LRUEviction(t *testing.T) {
	gen := &fakeGenerator{size: 100}
	c := newTestCache(t, gen, 250) // 容量只夠 2 段多一點

	ctx := context.Background()
	if _, err := c.EnsureSegment(ctx, "old", "/in.avi", 0, seg0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond) // 確保 lastAccess 有序
	if _, err := c.EnsureSegment(ctx, "newer", "/in.avi", 0, seg0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	// 第三段使總量 300 > 250 → 踢最久未存取的 "old" 整片目錄
	if _, err := c.EnsureSegment(ctx, "newer", "/in.avi", 1, model.SegmentBoundary{Start: 6, Duration: 6}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(c.cacheDir, "old")); !os.IsNotExist(err) {
		t.Error("old video dir should be evicted")
	}
	if _, err := os.Stat(filepath.Join(c.cacheDir, "newer")); err != nil {
		t.Error("newer video dir should survive eviction")
	}
}

func TestSweep_RemovesIdleVideos(t *testing.T) {
	gen := &fakeGenerator{size: 10}
	c := newTestCache(t, gen, 0) // idleTimeout = time.Minute

	if _, err := c.EnsureSegment(context.Background(), "v1", "/in.avi", 0, seg0); err != nil {
		t.Fatal(err)
	}
	c.Sweep(time.Now().Add(2 * time.Minute))
	if _, err := os.Stat(filepath.Join(c.cacheDir, "v1")); !os.IsNotExist(err) {
		t.Error("idle video dir should be swept")
	}
}

func TestNewSegmentCache_WipesLeftovers(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSegmentCache(&fakeGenerator{}, dir, time.Minute, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale")); !os.IsNotExist(err) {
		t.Error("leftover dir should be wiped on startup")
	}
}
