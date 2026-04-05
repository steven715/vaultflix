package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupVideoRouter(videoService *service.VideoService) (*gin.Engine, *VideoHandler) {
	r := gin.New()
	h := NewVideoHandler(nil, videoService)
	r.GET("/api/videos", h.List)
	r.GET("/api/videos/:id", h.GetByID)
	r.DELETE("/api/videos/:id", h.Delete)
	return r, h
}

func TestListVideos_DefaultPagination(t *testing.T) {
	var capturedFilter model.VideoFilter
	videoRepo := &mock.VideoRepository{
		ListFunc: func(ctx context.Context, filter model.VideoFilter) ([]model.Video, int64, error) {
			capturedFilter = filter
			return []model.Video{}, 0, nil
		},
	}
	tagRepo := &mock.TagRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := service.NewVideoService(videoRepo, tagRepo, minioSvc)
	r, _ := setupVideoRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/videos", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if capturedFilter.Page != 1 {
		t.Errorf("expected default page 1, got %d", capturedFilter.Page)
	}
	if capturedFilter.PageSize != 20 {
		t.Errorf("expected default page_size 20, got %d", capturedFilter.PageSize)
	}
	if capturedFilter.SortBy != "created_at" {
		t.Errorf("expected default sort_by created_at, got %s", capturedFilter.SortBy)
	}
	if capturedFilter.SortOrder != "desc" {
		t.Errorf("expected default sort_order desc, got %s", capturedFilter.SortOrder)
	}

	var resp model.PaginatedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Page != 1 || resp.PageSize != 20 {
		t.Errorf("unexpected response pagination: page=%d page_size=%d", resp.Page, resp.PageSize)
	}
}

func TestListVideos_InvalidPageSize(t *testing.T) {
	svc := service.NewVideoService(&mock.VideoRepository{}, &mock.TagRepository{}, &mock.MinIOClient{})
	r, _ := setupVideoRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/videos?page_size=999", nil)
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

func TestListVideos_InvalidSortBy(t *testing.T) {
	svc := service.NewVideoService(&mock.VideoRepository{}, &mock.TagRepository{}, &mock.MinIOClient{})
	r, _ := setupVideoRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/videos?sort_by=xxx", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetVideo_Success(t *testing.T) {
	video := &model.Video{
		ID:             "vid-1",
		Title:          "Test Video",
		MinIOObjectKey: "videos/vid-1/test.mp4",
		ThumbnailKey:   "thumbnails/vid-1.jpg",
	}
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return video, nil
		},
	}
	tagRepo := &mock.TagRepository{
		GetByVideoIDFunc: func(ctx context.Context, videoID string) ([]model.Tag, error) {
			return []model.Tag{{ID: 1, Name: "action", Category: "genre"}}, nil
		},
	}
	minioSvc := &mock.MinIOClient{
		GeneratePresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/stream", nil
		},
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb", nil
		},
	}

	svc := service.NewVideoService(videoRepo, tagRepo, minioSvc)
	r, _ := setupVideoRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/vid-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data model.VideoDetail `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Data.ID != "vid-1" {
		t.Errorf("expected id vid-1, got %s", resp.Data.ID)
	}
	if resp.Data.StreamURL != "https://minio/stream" {
		t.Errorf("expected stream url, got %s", resp.Data.StreamURL)
	}
	if len(resp.Data.Tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(resp.Data.Tags))
	}
}

func TestGetVideo_NotFound(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return nil, model.ErrNotFound
		},
	}
	tagRepo := &mock.TagRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := service.NewVideoService(videoRepo, tagRepo, minioSvc)
	r, _ := setupVideoRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error != "not_found" {
		t.Errorf("expected error 'not_found', got %s", resp.Error)
	}
}

func TestDeleteVideo_Success(t *testing.T) {
	video := &model.Video{
		ID:             "vid-1",
		MinIOObjectKey: "videos/vid-1/test.mp4",
		ThumbnailKey:   "thumbnails/vid-1.jpg",
	}
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return video, nil
		},
		DeleteFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}
	tagRepo := &mock.TagRepository{}
	minioSvc := &mock.MinIOClient{
		DeleteVideoFunc: func(ctx context.Context, objectKey string) error {
			return nil
		},
		DeleteThumbnailFunc: func(ctx context.Context, objectKey string) error {
			return nil
		},
	}

	svc := service.NewVideoService(videoRepo, tagRepo, minioSvc)
	r, _ := setupVideoRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d, body: %s", w.Code, w.Body.String())
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %s", w.Body.String())
	}
}

func TestGetVideo_InvalidExpiry(t *testing.T) {
	svc := service.NewVideoService(&mock.VideoRepository{}, &mock.TagRepository{}, &mock.MinIOClient{})
	r, _ := setupVideoRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/vid-1?url_expiry_minutes=9999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListVideos_InvalidTagIDs(t *testing.T) {
	svc := service.NewVideoService(&mock.VideoRepository{}, &mock.TagRepository{}, &mock.MinIOClient{})
	r, _ := setupVideoRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/videos?tag_ids=abc,def", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
