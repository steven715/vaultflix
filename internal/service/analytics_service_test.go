package service

import (
	"context"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestSummary_ZeroFillsDailyTrend(t *testing.T) {
	repo := &mock.AnalyticsRepository{
		KPIsFunc: func(_ context.Context, _ int) (int, int64, float64, int, error) {
			return 5, 7200, 0.5, 2, nil // 7200s = 2.0h
		},
		DailyRawFunc: func(_ context.Context, _ int) (map[string]model.DailyRawRow, error) {
			return map[string]model.DailyRawRow{}, nil // no days present
		},
		TopVideosFunc: func(_ context.Context, _, _ int) ([]model.TopVideo, error) { return []model.TopVideo{}, nil },
		TopTagsFunc:   func(_ context.Context, _, _ int) ([]model.TopTag, error) { return []model.TopTag{}, nil },
	}
	svc := NewAnalyticsService(repo)

	got, err := svc.Summary(context.Background(), model.AnalyticsQuery{Days: 7, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.DailyTrend) != 7 {
		t.Fatalf("expected 7 daily points, got %d", len(got.DailyTrend))
	}
	for _, p := range got.DailyTrend {
		if p.Views != 0 || p.WatchHours != 0 {
			t.Fatalf("expected zero-filled day, got %+v", p)
		}
	}
	if got.TotalWatchHours != 2.0 {
		t.Fatalf("expected 2.0 hours, got %v", got.TotalWatchHours)
	}
	if got.RangeDays != 7 {
		t.Fatalf("expected range_days 7, got %d", got.RangeDays)
	}
}

func TestSummary_MergesPresentDay(t *testing.T) {
	// The most recent day (today) carries data; verify it lands in the last slot.
	repo := &mock.AnalyticsRepository{
		KPIsFunc: func(_ context.Context, _ int) (int, int64, float64, int, error) { return 1, 1800, 0.9, 1, nil },
		DailyRawFunc: func(_ context.Context, _ int) (map[string]model.DailyRawRow, error) {
			today := todayDateString() // helper below, same tz as service
			return map[string]model.DailyRawRow{today: {Views: 3, WatchedSeconds: 1800}}, nil
		},
		TopVideosFunc: func(_ context.Context, _, _ int) ([]model.TopVideo, error) { return []model.TopVideo{}, nil },
		TopTagsFunc:   func(_ context.Context, _, _ int) ([]model.TopTag, error) { return []model.TopTag{}, nil },
	}
	svc := NewAnalyticsService(repo)
	got, err := svc.Summary(context.Background(), model.AnalyticsQuery{Days: 3, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := got.DailyTrend[len(got.DailyTrend)-1]
	if last.Views != 3 || last.WatchHours != 0.5 {
		t.Fatalf("expected today merged (3 views, 0.5h), got %+v", last)
	}
}

func TestSummary_RoundsTopVideoWatchHours(t *testing.T) {
	// The repository returns full-precision watch hours (e.g. derived from
	// raw seconds); Summary must round them to 1 decimal place, same as
	// TotalWatchHours and the daily trend.
	repo := &mock.AnalyticsRepository{
		KPIsFunc: func(_ context.Context, _ int) (int, int64, float64, int, error) { return 1, 600, 1.0, 1, nil },
		DailyRawFunc: func(_ context.Context, _ int) (map[string]model.DailyRawRow, error) {
			return map[string]model.DailyRawRow{}, nil
		},
		TopVideosFunc: func(_ context.Context, _, _ int) ([]model.TopVideo, error) {
			return []model.TopVideo{
				{VideoID: "v1", Title: "Video 1", WatchHours: 0.16666666666666666},
			}, nil
		},
		TopTagsFunc: func(_ context.Context, _, _ int) ([]model.TopTag, error) { return []model.TopTag{}, nil },
	}
	svc := NewAnalyticsService(repo)

	got, err := svc.Summary(context.Background(), model.AnalyticsQuery{Days: 7, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.TopVideos) != 1 {
		t.Fatalf("expected 1 top video, got %d", len(got.TopVideos))
	}
	if got.TopVideos[0].WatchHours != 0.2 {
		t.Fatalf("expected rounded watch_hours 0.2, got %v", got.TopVideos[0].WatchHours)
	}
}

func todayDateString() string {
	return time.Now().Format("2006-01-02")
}
