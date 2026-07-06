package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

// PlaybackTelemetryRepository is a hand-written mock for
// repository.PlaybackTelemetryRepository. Set each Func field in tests.
type PlaybackTelemetryRepository struct {
	InsertFunc    func(ctx context.Context, in model.PlaybackTelemetryInput) error
	AggregateFunc func(ctx context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error)
}

func (m *PlaybackTelemetryRepository) Insert(ctx context.Context, in model.PlaybackTelemetryInput) error {
	if m.InsertFunc == nil {
		return fmt.Errorf("mock: InsertFunc not set")
	}
	return m.InsertFunc(ctx, in)
}

func (m *PlaybackTelemetryRepository) Aggregate(ctx context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error) {
	if m.AggregateFunc == nil {
		return nil, fmt.Errorf("mock: AggregateFunc not set")
	}
	return m.AggregateFunc(ctx, q)
}
