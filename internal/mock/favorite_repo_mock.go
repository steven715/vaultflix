package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

type FavoriteRepository struct {
	AddFunc        func(ctx context.Context, userID, videoID string) error
	RemoveFunc     func(ctx context.Context, userID, videoID string) error
	ExistsFunc     func(ctx context.Context, userID, videoID string) (bool, error)
	ListByUserFunc func(ctx context.Context, userID string, page, pageSize int) ([]model.VideoSummary, int64, error)
}

func (m *FavoriteRepository) Add(ctx context.Context, userID, videoID string) error {
	if m.AddFunc == nil {
		return fmt.Errorf("mock: AddFunc not set")
	}
	return m.AddFunc(ctx, userID, videoID)
}

func (m *FavoriteRepository) Remove(ctx context.Context, userID, videoID string) error {
	if m.RemoveFunc == nil {
		return fmt.Errorf("mock: RemoveFunc not set")
	}
	return m.RemoveFunc(ctx, userID, videoID)
}

func (m *FavoriteRepository) Exists(ctx context.Context, userID, videoID string) (bool, error) {
	if m.ExistsFunc == nil {
		return false, fmt.Errorf("mock: ExistsFunc not set")
	}
	return m.ExistsFunc(ctx, userID, videoID)
}

func (m *FavoriteRepository) ListByUser(ctx context.Context, userID string, page, pageSize int) ([]model.VideoSummary, int64, error) {
	if m.ListByUserFunc == nil {
		return nil, 0, fmt.Errorf("mock: ListByUserFunc not set")
	}
	return m.ListByUserFunc(ctx, userID, page, pageSize)
}
