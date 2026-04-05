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

func setupFavoriteRouter() (*gin.Engine, *mock.FavoriteRepository) {
	favoriteRepo := &mock.FavoriteRepository{}
	minioSvc := &mock.MinIOClient{
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb/" + objectKey, nil
		},
	}

	favoriteSvc := service.NewFavoriteService(favoriteRepo, minioSvc)
	h := NewFavoriteHandler(favoriteSvc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user")
		c.Next()
	})
	r.GET("/api/favorites", h.List)
	r.POST("/api/favorites", h.Add)
	r.DELETE("/api/favorites/:videoId", h.Remove)

	return r, favoriteRepo
}

func TestPostFavorite_Success(t *testing.T) {
	r, favoriteRepo := setupFavoriteRouter()

	favoriteRepo.AddFunc = func(ctx context.Context, userID, videoID string) error {
		return nil
	}

	body := `{"video_id":"vid-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/favorites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestDeleteFavorite_NotFound(t *testing.T) {
	r, favoriteRepo := setupFavoriteRouter()

	favoriteRepo.RemoveFunc = func(ctx context.Context, userID, videoID string) error {
		return model.ErrNotFound
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/favorites/vid-1", nil)
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

func TestDeleteFavorite_Success(t *testing.T) {
	r, favoriteRepo := setupFavoriteRouter()

	favoriteRepo.RemoveFunc = func(ctx context.Context, userID, videoID string) error {
		return nil
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/favorites/vid-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d, body: %s", w.Code, w.Body.String())
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %s", w.Body.String())
	}
}

func TestGetFavorites_Pagination(t *testing.T) {
	r, favoriteRepo := setupFavoriteRouter()

	favoriteRepo.ListByUserFunc = func(ctx context.Context, userID string, page, pageSize int) ([]model.VideoSummary, int64, error) {
		return []model.VideoSummary{}, 0, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/favorites?page=3&page_size=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp model.PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Page != 3 || resp.PageSize != 5 {
		t.Errorf("unexpected pagination: page=%d page_size=%d", resp.Page, resp.PageSize)
	}
}
