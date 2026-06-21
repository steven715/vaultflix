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

// setupStreamScopeRouter wires the real streaming route plus an unrelated route,
// both behind JWTAuth, so scope enforcement can be exercised end to end.
func setupStreamScopeRouter() *gin.Engine {
	r := gin.New()
	r.Use(JWTAuth(testJWTSecret))
	ok := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
	r.GET("/api/videos/:id/stream", ok)
	r.GET("/api/videos/:id/other", ok)
	r.GET("/api/users", ok)
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

func TestJWTAuth_StreamScope(t *testing.T) {
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
