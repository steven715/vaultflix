package handler

import (
	"bytes"
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

func setupTelemetryRouter(repo *mock.PlaybackTelemetryRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewPlaybackTelemetryService(repo)
	h := NewPlaybackTelemetryHandler(svc)
	r := gin.New()
	// Inject a user_id like the JWT middleware would.
	r.Use(func(c *gin.Context) { c.Set("user_id", "u1"); c.Next() })
	r.POST("/api/playback/telemetry", h.Record)
	r.GET("/api/admin/playback/telemetry", h.Summary)
	return r
}

func TestTelemetryRecord_Success(t *testing.T) {
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, _ model.PlaybackTelemetryInput) error { return nil },
	}
	r := setupTelemetryRouter(repo)
	body := `{"session_id":"s1","video_id":"v1","play_mode":"direct","ttff_ms":900,"watched_ms":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/playback/telemetry", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestTelemetryRecord_MissingFields(t *testing.T) {
	r := setupTelemetryRouter(&mock.PlaybackTelemetryRepository{})
	req := httptest.NewRequest(http.MethodPost, "/api/playback/telemetry", bytes.NewBufferString(`{"session_id":"s1"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestTelemetryRecord_InvalidPlayMode(t *testing.T) {
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, _ model.PlaybackTelemetryInput) error {
			t.Fatal("Insert must not run for invalid play_mode")
			return nil
		},
	}
	r := setupTelemetryRouter(repo)
	body := `{"session_id":"s1","video_id":"v1","play_mode":"bogus"}`
	req := httptest.NewRequest(http.MethodPost, "/api/playback/telemetry", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestTelemetryRecord_VideoNotFound(t *testing.T) {
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, _ model.PlaybackTelemetryInput) error { return model.ErrNotFound },
	}
	r := setupTelemetryRouter(repo)
	body := `{"session_id":"s1","video_id":"ghost","play_mode":"direct"}`
	req := httptest.NewRequest(http.MethodPost, "/api/playback/telemetry", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestTelemetrySummary_Success(t *testing.T) {
	var gotDays int
	var gotScope string
	repo := &mock.PlaybackTelemetryRepository{
		AggregateFunc: func(_ context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error) {
			gotDays, gotScope = q.Days, q.Scope
			return []model.PlayModeStats{{PlayMode: "direct", Sessions: 2}}, nil
		},
	}
	r := setupTelemetryRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/playback/telemetry?days=7&scope=external", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if gotDays != 7 || gotScope != "external" {
		t.Fatalf("query not clamped/passed: days=%d scope=%q", gotDays, gotScope)
	}
	var resp struct {
		Data model.TelemetrySummary `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(resp.Data.ByPlayMode) != 1 || resp.Data.ByPlayMode[0].PlayMode != "direct" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestTelemetrySummary_BadScopeBecomesAll(t *testing.T) {
	var gotScope = "sentinel"
	repo := &mock.PlaybackTelemetryRepository{
		AggregateFunc: func(_ context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error) {
			gotScope = q.Scope
			return []model.PlayModeStats{}, nil
		},
	}
	r := setupTelemetryRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/playback/telemetry?scope=wat", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if gotScope != "" {
		t.Fatalf("want empty scope for unknown filter, got %q", gotScope)
	}
}
