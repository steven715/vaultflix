package handler

import (
	"bytes"
	"context"
	"encoding/json"
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
	api.POST("/admin/ws-test", h.TestSend)
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

func TestWSHandler_TestSend(t *testing.T) {
	hub := websocket.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	r := setupWSRouter(hub)
	server := httptest.NewServer(r)
	defer server.Close()

	// Connect a WebSocket client as the target user
	token := generateTestToken(t, "target-user", "viewer1", "viewer")
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws?token=" + token

	dialer := gorillaWS.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer conn.Close()

	// Allow registration to complete
	time.Sleep(50 * time.Millisecond)

	// Call the admin test endpoint to push a message
	adminToken := generateTestToken(t, "admin1", "admin", "admin")
	body := `{"user_id":"target-user","message":"hello from test"}`
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/admin/ws-test",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to call ws-test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Read the pushed message from the WebSocket
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read websocket message: %v", err)
	}

	var msg websocket.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	if msg.Type != websocket.TypeNotification {
		t.Errorf("expected type %q, got %q", websocket.TypeNotification, msg.Type)
	}
}
