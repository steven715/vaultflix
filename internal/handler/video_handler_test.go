package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	h := NewVideoHandler(nil, videoService, nil)
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
	if resp.Data.StreamURL != "/api/videos/vid-1/stream" {
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

func TestImportHandler_MissingSourceID(t *testing.T) {
	r := gin.New()
	h := NewVideoHandler(nil, nil, nil)
	r.POST("/api/videos/import", h.Import)

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/videos/import", body)
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

func TestImportHandler_SourceNotFound(t *testing.T) {
	mediaSourceRepo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return nil, model.ErrNotFound
		},
	}
	mediaSourceSvc := service.NewMediaSourceService(mediaSourceRepo, "/mnt/host/")

	r := gin.New()
	h := NewVideoHandler(nil, nil, mediaSourceSvc)
	r.POST("/api/videos/import", h.Import)

	body := strings.NewReader(`{"source_id": "nonexistent-id"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/videos/import", body)
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

func TestImportHandler_SourceDisabled(t *testing.T) {
	mediaSourceRepo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return &model.MediaSource{
				ID:        "src-1",
				Label:     "Test",
				MountPath: "/mnt/host/videos",
				Enabled:   false,
			}, nil
		},
	}
	mediaSourceSvc := service.NewMediaSourceService(mediaSourceRepo, "/mnt/host/")

	r := gin.New()
	h := NewVideoHandler(nil, nil, mediaSourceSvc)
	r.POST("/api/videos/import", h.Import)

	body := strings.NewReader(`{"source_id": "src-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/videos/import", body)
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
	if resp.Message != "media source is currently disabled" {
		t.Errorf("expected message 'media source is currently disabled', got %s", resp.Message)
	}
}

func setupStreamRouter(videoSvc *service.VideoService, mediaSourceSvc *service.MediaSourceService) (*gin.Engine, *VideoHandler) {
	r := gin.New()
	h := NewVideoHandler(nil, videoSvc, mediaSourceSvc)
	r.GET("/api/videos/:id/stream", h.Stream)
	return r, h
}

func TestStreamVideo(t *testing.T) {
	tmpDir := t.TempDir()
	videoContent := []byte("fake video content for testing")
	videoFile := tmpDir + "/test.mp4"
	if err := os.WriteFile(videoFile, videoContent, 0644); err != nil {
		t.Fatalf("failed to write temp video file: %v", err)
	}

	sourceID := "src-1"
	filePath := "test.mp4"

	tests := []struct {
		name           string
		videoID        string
		video          *model.Video
		videoErr       error
		source         *model.MediaSource
		sourceErr      error
		expectedStatus int
		expectedBody   string
		rangeHeader    string
	}{
		{
			name:    "success - new mode",
			videoID: "vid-1",
			video: &model.Video{
				ID:       "vid-1",
				MimeType: "video/mp4",
				SourceID: &sourceID,
				FilePath: &filePath,
			},
			source: &model.MediaSource{
				ID:        "src-1",
				MountPath: tmpDir,
				Enabled:   true,
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "fake video content for testing",
		},
		{
			name:    "range request",
			videoID: "vid-1",
			video: &model.Video{
				ID:       "vid-1",
				MimeType: "video/mp4",
				SourceID: &sourceID,
				FilePath: &filePath,
			},
			source: &model.MediaSource{
				ID:        "src-1",
				MountPath: tmpDir,
				Enabled:   true,
			},
			expectedStatus: http.StatusPartialContent,
			rangeHeader:    "bytes=0-9",
		},
		{
			name:           "video not found in DB",
			videoID:        "nonexistent",
			videoErr:       model.ErrNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:    "source disabled",
			videoID: "vid-1",
			video: &model.Video{
				ID:       "vid-1",
				MimeType: "video/mp4",
				SourceID: &sourceID,
				FilePath: &filePath,
			},
			source: &model.MediaSource{
				ID:        "src-1",
				MountPath: tmpDir,
				Enabled:   false,
			},
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:    "file not on disk",
			videoID: "vid-1",
			video: func() *model.Video {
				fp := "nonexistent.mp4"
				return &model.Video{
					ID:       "vid-1",
					MimeType: "video/mp4",
					SourceID: &sourceID,
					FilePath: &fp,
				}
			}(),
			source: &model.MediaSource{
				ID:        "src-1",
				MountPath: tmpDir,
				Enabled:   true,
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:    "path traversal attempt",
			videoID: "vid-1",
			video: func() *model.Video {
				fp := "../../etc/passwd"
				return &model.Video{
					ID:       "vid-1",
					MimeType: "video/mp4",
					SourceID: &sourceID,
					FilePath: &fp,
				}
			}(),
			source: &model.MediaSource{
				ID:        "src-1",
				MountPath: "/mnt/host/videos",
				Enabled:   true,
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:    "legacy mode - redirect to presigned URL",
			videoID: "vid-legacy",
			video: &model.Video{
				ID:             "vid-legacy",
				MinIOObjectKey: "videos/vid-legacy/test.mp4",
				MimeType:       "video/mp4",
			},
			expectedStatus: http.StatusTemporaryRedirect,
		},
		{
			name:    "source_id is nil and no minio key",
			videoID: "vid-broken",
			video: &model.Video{
				ID:       "vid-broken",
				MimeType: "video/mp4",
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			videoRepo := &mock.VideoRepository{
				GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
					if tt.videoErr != nil {
						return nil, tt.videoErr
					}
					return tt.video, nil
				},
			}
			tagRepo := &mock.TagRepository{
				GetByVideoIDFunc: func(ctx context.Context, videoID string) ([]model.Tag, error) {
					return []model.Tag{}, nil
				},
			}
			minioSvc := &mock.MinIOClient{
				GeneratePresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
					return "https://minio/legacy-stream", nil
				},
				GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
					return "", nil
				},
			}

			videoSvc := service.NewVideoService(videoRepo, tagRepo, minioSvc)

			var mediaSourceSvc *service.MediaSourceService
			if tt.source != nil || tt.sourceErr != nil {
				mediaSourceRepo := &mock.MediaSourceRepository{
					FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
						if tt.sourceErr != nil {
							return nil, tt.sourceErr
						}
						return tt.source, nil
					},
				}
				mediaSourceSvc = service.NewMediaSourceService(mediaSourceRepo, "/mnt/host/")
			}

			r, _ := setupStreamRouter(videoSvc, mediaSourceSvc)

			url := "/api/videos/" + tt.videoID + "/stream"
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.rangeHeader != "" {
				req.Header.Set("Range", tt.rangeHeader)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.expectedBody != "" && w.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, w.Body.String())
			}

			if tt.expectedStatus == http.StatusTemporaryRedirect {
				loc := w.Header().Get("Location")
				if loc != "https://minio/legacy-stream" {
					t.Errorf("expected redirect to presigned URL, got Location: %s", loc)
				}
			}
		})
	}
}
