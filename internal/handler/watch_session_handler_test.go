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

func setupWatchSessionRouter(repo *mock.WatchSessionRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewWatchSessionService(repo)
	h := NewWatchSessionHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "u1") })
	r.POST("/api/watch-sessions/heartbeat", h.Heartbeat)
	return r
}

func postJSON(r *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/watch-sessions/heartbeat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHeartbeat_OK(t *testing.T) {
	var got model.HeartbeatInput
	repo := &mock.WatchSessionRepository{
		UpsertFunc: func(_ context.Context, in model.HeartbeatInput) error { got = in; return nil },
	}
	r := setupWatchSessionRouter(repo)
	w := postJSON(r, `{"session_id":"s1","video_id":"v1","played_delta":15,"position_seconds":42}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", w.Code, w.Body.String())
	}
	if got.UserID != "u1" || got.VideoID != "v1" || got.PlayedDelta != 15 {
		t.Fatalf("unexpected heartbeat forwarded: %+v", got)
	}
}

func TestHeartbeat_MissingFields_400(t *testing.T) {
	repo := &mock.WatchSessionRepository{UpsertFunc: func(_ context.Context, _ model.HeartbeatInput) error { return nil }}
	r := setupWatchSessionRouter(repo)
	w := postJSON(r, `{"session_id":"s1"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var resp model.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Fatalf("expected error body")
	}
}

func TestHeartbeat_VideoNotFound_404(t *testing.T) {
	repo := &mock.WatchSessionRepository{
		UpsertFunc: func(_ context.Context, _ model.HeartbeatInput) error { return model.ErrNotFound },
	}
	r := setupWatchSessionRouter(repo)
	w := postJSON(r, `{"session_id":"s1","video_id":"vX","played_delta":5,"position_seconds":5}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
