package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

func setupMediaSourceRouter(svc *service.MediaSourceService) *gin.Engine {
	r := gin.New()
	h := NewMediaSourceHandler(svc)
	r.GET("/api/media-sources", h.List)
	r.POST("/api/media-sources", h.Create)
	r.PUT("/api/media-sources/:id", h.Update)
	r.DELETE("/api/media-sources/:id", h.Delete)
	return r
}

func setupTempMountForHandler(t *testing.T) (prefix string, validPath string) {
	t.Helper()
	base := t.TempDir()
	mountHost := filepath.Join(base, "mnt", "host")
	subDir := filepath.Join(mountHost, "Videos")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	return mountHost + string(filepath.Separator), subDir
}

func TestMediaSourceHandler_List_Empty(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		ListFunc: func(ctx context.Context) ([]model.MediaSource, error) {
			return []model.MediaSource{}, nil
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/media-sources", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp model.SuccessResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	sources, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected data to be array, got %T", resp.Data)
	}
	if len(sources) != 0 {
		t.Errorf("expected empty array, got %d items", len(sources))
	}
}

func TestMediaSourceHandler_List_WithVideoCount(t *testing.T) {
	now := time.Now()
	repo := &mock.MediaSourceRepository{
		ListFunc: func(ctx context.Context) ([]model.MediaSource, error) {
			return []model.MediaSource{
				{ID: "ms-1", Label: "Videos", MountPath: "/mnt/host/Videos", Enabled: true, CreatedAt: now, UpdatedAt: now, VideoCount: 42},
				{ID: "ms-2", Label: "Anime", MountPath: "/mnt/host/Anime", Enabled: false, CreatedAt: now, UpdatedAt: now, VideoCount: 0},
			}, nil
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/media-sources", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []struct {
			ID         string `json:"id"`
			VideoCount int    `json:"video_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(resp.Data))
	}
	if resp.Data[0].VideoCount != 42 {
		t.Errorf("expected video_count 42, got %d", resp.Data[0].VideoCount)
	}
	if resp.Data[1].VideoCount != 0 {
		t.Errorf("expected video_count 0, got %d", resp.Data[1].VideoCount)
	}
}

func TestMediaSourceHandler_Create_Success(t *testing.T) {
	prefix, validPath := setupTempMountForHandler(t)

	repo := &mock.MediaSourceRepository{
		CreateFunc: func(ctx context.Context, source *model.MediaSource) error {
			source.ID = "ms-1"
			source.Enabled = true
			source.CreatedAt = time.Now()
			source.UpdatedAt = time.Now()
			return nil
		},
	}
	svc := service.NewMediaSourceService(repo, prefix)
	r := setupMediaSourceRouter(svc)

	body := `{"label":"Videos","mount_path":"` + validPath + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/media-sources", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMediaSourceHandler_Create_MissingFields(t *testing.T) {
	svc := service.NewMediaSourceService(&mock.MediaSourceRepository{}, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	body := `{"label":"Videos"}`
	req := httptest.NewRequest(http.MethodPost, "/api/media-sources", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMediaSourceHandler_Create_DuplicatePath(t *testing.T) {
	prefix, validPath := setupTempMountForHandler(t)

	repo := &mock.MediaSourceRepository{
		CreateFunc: func(ctx context.Context, source *model.MediaSource) error {
			return model.ErrAlreadyExists
		},
	}
	svc := service.NewMediaSourceService(repo, prefix)
	r := setupMediaSourceRouter(svc)

	body := `{"label":"Dup","mount_path":"` + validPath + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/media-sources", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMediaSourceHandler_Update_Success(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return &model.MediaSource{ID: id, Label: "Old", Enabled: true}, nil
		},
		UpdateFunc: func(ctx context.Context, source *model.MediaSource) error {
			return nil
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	body := `{"label":"New","enabled":false}`
	req := httptest.NewRequest(http.MethodPut, "/api/media-sources/ms-1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMediaSourceHandler_Update_NotFound(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return nil, model.ErrNotFound
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	body := `{"label":"X","enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/media-sources/nope", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMediaSourceHandler_Delete_Success(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/media-sources/ms-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMediaSourceHandler_Delete_NotFound(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			return model.ErrNotFound
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/media-sources/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
