package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	gorillaWS "github.com/gorilla/websocket"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/websocket"
)

var upgrader = gorillaWS.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Self-hosted single-user app; allow all origins.
		return true
	},
}

// WSHandler handles WebSocket upgrade requests and admin test endpoints.
type WSHandler struct {
	hub *websocket.Hub
}

// NewWSHandler creates a WSHandler with the given Hub.
func NewWSHandler(hub *websocket.Hub) *WSHandler {
	return &WSHandler{hub: hub}
}

// HandleWebSocket upgrades the HTTP connection to a WebSocket connection.
// The JWT auth middleware must run before this handler to populate user_id.
func (h *WSHandler) HandleWebSocket(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, ok := userID.(string)
	if !ok || uid == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
			Error:   "unauthorized",
			Message: "missing user identity",
		})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// gorilla already wrote an HTTP error response on upgrade failure.
		slog.Error("websocket upgrade failed",
			"error", err,
			"user_id", uid,
		)
		return
	}

	client := websocket.NewClient(h.hub, conn, uid)
	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}

type wsTestRequest struct {
	UserID  string `json:"user_id" binding:"required"`
	Message string `json:"message" binding:"required"`
}

// TestSend is a temporary admin-only endpoint for verifying WebSocket delivery.
// POST /api/admin/ws-test
// TODO: remove or keep as admin tool after Phase 12
func (h *WSHandler) TestSend(c *gin.Context) {
	var req wsTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "user_id and message are required",
		})
		return
	}

	h.hub.SendToUser(req.UserID, &websocket.Message{
		Type: websocket.TypeNotification,
		Payload: websocket.NotificationPayload{
			Title:   "Test",
			Message: req.Message,
			Level:   "info",
		},
	})

	c.JSON(http.StatusOK, model.SuccessResponse{Data: "sent"})
}
