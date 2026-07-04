package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

type WatchSessionRepository struct {
	UpsertFunc func(ctx context.Context, in model.HeartbeatInput) error
}

func (m *WatchSessionRepository) Upsert(ctx context.Context, in model.HeartbeatInput) error {
	if m.UpsertFunc == nil {
		return fmt.Errorf("mock: UpsertFunc not set")
	}
	return m.UpsertFunc(ctx, in)
}
