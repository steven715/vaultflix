package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func setupTagRouter(tagRepo *mock.TagRepository, videoRepo *mock.VideoRepository) *gin.Engine {
	r := gin.New()
	h := NewTagHandler(tagRepo, videoRepo)
	r.GET("/api/tags", h.List)
	r.POST("/api/tags", h.Create)
	r.POST("/api/videos/:id/tags", h.AddVideoTag)
	r.DELETE("/api/videos/:id/tags/:tagId", h.RemoveVideoTag)
	return r
}

func doRequest(r *gin.Engine, method, url, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTagList_InvalidCategory(t *testing.T) {
	r := setupTagRouter(&mock.TagRepository{}, &mock.VideoRepository{})
	w := doRequest(r, http.MethodGet, "/api/tags?category=bogus", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTagList_Success(t *testing.T) {
	tagRepo := &mock.TagRepository{
		ListFunc: func(ctx context.Context, category string) ([]model.TagWithCount, error) {
			return []model.TagWithCount{}, nil
		},
	}
	r := setupTagRouter(tagRepo, &mock.VideoRepository{})
	w := doRequest(r, http.MethodGet, "/api/tags?category=genre", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestTagCreate_MissingName(t *testing.T) {
	r := setupTagRouter(&mock.TagRepository{}, &mock.VideoRepository{})
	w := doRequest(r, http.MethodPost, "/api/tags", `{"category":"genre"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTagCreate_InvalidCategory(t *testing.T) {
	r := setupTagRouter(&mock.TagRepository{}, &mock.VideoRepository{})
	w := doRequest(r, http.MethodPost, "/api/tags", `{"name":"x","category":"bogus"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestTagCreate_Conflict(t *testing.T) {
	tagRepo := &mock.TagRepository{
		CreateFunc: func(ctx context.Context, tag *model.Tag) error {
			return model.ErrAlreadyExists
		},
	}
	r := setupTagRouter(tagRepo, &mock.VideoRepository{})
	w := doRequest(r, http.MethodPost, "/api/tags", `{"name":"action","category":"genre"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestTagCreate_Success(t *testing.T) {
	tagRepo := &mock.TagRepository{
		CreateFunc: func(ctx context.Context, tag *model.Tag) error { return nil },
	}
	r := setupTagRouter(tagRepo, &mock.VideoRepository{})
	w := doRequest(r, http.MethodPost, "/api/tags", `{"name":"action"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestAddVideoTag_MissingTagID(t *testing.T) {
	r := setupTagRouter(&mock.TagRepository{}, &mock.VideoRepository{})
	w := doRequest(r, http.MethodPost, "/api/videos/v1/tags", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAddVideoTag_VideoNotFound(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return nil, model.ErrNotFound
		},
	}
	r := setupTagRouter(&mock.TagRepository{}, videoRepo)
	w := doRequest(r, http.MethodPost, "/api/videos/v1/tags", `{"tag_id":5}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (video), got %d", w.Code)
	}
}

func TestAddVideoTag_TagNotFound(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: id}, nil
		},
	}
	tagRepo := &mock.TagRepository{
		GetByIDFunc: func(ctx context.Context, id int) (*model.Tag, error) {
			return nil, model.ErrNotFound
		},
	}
	r := setupTagRouter(tagRepo, videoRepo)
	w := doRequest(r, http.MethodPost, "/api/videos/v1/tags", `{"tag_id":5}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (tag), got %d", w.Code)
	}
}

func TestAddVideoTag_ConflictIsIdempotent(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: id}, nil
		},
	}
	tagRepo := &mock.TagRepository{
		GetByIDFunc: func(ctx context.Context, id int) (*model.Tag, error) {
			return &model.Tag{ID: id, Name: "x", Category: "genre"}, nil
		},
		AddVideoTagFunc: func(ctx context.Context, videoID string, tagID int) error {
			return model.ErrConflict
		},
	}
	r := setupTagRouter(tagRepo, videoRepo)
	w := doRequest(r, http.MethodPost, "/api/videos/v1/tags", `{"tag_id":5}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (idempotent conflict), got %d", w.Code)
	}
}

func TestAddVideoTag_Success(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: id}, nil
		},
	}
	tagRepo := &mock.TagRepository{
		GetByIDFunc: func(ctx context.Context, id int) (*model.Tag, error) {
			return &model.Tag{ID: id, Name: "x", Category: "genre"}, nil
		},
		AddVideoTagFunc: func(ctx context.Context, videoID string, tagID int) error { return nil },
	}
	r := setupTagRouter(tagRepo, videoRepo)
	w := doRequest(r, http.MethodPost, "/api/videos/v1/tags", `{"tag_id":5}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestRemoveVideoTag_InvalidTagID(t *testing.T) {
	r := setupTagRouter(&mock.TagRepository{}, &mock.VideoRepository{})
	w := doRequest(r, http.MethodDelete, "/api/videos/v1/tags/abc", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRemoveVideoTag_NotFound(t *testing.T) {
	tagRepo := &mock.TagRepository{
		RemoveVideoTagFunc: func(ctx context.Context, videoID string, tagID int) error {
			return model.ErrNotFound
		},
	}
	r := setupTagRouter(tagRepo, &mock.VideoRepository{})
	w := doRequest(r, http.MethodDelete, "/api/videos/v1/tags/5", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRemoveVideoTag_Success(t *testing.T) {
	tagRepo := &mock.TagRepository{
		RemoveVideoTagFunc: func(ctx context.Context, videoID string, tagID int) error { return nil },
	}
	r := setupTagRouter(tagRepo, &mock.VideoRepository{})
	w := doRequest(r, http.MethodDelete, "/api/videos/v1/tags/5", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}
