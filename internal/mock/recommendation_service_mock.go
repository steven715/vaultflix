package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

type RecommendationService struct {
	GetTodayFunc   func(ctx context.Context, userID string, date time.Time, fallbackCount int) ([]model.RecommendationItem, error)
	ListByDateFunc func(ctx context.Context, date time.Time) ([]model.RecommendationItem, error)
	CreateFunc          func(ctx context.Context, videoID string, date time.Time, sortOrder int) (*model.DailyRecommendation, error)
	UpdateSortOrderFunc func(ctx context.Context, id string, sortOrder int) error
	DeleteFunc          func(ctx context.Context, id string) error
}

func (m *RecommendationService) GetToday(ctx context.Context, userID string, date time.Time, fallbackCount int) ([]model.RecommendationItem, error) {
	if m.GetTodayFunc == nil {
		return nil, fmt.Errorf("mock: GetTodayFunc not set")
	}
	return m.GetTodayFunc(ctx, userID, date, fallbackCount)
}

func (m *RecommendationService) ListByDate(ctx context.Context, date time.Time) ([]model.RecommendationItem, error) {
	if m.ListByDateFunc == nil {
		return nil, fmt.Errorf("mock: ListByDateFunc not set")
	}
	return m.ListByDateFunc(ctx, date)
}

func (m *RecommendationService) Create(ctx context.Context, videoID string, date time.Time, sortOrder int) (*model.DailyRecommendation, error) {
	if m.CreateFunc == nil {
		return nil, fmt.Errorf("mock: CreateFunc not set")
	}
	return m.CreateFunc(ctx, videoID, date, sortOrder)
}

func (m *RecommendationService) UpdateSortOrder(ctx context.Context, id string, sortOrder int) error {
	if m.UpdateSortOrderFunc == nil {
		return fmt.Errorf("mock: UpdateSortOrderFunc not set")
	}
	return m.UpdateSortOrderFunc(ctx, id, sortOrder)
}

func (m *RecommendationService) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc == nil {
		return fmt.Errorf("mock: DeleteFunc not set")
	}
	return m.DeleteFunc(ctx, id)
}
