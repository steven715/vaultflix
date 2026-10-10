package mock

import (
	"context"
	"sync"

	"github.com/steven/vaultflix/internal/model"
)

// KeyframeLookup is a hand-written fake for KeyframeService.LookupOrProbe that records
// how often it was asked.
type KeyframeLookup struct {
	Segments []model.SegmentBoundary
	Err      error

	mu    sync.Mutex
	calls int
}

func (m *KeyframeLookup) LookupOrProbe(ctx context.Context, videoID, absPath string) ([]model.SegmentBoundary, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	return m.Segments, m.Err
}

// Calls returns how many times LookupOrProbe ran.
func (m *KeyframeLookup) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}
