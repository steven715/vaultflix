package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/steven/vaultflix/internal/model"
)

// setupStreamScopeRouter mounts stream routes plus unrelated ones behind
// JWTAuth and the route guard, to exercise the Stream Token's Token Scope end
// to end. This fixture marks every route Viewer so only the Token Scope rule
// decides; the real table's viewer permissions are pinned in cmd/server.
func setupStreamScopeRouter() *gin.Engine {
	r := gin.New()
	api := r.Group("/api")
	api.Use(JWTAuth(testJWTSecret))
	ok := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
	RegisterRoutes(api, []Route{
		{Method: http.MethodGet, Path: "/videos/:id/stream", Handler: ok, Viewer: true, StreamToken: true},
		{Method: http.MethodGet, Path: "/videos/:id/hls/index.m3u8", Handler: ok, Viewer: true, StreamToken: true},
		{Method: http.MethodGet, Path: "/videos/:id/hls/:segment", Handler: ok, Viewer: true, StreamToken: true},
		{Method: http.MethodGet, Path: "/videos/:id/other", Handler: ok, Viewer: true},
		{Method: http.MethodGet, Path: "/users", Handler: ok, Viewer: true},
	})
	return r
}

func streamClaims(videoID string) jwt.MapClaims {
	return jwt.MapClaims{
		"user_id":  "u1",
		"username": "alice",
		"role":     "viewer",
		"scope":    model.StreamTokenScope,
		"video_id": videoID,
		"exp":      float64(time.Now().Add(time.Hour).Unix()),
	}
}

func TestRegisterRoutes_StreamTokenScope(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		path   string
		status int
	}{
		{
			name:   "stream token on its stream route",
			token:  generateTestToken(t, testJWTSecret, streamClaims("v1")),
			path:   "/api/videos/v1/stream",
			status: http.StatusOK,
		},
		{
			name:   "stream token on a different video's stream route",
			token:  generateTestToken(t, testJWTSecret, streamClaims("v1")),
			path:   "/api/videos/v2/stream",
			status: http.StatusForbidden,
		},
		{
			name:   "stream token on a non-stream route",
			token:  generateTestToken(t, testJWTSecret, streamClaims("v1")),
			path:   "/api/videos/v1/other",
			status: http.StatusForbidden,
		},
		{
			name:   "stream token on an unrelated endpoint",
			token:  generateTestToken(t, testJWTSecret, streamClaims("v1")),
			path:   "/api/users",
			status: http.StatusForbidden,
		},
		{
			name:   "stream token on HLS playlist for same video",
			token:  generateTestToken(t, testJWTSecret, streamClaims("v1")),
			path:   "/api/videos/v1/hls/index.m3u8",
			status: http.StatusOK,
		},
		{
			name:   "stream token on HLS segment for same video",
			token:  generateTestToken(t, testJWTSecret, streamClaims("v1")),
			path:   "/api/videos/v1/hls/seg00001.ts",
			status: http.StatusOK,
		},
		{
			name:   "stream token on HLS playlist for different video",
			token:  generateTestToken(t, testJWTSecret, streamClaims("v1")),
			path:   "/api/videos/v2/hls/index.m3u8",
			status: http.StatusForbidden,
		},
		{
			name:   "stream token on HLS segment for different video",
			token:  generateTestToken(t, testJWTSecret, streamClaims("v1")),
			path:   "/api/videos/v2/hls/seg00001.ts",
			status: http.StatusForbidden,
		},
		{
			name:   "full token works on the stream route",
			token:  generateTestToken(t, testJWTSecret, validClaims()),
			path:   "/api/videos/v1/stream",
			status: http.StatusOK,
		},
		{
			name:   "full token works on an unrelated endpoint",
			token:  generateTestToken(t, testJWTSecret, validClaims()),
			path:   "/api/users",
			status: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupStreamScopeRouter()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.status {
				t.Fatalf("expected %d, got %d, body: %s", tt.status, w.Code, w.Body.String())
			}
		})
	}
}
