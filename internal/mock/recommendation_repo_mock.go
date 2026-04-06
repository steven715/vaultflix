package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

type RecommendationRepository struct {
	ListByDateFunc         func(ctx context.Context, date time.Time) ([]model.RecommendationWithVideo, error)
	CreateFunc             func(ctx context.Context, rec *model.DailyRecommendation) error
	UpdateSortOrderFunc    func(ctx context.Context, id string, sortOrder int) error
	DeleteFunc             func(ctx context.Context, id string) error
	GetRandomUnwatchedFunc func(ctx context.Context, userID string, limit int) ([]model.Video, error)
}

func (m *RecommendationRepository) ListByDate(ctx context.Context, date time.Time) ([]model.RecommendationWithVideo, error) {
	if m.ListByDateFunc == nil {
		return nil, fmt.Errorf("mock: ListByDateFunc not set")
	}
	return m.ListByDateFunc(ctx, date)
}

func (m *RecommendationRepository) Create(ctx context.Context, rec *model.DailyRecommendation) error {
	if m.CreateFunc == nil {
		return fmt.Errorf("mock: CreateFunc not set")
	}
	return m.CreateFunc(ctx, rec)
}

func (m *RecommendationRepository) UpdateSortOrder(ctx context.Context, id string, sortOrder int) error {
	if m.UpdateSortOrderFunc == nil {
		return fmt.Errorf("mock: UpdateSortOrderFunc not set")
	}
	return m.UpdateSortOrderFunc(ctx, id, sortOrder)
}

func (m *RecommendationRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc == nil {
		return fmt.Errorf("mock: DeleteFunc not set")
	}
	return m.DeleteFunc(ctx, id)
}

func (m *RecommendationRepository) GetRandomUnwatched(ctx context.Context, userID string, limit int) ([]model.Video, error) {
	if m.GetRandomUnwatchedFunc == nil {
		return nil, fmt.Errorf("mock: GetRandomUnwatchedFunc not set")
	}
	return m.GetRandomUnwatchedFunc(ctx, userID, limit)
}
