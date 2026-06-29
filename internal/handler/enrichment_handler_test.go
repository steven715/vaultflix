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

// buildEnrichmentRouter wires up a gin router with the EnrichmentHandler.
// videoRepo, actressRepo, suggestionRepo, tagRepo may be nil if the test
// does not exercise paths that call those repos.
func buildEnrichmentRouter(
	videoRepo *mock.VideoRepository,
	actressRepo *mock.ActressRepository,
	suggestionRepo *mock.SuggestionRepository,
	tagRepo *mock.TagRepository,
) (*gin.Engine, *service.EnrichmentService) {
	if videoRepo == nil {
		videoRepo = &mock.VideoRepository{}
	}
	if actressRepo == nil {
		actressRepo = &mock.ActressRepository{}
	}
	if suggestionRepo == nil {
		suggestionRepo = &mock.SuggestionRepository{}
	}
	if tagRepo == nil {
		tagRepo = &mock.TagRepository{}
	}

	svc := service.NewEnrichmentService(
		nil,
		videoRepo,
		actressRepo,
		suggestionRepo,
		tagRepo,
		&mock.MinIOClient{},
		&mock.Notifier{},
	)
	h := NewEnrichmentHandler(svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	})
	r.POST("/videos/:id/enrich", h.EnrichVideo)
	r.GET("/videos/:id/suggestions", h.ListSuggestions)
	r.POST("/videos/:id/suggestions/:sid/accept", h.AcceptSuggestion)
	r.DELETE("/videos/:id/suggestions/:sid", h.RejectSuggestion)
	r.POST("/enrich-jobs", h.StartBatch)
	r.GET("/enrich-jobs/active", h.ActiveJob)
	r.DELETE("/enrich-jobs/:jid", h.CancelBatch)

	return r, svc
}

// --- AcceptSuggestion tests ---

func TestEnrichmentHandler_AcceptSuggestion_NotFound(t *testing.T) {
	suggestionRepo := &mock.SuggestionRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.MetadataSuggestion, error) {
			return nil, model.ErrNotFound
		},
	}
	r, _ := buildEnrichmentRouter(nil, nil, suggestionRepo, nil)

	req := httptest.NewRequest(http.MethodPost, "/videos/vid-1/suggestions/sid-ghost/accept", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body: %s", w.Code, w.Body.String())
	}
	var resp model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error != "not_found" {
		t.Errorf("expected error 'not_found', got %q", resp.Error)
	}
}

func TestEnrichmentHandler_AcceptSuggestion_HappyPath(t *testing.T) {
	sug := &model.MetadataSuggestion{
		ID:      "sid-1",
		VideoID: "vid-1",
		Source:  "test",
		Code:    "TEST-001",
		Payload: model.EnrichedMetadata{Code: "TEST-001", Title: "Test Title"},
		Status:  "pending",
	}
	suggestionRepo := &mock.SuggestionRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.MetadataSuggestion, error) {
			return sug, nil
		},
		DeleteFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}
	videoRepo := &mock.VideoRepository{
		UpdateMetadataFunc: func(ctx context.Context, id string, m model.VideoMetadataUpdate) error {
			return nil
		},
	}
	// No actresses or genres in payload, so actressRepo/tagRepo won't be called.
	r, _ := buildEnrichmentRouter(videoRepo, nil, suggestionRepo, nil)

	req := httptest.NewRequest(http.MethodPost, "/videos/vid-1/suggestions/sid-1/accept", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body: %s", w.Code, w.Body.String())
	}
	var resp model.SuccessResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal success response: %v", err)
	}
}

func TestEnrichmentHandler_AcceptSuggestion_MalformedJSON_Returns400(t *testing.T) {
	suggestionRepo := &mock.SuggestionRepository{}
	r, _ := buildEnrichmentRouter(nil, nil, suggestionRepo, nil)

	req := httptest.NewRequest(http.MethodPost, "/videos/vid-1/suggestions/sid-1/accept",
		strings.NewReader(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d body: %s", w.Code, w.Body.String())
	}
}

func TestEnrichmentHandler_AcceptSuggestion_EmptyBody_Returns200(t *testing.T) {
	sug := &model.MetadataSuggestion{
		ID:      "sid-2",
		VideoID: "vid-2",
		Source:  "test",
		Code:    "TEST-002",
		Payload: model.EnrichedMetadata{Code: "TEST-002", Title: "Test 2"},
		Status:  "pending",
	}
	suggestionRepo := &mock.SuggestionRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.MetadataSuggestion, error) {
			return sug, nil
		},
		DeleteFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}
	videoRepo := &mock.VideoRepository{
		UpdateMetadataFunc: func(ctx context.Context, id string, m model.VideoMetadataUpdate) error {
			return nil
		},
	}
	r, _ := buildEnrichmentRouter(videoRepo, nil, suggestionRepo, nil)

	// Empty body — should succeed (treat as zero-value override).
	req := httptest.NewRequest(http.MethodPost, "/videos/vid-2/suggestions/sid-2/accept", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty body, got %d body: %s", w.Code, w.Body.String())
	}
}

// --- RejectSuggestion tests ---

func TestEnrichmentHandler_RejectSuggestion_HappyPath(t *testing.T) {
	sug := &model.MetadataSuggestion{
		ID:      "sid-3",
		VideoID: "vid-3",
	}
	suggestionRepo := &mock.SuggestionRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.MetadataSuggestion, error) {
			return sug, nil
		},
		DeleteFunc: func(ctx context.Context, id string) error {
			return nil
		},
		GetByVideoIDFunc: func(ctx context.Context, videoID string) ([]model.MetadataSuggestion, error) {
			// No remaining suggestions → triggers SetEnrichmentStatus.
			return nil, nil
		},
	}
	videoRepo := &mock.VideoRepository{
		SetEnrichmentStatusFunc: func(ctx context.Context, id, status string) error {
			return nil
		},
	}
	r, _ := buildEnrichmentRouter(videoRepo, nil, suggestionRepo, nil)

	req := httptest.NewRequest(http.MethodDelete, "/videos/vid-3/suggestions/sid-3", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body: %s", w.Code, w.Body.String())
	}
}

// --- StartBatch conflict test ---

func TestEnrichmentHandler_StartBatch_ConflictWhenRunning(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		ListByEnrichmentStatusFunc: func(ctx context.Context, status string) ([]model.Video, error) {
			// Return one video so the batch worker blocks for a moment.
			return []model.Video{{ID: "vid-block", OriginalFilename: "video.mp4"}}, nil
		},
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			// Worker calls EnrichVideo → GetByID. Block in a long sleep via
			// SetEnrichmentStatus so the job stays "running" long enough.
			return &model.Video{ID: id, OriginalFilename: "video.mp4"}, nil
		},
		SetEnrichmentStatusFunc: func(ctx context.Context, id, status string) error {
			// Pause the worker so the job is still running when we call
			// StartBatch a second time.
			time.Sleep(200 * time.Millisecond)
			return nil
		},
	}
	r, _ := buildEnrichmentRouter(videoRepo, nil, nil, nil)

	// First call: should succeed with 202.
	req1 := httptest.NewRequest(http.MethodPost, "/enrich-jobs", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("expected 202 on first StartBatch, got %d body: %s", w1.Code, w1.Body.String())
	}

	// Second call while first is still running: expect 409.
	req2 := httptest.NewRequest(http.MethodPost, "/enrich-jobs", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on second StartBatch, got %d body: %s", w2.Code, w2.Body.String())
	}
}
