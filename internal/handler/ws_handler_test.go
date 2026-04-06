package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	gorillaWS "github.com/gorilla/websocket"

	"github.com/steven/vaultflix/internal/middleware"
	"github.com/steven/vaultflix/internal/websocket"
)

const testJWTSecret = "test-secret-key-for-ws-tests"

func setupWSRouter(hub *websocket.Hub) *gin.Engine {
	r := gin.New()
	h := NewWSHandler(hub)

	api := r.Group("/api")
	api.Use(middleware.JWTAuth(testJWTSecret))
	api.GET("/ws", h.HandleWebSocket)
	return r
}

func generateTestToken(t *testing.T, userID, username, role string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"role":     role,
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

func TestWSHandler_UpgradeSuccess(t *testing.T) {
	hub := websocket.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	r := setupWSRouter(hub)
	server := httptest.NewServer(r)
	defer server.Close()

	token := generateTestToken(t, "user1", "admin", "admin")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws?token=" + token

	dialer := gorillaWS.Dialer{}
	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("expected 101, got %d", resp.StatusCode)
	}
}

func TestWSHandler_NoToken(t *testing.T) {
	hub := websocket.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	r := setupWSRouter(hub)
	server := httptest.NewServer(r)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws"

	dialer := gorillaWS.Dialer{}
	_, resp, err := dialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected dial to fail without token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestWSHandler_InvalidToken(t *testing.T) {
	hub := websocket.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	r := setupWSRouter(hub)
	server := httptest.NewServer(r)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws?token=invalid.token.here"

	dialer := gorillaWS.Dialer{}
	_, resp, err := dialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected dial to fail with invalid token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

