package mock

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/steven/vaultflix/internal/model"
)

// PlaybackResolver is a hand-written fake for VideoService.ResolvePlayback.
type PlaybackResolver struct {
	Path string
	Mode model.PlayMode
	Err  error
}

func (m *PlaybackResolver) ResolvePlayback(ctx context.Context, videoID string) (string, model.PlayMode, error) {
	return m.Path, m.Mode, m.Err
}

// KeyframeLookup is a hand-written fake for KeyframeService.Lookup that records
// how often it was asked.
type KeyframeLookup struct {
	Segments []model.SegmentBoundary
	Err      error

	mu    sync.Mutex
	calls int
}

func (m *KeyframeLookup) Lookup(ctx context.Context, videoID, absPath string) ([]model.SegmentBoundary, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return m.Segments, m.Err
}

// Calls returns how many times Lookup ran.
func (m *KeyframeLookup) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// SegmentGenerator is a hand-written fake for streaming.SegmentGenerator. It
// writes a one-byte file and records each requested boundary in call order.
type SegmentGenerator struct {
	mu    sync.Mutex
	calls []model.SegmentBoundary
}

func (m *SegmentGenerator) Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error {
	m.mu.Lock()
	m.calls = append(m.calls, model.SegmentBoundary{Start: start, Duration: duration})
	m.mu.Unlock()
	if err := os.WriteFile(outPath, []byte{0x47}, 0o644); err != nil {
		return fmt.Errorf("mock: write segment: %w", err)
	}
	return nil
}

// Calls returns the boundaries Generate was asked for, in call order.
func (m *SegmentGenerator) Calls() []model.SegmentBoundary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]model.SegmentBoundary(nil), m.calls...)
}

// HLSStreamer is a hand-written fake for service.HLSService as seen by the
// HLS handler. Err, when set, is returned by both calls.
type HLSStreamer struct {
	ManifestBytes []byte
	SegmentPath   string
	Err           error

	LastSegmentName string
}

func (m *HLSStreamer) Manifest(ctx context.Context, videoID string) ([]byte, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.ManifestBytes, nil
}

func (m *HLSStreamer) Segment(ctx context.Context, videoID, name string) (string, error) {
	m.LastSegmentName = name
	if m.Err != nil {
		return "", m.Err
	}
	return m.SegmentPath, nil
}
