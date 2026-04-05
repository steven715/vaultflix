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
	"github.com/steven/vaultflix/internal/service"
)

func setupHistoryRouter() (*gin.Engine, *mock.WatchHistoryRepository, *mock.VideoRepository) {
	historyRepo := &mock.WatchHistoryRepository{}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb/" + objectKey, nil
		},
	}

	historySvc := service.NewWatchHistoryService(historyRepo, videoRepo, minioSvc)
	h := NewHistoryHandler(historySvc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user")
		c.Next()
	})
	r.POST("/api/watch-history", h.SaveProgress)
	r.GET("/api/watch-history", h.List)

	return r, historyRepo, videoRepo
}

func TestPostWatchHistory_Success(t *testing.T) {
	r, historyRepo, videoRepo := setupHistoryRouter()

	videoRepo.GetByIDFunc = func(ctx context.Context, id string) (*model.Video, error) {
		return &model.Video{ID: "vid-1", DurationSeconds: 600}, nil
	}
	historyRepo.UpsertFunc = func(ctx context.Context, record *model.WatchHistory) error {
		return nil
	}

	body := `{"video_id":"vid-1","progress_seconds":120}`
	req := httptest.NewRequest(http.MethodPost, "/api/watch-history", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestPostWatchHistory_MissingVideoID(t *testing.T) {
	r, _, _ := setupHistoryRouter()

	body := `{"progress_seconds":120}`
	req := httptest.NewRequest(http.MethodPost, "/api/watch-history", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error != "bad_request" {
		t.Errorf("expected error 'bad_request', got %s", resp.Error)
	}
}

func TestGetWatchHistory_Pagination(t *testing.T) {
	r, historyRepo, _ := setupHistoryRouter()

	var capturedPage, capturedPageSize int
	historyRepo.ListByUserFunc = func(ctx context.Context, userID string, page, pageSize int) ([]model.WatchHistoryWithVideo, int64, error) {
		capturedPage = page
		capturedPageSize = pageSize
		return []model.WatchHistoryWithVideo{}, 0, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/watch-history?page=2&page_size=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if capturedPage != 2 {
		t.Errorf("expected page 2, got %d", capturedPage)
	}
	if capturedPageSize != 10 {
		t.Errorf("expected page_size 10, got %d", capturedPageSize)
	}

	var resp model.PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Page != 2 || resp.PageSize != 10 {
		t.Errorf("unexpected pagination: page=%d page_size=%d", resp.Page, resp.PageSize)
	}
}
