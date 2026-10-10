package mock

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/steven/vaultflix/internal/model"
)

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
