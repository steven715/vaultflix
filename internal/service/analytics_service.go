package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

type AnalyticsService struct {
	repo repository.AnalyticsRepository
}

func NewAnalyticsService(repo repository.AnalyticsRepository) *AnalyticsService {
	return &AnalyticsService{repo: repo}
}

// Summary assembles the windowed analytics payload with a zero-filled daily
// trend of exactly q.Days points (oldest first, ending today).
func (s *AnalyticsService) Summary(ctx context.Context, q model.AnalyticsQuery) (*model.AnalyticsSummary, error) {
	views, watchedSeconds, avgCompletion, activeUsers, err := s.repo.KPIs(ctx, q.Days)
	if err != nil {
		return nil, fmt.Errorf("failed to get kpis: %w", err)
	}
	dailyRaw, err := s.repo.DailyRaw(ctx, q.Days)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily trend: %w", err)
	}
	topVideos, err := s.repo.TopVideos(ctx, q.Days, q.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top videos: %w", err)
	}
	topTags, err := s.repo.TopTags(ctx, q.Days, q.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top tags: %w", err)
	}

	return &model.AnalyticsSummary{
		RangeDays:         q.Days,
		TotalViews:        views,
		TotalWatchHours:   round1(float64(watchedSeconds) / 3600.0),
		AvgCompletionRate: avgCompletion,
		ActiveUsers:       activeUsers,
		DailyTrend:        buildDailyTrend(q.Days, dailyRaw),
		TopVideos:         topVideos,
		TopTags:           topTags,
	}, nil
}

// buildDailyTrend produces days points ending today, merging present rows.
func buildDailyTrend(days int, raw map[string]model.DailyRawRow) []model.DailyPoint {
	trend := make([]model.DailyPoint, 0, days)
	today := time.Now()
	for i := days - 1; i >= 0; i-- {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		p := model.DailyPoint{Date: date}
		if row, ok := raw[date]; ok {
			p.Views = row.Views
			p.WatchHours = round1(float64(row.WatchedSeconds) / 3600.0)
		}
		trend = append(trend, p)
	}
	return trend
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
