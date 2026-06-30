package streaming

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeTranscoder 不跑 ffmpeg：建立一個假的 playlist 檔，回傳一個可手動結束的 proc。
type fakeTranscoder struct{ started int }
type fakeProc struct{ done chan struct{} }

func (p *fakeProc) Done() <-chan struct{} { return p.done }
func (p *fakeProc) Stop() error {
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	return nil
}

func (f *fakeTranscoder) Start(_ context.Context, _, outDir string) (TranscodeProc, error) {
	f.started++
	_ = os.WriteFile(filepath.Join(outDir, PlaylistName), []byte("#EXTM3U\n"), 0o644)
	return &fakeProc{done: make(chan struct{})}, nil
}

func TestEnsureSession_ReusesSameSession(t *testing.T) {
	ft := &fakeTranscoder{}
	m := NewManager(ft, t.TempDir(), time.Minute)
	if _, err := m.EnsureSession(context.Background(), "v1", "u1", "/in.mkv"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, err := m.EnsureSession(context.Background(), "v1", "u1", "/in.mkv"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if ft.started != 1 {
		t.Errorf("transcoder started %d times, want 1 (session reused)", ft.started)
	}
}

func TestSweep_RemovesIdleSession(t *testing.T) {
	ft := &fakeTranscoder{}
	m := NewManager(ft, t.TempDir(), 10*time.Second)
	s, _ := m.EnsureSession(context.Background(), "v1", "u1", "/in.mkv")
	if _, err := os.Stat(s.Dir); err != nil {
		t.Fatalf("session dir should exist: %v", err)
	}
	// 模擬時間前進超過 idleTimeout
	m.Sweep(time.Now().Add(time.Minute))
	if _, ok := m.SessionDir("v1", "u1"); ok {
		t.Error("idle session should have been removed")
	}
	if _, err := os.Stat(s.Dir); !os.IsNotExist(err) {
		t.Error("session dir should have been deleted")
	}
}
