package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func setupRecommendationRouter(recSvc *mock.RecommendationService) (*gin.Engine, *RecommendationHandler) {
	r := gin.New()
	h := NewRecommendationHandler(recSvc)
	r.GET("/api/recommendations/today", func(c *gin.Context) {
		c.Set("user_id", "user-1")
		h.GetToday(c)
	})
	r.POST("/api/recommendations", h.Create)
	r.PUT("/api/recommendations/:id", func(c *gin.Context) {
		c.Set("user_id", "user-1")
		h.UpdateSortOrder(c)
	})
	r.DELETE("/api/recommendations/:id", h.Delete)
	return r, h
}

func TestGetTodayRecommendations_Success(t *testing.T) {
	items := []model.RecommendationItem{
		{ID: "rec-1", VideoID: "vid-1", Title: "Video 1", IsFallback: false},
		{ID: "rec-2", VideoID: "vid-2", Title: "Video 2", IsFallback: false},
	}

	recSvc := &mock.RecommendationService{
		GetTodayFunc: func(ctx context.Context, userID string, date time.Time, fallbackCount int) ([]model.RecommendationItem, error) {
			if userID != "user-1" {
				t.Errorf("expected user-1, got %s", userID)
			}
			if fallbackCount != 5 {
				t.Errorf("expected default fallback_count 5, got %d", fallbackCount)
			}
			return items, nil
		},
	}

	r, _ := setupRecommendationRouter(recSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/recommendations/today", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []model.RecommendationItem `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Data))
	}
}

func TestGetTodayRecommendations_WithFallbackCount(t *testing.T) {
	var capturedCount int
	recSvc := &mock.RecommendationService{
		GetTodayFunc: func(ctx context.Context, userID string, date time.Time, fallbackCount int) ([]model.RecommendationItem, error) {
			capturedCount = fallbackCount
			return []model.RecommendationItem{}, nil
		},
	}

	r, _ := setupRecommendationRouter(recSvc)
	req := httptest.NewRequest(http.MethodGet, "/api/recommendations/today?fallback_count=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if capturedCount != 10 {
		t.Errorf("expected fallback_count 10, got %d", capturedCount)
	}
}

func TestPostRecommendation_Success(t *testing.T) {
	recSvc := &mock.RecommendationService{
		CreateFunc: func(ctx context.Context, videoID string, date time.Time, sortOrder int) (*model.DailyRecommendation, error) {
			return &model.DailyRecommendation{
				ID:            "rec-new",
				VideoID:       videoID,
				RecommendDate: date,
				SortOrder:     sortOrder,
			}, nil
		},
	}

	r, _ := setupRecommendationRouter(recSvc)
	body := `{"video_id":"vid-1","recommend_date":"2025-04-06","sort_order":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/recommendations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data model.DailyRecommendation `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Data.ID != "rec-new" {
		t.Errorf("expected id rec-new, got %s", resp.Data.ID)
	}
}

func TestPostRecommendation_Conflict(t *testing.T) {
	recSvc := &mock.RecommendationService{
		CreateFunc: func(ctx context.Context, videoID string, date time.Time, sortOrder int) (*model.DailyRecommendation, error) {
			return nil, model.ErrConflict
		},
	}

	r, _ := setupRecommendationRouter(recSvc)
	body := `{"video_id":"vid-1","recommend_date":"2025-04-06","sort_order":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/recommendations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error != "conflict" {
		t.Errorf("expected error 'conflict', got %s", resp.Error)
	}
}

func TestUpdateSortOrder_Success(t *testing.T) {
	var capturedID string
	var capturedOrder int
	recSvc := &mock.RecommendationService{
		UpdateSortOrderFunc: func(ctx context.Context, id string, sortOrder int) error {
			capturedID = id
			capturedOrder = sortOrder
			return nil
		},
	}

	r, _ := setupRecommendationRouter(recSvc)
	body := `{"sort_order": 3}`
	req := httptest.NewRequest(http.MethodPut, "/api/recommendations/rec-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if capturedID != "rec-1" {
		t.Errorf("expected id rec-1, got %s", capturedID)
	}
	if capturedOrder != 3 {
		t.Errorf("expected sort_order 3, got %d", capturedOrder)
	}

	var resp struct {
		Data struct {
			SortOrder int `json:"sort_order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Data.SortOrder != 3 {
		t.Errorf("expected response sort_order 3, got %d", resp.Data.SortOrder)
	}
}

func TestUpdateSortOrder_NotFound(t *testing.T) {
	recSvc := &mock.RecommendationService{
		UpdateSortOrderFunc: func(ctx context.Context, id string, sortOrder int) error {
			return model.ErrNotFound
		},
	}

	r, _ := setupRecommendationRouter(recSvc)
	body := `{"sort_order": 3}`
	req := httptest.NewRequest(http.MethodPut, "/api/recommendations/nonexistent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error != "not_found" {
		t.Errorf("expected error 'not_found', got %s", resp.Error)
	}
}

func TestUpdateSortOrder_BadRequest(t *testing.T) {
	recSvc := &mock.RecommendationService{}

	r, _ := setupRecommendationRouter(recSvc)
	body := `{"sort_order": 0}`
	req := httptest.NewRequest(http.MethodPut, "/api/recommendations/rec-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error != "bad_request" {
		t.Errorf("expected error 'bad_request', got %s", resp.Error)
	}
}

func TestDeleteRecommendation_Success(t *testing.T) {
	recSvc := &mock.RecommendationService{
		DeleteFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}

	r, _ := setupRecommendationRouter(recSvc)
	req := httptest.NewRequest(http.MethodDelete, "/api/recommendations/rec-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestDeleteRecommendation_NotFound(t *testing.T) {
	recSvc := &mock.RecommendationService{
		DeleteFunc: func(ctx context.Context, id string) error {
			return model.ErrNotFound
		},
	}

	r, _ := setupRecommendationRouter(recSvc)
	req := httptest.NewRequest(http.MethodDelete, "/api/recommendations/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error != "not_found" {
		t.Errorf("expected error 'not_found', got %s", resp.Error)
	}
}
