package mock

import (
	"context"
	"sync"

	"github.com/steven/vaultflix/internal/model"
)

// BackfillTask is a hand-written fake for the service-side backfillTask. It
// lists Videos and runs ProcessFunc (nil = success) for each, recording the
// processed ids in order.
type BackfillTask struct {
	Videos      []model.Video
	ListErr     error
	ProcessFunc func(ctx context.Context, v *model.Video) error

	mu        sync.Mutex
	processed []string
}

func (m *BackfillTask) List(ctx context.Context) ([]model.Video, error) {
	return m.Videos, m.ListErr
}

func (m *BackfillTask) ProcessOne(ctx context.Context, v *model.Video) error {
	m.mu.Lock()
	m.processed = append(m.processed, v.ID)
	m.mu.Unlock()
	if m.ProcessFunc == nil {
		return nil
	}
	return m.ProcessFunc(ctx, v)
}

// Processed returns the ids ProcessOne ran for, in call order.
func (m *BackfillTask) Processed() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.processed...)
}
