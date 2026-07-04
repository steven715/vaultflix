package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

// AnalyticsRepository is a hand-written mock for repository.AnalyticsRepository.
// Set each Func field to override the corresponding method in tests.
type AnalyticsRepository struct {
	KPIsFunc      func(ctx context.Context, days int) (int, int64, float64, int, error)
	DailyRawFunc  func(ctx context.Context, days int) (map[string]model.DailyRawRow, error)
	TopVideosFunc func(ctx context.Context, days, limit int) ([]model.TopVideo, error)
	TopTagsFunc   func(ctx context.Context, days, limit int) ([]model.TopTag, error)
}

func (m *AnalyticsRepository) KPIs(ctx context.Context, days int) (int, int64, float64, int, error) {
	if m.KPIsFunc == nil {
		return 0, 0, 0, 0, fmt.Errorf("mock: KPIsFunc not set")
	}
	return m.KPIsFunc(ctx, days)
}

func (m *AnalyticsRepository) DailyRaw(ctx context.Context, days int) (map[string]model.DailyRawRow, error) {
	if m.DailyRawFunc == nil {
		return nil, fmt.Errorf("mock: DailyRawFunc not set")
	}
	return m.DailyRawFunc(ctx, days)
}

func (m *AnalyticsRepository) TopVideos(ctx context.Context, days, limit int) ([]model.TopVideo, error) {
	if m.TopVideosFunc == nil {
		return nil, fmt.Errorf("mock: TopVideosFunc not set")
	}
	return m.TopVideosFunc(ctx, days, limit)
}

func (m *AnalyticsRepository) TopTags(ctx context.Context, days, limit int) ([]model.TopTag, error) {
	if m.TopTagsFunc == nil {
		return nil, fmt.Errorf("mock: TopTagsFunc not set")
	}
	return m.TopTagsFunc(ctx, days, limit)
}
