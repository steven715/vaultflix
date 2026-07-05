package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

func setupAnalyticsRouter(repo *mock.AnalyticsRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewAnalyticsService(repo)
	h := NewAnalyticsHandler(svc)
	r := gin.New()
	r.GET("/api/admin/analytics", h.Get)
	return r
}

func emptyAnalyticsRepo(capture *int) *mock.AnalyticsRepository {
	return &mock.AnalyticsRepository{
		KPIsFunc: func(_ context.Context, days int) (int, int64, float64, int, error) {
			if capture != nil {
				*capture = days
			}
			return 0, 0, 0, 0, nil
		},
		DailyRawFunc: func(_ context.Context, _ int) (map[string]model.DailyRawRow, error) {
			return map[string]model.DailyRawRow{}, nil
		},
		TopVideosFunc: func(_ context.Context, _, _ int) ([]model.TopVideo, error) { return []model.TopVideo{}, nil },
		TopTagsFunc:   func(_ context.Context, _, _ int) ([]model.TopTag, error) { return []model.TopTag{}, nil },
	}
}

func TestAnalytics_DefaultDays(t *testing.T) {
	var gotDays int
	r := setupAnalyticsRouter(emptyAnalyticsRepo(&gotDays))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotDays != 30 {
		t.Fatalf("expected default days 30, got %d", gotDays)
	}
}

func TestAnalytics_ClampsDays(t *testing.T) {
	var gotDays int
	r := setupAnalyticsRouter(emptyAnalyticsRepo(&gotDays))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics?days=9999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if gotDays != 365 {
		t.Fatalf("expected clamped days 365, got %d", gotDays)
	}
}

func TestAnalytics_ReturnsSummaryShape(t *testing.T) {
	r := setupAnalyticsRouter(emptyAnalyticsRepo(nil))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics?days=7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Data model.AnalyticsSummary `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if resp.Data.RangeDays != 7 || len(resp.Data.DailyTrend) != 7 {
		t.Fatalf("unexpected summary: %+v", resp.Data)
	}
	if resp.Data.TopVideos == nil || resp.Data.TopTags == nil {
		t.Fatalf("expected non-nil slices for empty top lists")
	}
}
