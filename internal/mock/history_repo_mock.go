package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

type WatchHistoryRepository struct {
	UpsertFunc            func(ctx context.Context, record *model.WatchHistory) error
	ListByUserFunc        func(ctx context.Context, userID string, page, pageSize int) ([]model.WatchHistoryWithVideo, int64, error)
	GetByUserAndVideoFunc func(ctx context.Context, userID, videoID string) (*model.WatchHistory, error)
	DeleteAllByUserFunc   func(ctx context.Context, userID string) (int64, error)
}

func (m *WatchHistoryRepository) Upsert(ctx context.Context, record *model.WatchHistory) error {
	if m.UpsertFunc == nil {
		return fmt.Errorf("mock: UpsertFunc not set")
	}
	return m.UpsertFunc(ctx, record)
}

func (m *WatchHistoryRepository) ListByUser(ctx context.Context, userID string, page, pageSize int) ([]model.WatchHistoryWithVideo, int64, error) {
	if m.ListByUserFunc == nil {
		return nil, 0, fmt.Errorf("mock: ListByUserFunc not set")
	}
	return m.ListByUserFunc(ctx, userID, page, pageSize)
}

func (m *WatchHistoryRepository) GetByUserAndVideo(ctx context.Context, userID, videoID string) (*model.WatchHistory, error) {
	if m.GetByUserAndVideoFunc == nil {
		return nil, fmt.Errorf("mock: GetByUserAndVideoFunc not set")
	}
	return m.GetByUserAndVideoFunc(ctx, userID, videoID)
}

func (m *WatchHistoryRepository) DeleteAllByUser(ctx context.Context, userID string) (int64, error) {
	if m.DeleteAllByUserFunc == nil {
		return 0, fmt.Errorf("mock: DeleteAllByUserFunc not set")
	}
	return m.DeleteAllByUserFunc(ctx, userID)
}
