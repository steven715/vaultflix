package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// routeTableRouter mounts a small route table behind JWTAuth, the way
// cmd/server does with the real one.
func routeTableRouter() *gin.Engine {
	r := gin.New()
	api := r.Group("/api")
	api.Use(JWTAuth(testJWTSecret))
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	RegisterRoutes(api, []Route{
		{Method: http.MethodGet, Path: "/videos/:id", Handler: ok, Viewer: true},
		{Method: http.MethodGet, Path: "/videos/:id/stream", Handler: ok, Viewer: true, StreamToken: true},
		{Method: http.MethodGet, Path: "/videos/:id/hls/:segment", Handler: ok, StreamToken: true},
		{Method: http.MethodPost, Path: "/users", Handler: ok},
	})
	return r
}

func TestRouteGuard_RoleAndStreamToken(t *testing.T) {
	login := func(role string) jwt.MapClaims {
		c := validClaims()
		c["role"] = role
		return c
	}
	tests := []struct {
		name   string
		claims jwt.MapClaims
		method string
		path   string
		want   int
	}{
		{"admin reaches an admin-only route", login("admin"), http.MethodPost, "/api/users", http.StatusOK},
		{"admin reaches a stream-token route", login("admin"), http.MethodGet, "/api/videos/v1/hls/seg00000.ts", http.StatusOK},
		{"viewer reaches a viewer route", login("viewer"), http.MethodGet, "/api/videos/v1", http.StatusOK},
		{"viewer is refused an admin-only route", login("viewer"), http.MethodPost, "/api/users", http.StatusForbidden},
		{"viewer is refused a route not granted to viewers", login("viewer"), http.MethodGet, "/api/videos/v1/hls/seg00000.ts", http.StatusForbidden},
		{"unknown role is refused", login("guest"), http.MethodGet, "/api/videos/v1", http.StatusForbidden},
		{"stream token on its own Video's stream route", streamClaims("v1"), http.MethodGet, "/api/videos/v1/stream", http.StatusOK},
		{"stream token for another Video", streamClaims("v2"), http.MethodGet, "/api/videos/v1/stream", http.StatusForbidden},
		{"stream token on a non-stream route", streamClaims("v1"), http.MethodGet, "/api/videos/v1", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+generateTestToken(t, testJWTSecret, tt.claims))
			w := httptest.NewRecorder()
			routeTableRouter().ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

// A role check that cannot find the route (e.g. registered without the
// table) must refuse rather than allow.
func TestRouteGuard_MissingRoleRefused(t *testing.T) {
	r := gin.New()
	RegisterRoutes(r, []Route{{Method: http.MethodGet, Path: "/x", Handler: func(c *gin.Context) { c.Status(http.StatusOK) }, Viewer: true}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 without an authenticated role", w.Code)
	}
}
