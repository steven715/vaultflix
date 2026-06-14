package service

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSegmentHasVideoBytes pins the 1024-byte threshold that distinguishes an
// empty ffmpeg segment (~260 bytes of bare MP4 container) from a real clip.
// This is the load-bearing predicate behind the input/output seek fallback in
// extractPreviewSegment; it must run in the default suite (not gated behind
// -tags=smoke) so a regression in the boundary is caught without ffmpeg.
func TestSegmentHasVideoBytes(t *testing.T) {
	dir := t.TempDir()

	write := func(name string, size int) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"empty file", write("empty.mp4", 0), false},
		{"bare container ~260 bytes", write("bare.mp4", 260), false},
		{"exactly 1024 bytes", write("boundary.mp4", 1024), false},
		{"just over threshold", write("over.mp4", 1025), true},
		{"real clip size", write("real.mp4", 50_000), true},
		{"nonexistent path", filepath.Join(dir, "missing.mp4"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := segmentHasVideoBytes(tt.path); got != tt.want {
				t.Errorf("segmentHasVideoBytes(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
