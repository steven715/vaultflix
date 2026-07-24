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
// gateInput/started/gate 讓測試能對特定 inputPath 的呼叫做確定性同步:
// 呼叫一開始(尚未寫檔前)關閉 started 通知測試已進入 in-flight 狀態,
// 接著阻塞在 gate 直到測試關閉它才繼續 —— 用來製造「其他影片的
// eviction 掃描發生時,這個呼叫仍在 in-flight」的可重現時序。
type fakeGenerator struct {
	calls atomic.Int64
	size  int
	delay time.Duration
	fail  bool

	gateInput string
	started   chan struct{}
	gate      chan struct{}
}

func (g *fakeGenerator) Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error {
	g.calls.Add(1)
	if g.gateInput != "" && inputPath == g.gateInput {
		if g.started != nil {
			close(g.started)
		}
		if g.gate != nil {
			select {
			case <-g.gate:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
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

func TestEnsureSegment_EvictionSkipsInflightVideo(t *testing.T) {
	started := make(chan struct{})
	gate := make(chan struct{})
	gen := &fakeGenerator{size: 100, gateInput: "/busy2.avi", started: started, gate: gate}
	c := newTestCache(t, gen, 150) // 容量只夠 1 段多一點

	ctx := context.Background()

	// "busy" 先完成一段(建立 videoCacheState,lastAccess 較舊)。
	if _, err := c.EnsureSegment(ctx, "busy", "/in.avi", 0, seg0); err != nil {
		t.Fatal(err)
	}

	// "busy" 的第二段用會被 gate 卡住的 inputPath,在背景跑,
	// 模擬「已有紀錄的影片正在產生新分段」。
	busyErrCh := make(chan error, 1)
	go func() {
		_, err := c.EnsureSegment(ctx, "busy", "/busy2.avi", 1, model.SegmentBoundary{Start: 100, Duration: 6})
		busyErrCh <- err
	}()
	<-started // 確定已進入 in-flight(inflight map 已登記)
	time.Sleep(2 * time.Millisecond)

	// 兩段快速的 "other" 寫入把總量推過上限,觸發 eviction 掃描。
	if _, err := c.EnsureSegment(ctx, "other", "/other.avi", 0, seg0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.EnsureSegment(ctx, "other", "/other.avi", 1, model.SegmentBoundary{Start: 6, Duration: 6}); err != nil {
		t.Fatal(err)
	}

	close(gate) // 放行 "busy" 的第二段

	if err := <-busyErrCh; err != nil {
		t.Fatalf("in-flight busy segment should survive eviction scan, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.cacheDir, sanitizeKey("busy"), SegmentName(1))); err != nil {
		t.Errorf("busy segment 1 file should exist: %v", err)
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

func TestSweep_SkipsInflightVideo(t *testing.T) {
	started := make(chan struct{})
	gate := make(chan struct{})
	gen := &fakeGenerator{size: 10, gateInput: "/busy2.avi", started: started, gate: gate}
	c := newTestCache(t, gen, 0) // idleTimeout = time.Minute

	ctx := context.Background()

	// "busy" 先完成一段(建立 videoCacheState,lastAccess 較舊)。
	if _, err := c.EnsureSegment(ctx, "busy", "/in.avi", 0, seg0); err != nil {
		t.Fatal(err)
	}

	// "busy" 的第二段用會被 gate 卡住的 inputPath,模擬「已有紀錄的
	// 影片正在產生新分段」時 Sweep 掃過。
	busyErrCh := make(chan error, 1)
	go func() {
		_, err := c.EnsureSegment(ctx, "busy", "/busy2.avi", 1, model.SegmentBoundary{Start: 100, Duration: 6})
		busyErrCh <- err
	}()
	<-started // 確定已進入 in-flight(inflight map 已登記)

	c.Sweep(time.Now().Add(2 * time.Minute))
	if _, err := os.Stat(filepath.Join(c.cacheDir, sanitizeKey("busy"))); err != nil {
		t.Errorf("busy video dir should survive sweep while in-flight: %v", err)
	}

	close(gate) // 放行 "busy" 的第二段

	if err := <-busyErrCh; err != nil {
		t.Fatalf("in-flight busy segment should complete without error, got: %v", err)
	}

	// 產生完成後,不再 in-flight,下一次 sweep 應正常清掉閒置目錄。
	c.Sweep(time.Now().Add(2 * time.Minute))
	if _, err := os.Stat(filepath.Join(c.cacheDir, sanitizeKey("busy"))); !os.IsNotExist(err) {
		t.Error("busy video dir should be swept after generation completes")
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
