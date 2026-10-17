package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	gorillaWS "github.com/gorilla/websocket"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/websocket"
)

// WSHandler handles WebSocket upgrade requests.
type WSHandler struct {
	hub      *websocket.Hub
	upgrader gorillaWS.Upgrader
}

// NewWSHandler creates a WSHandler with the given Hub.
func NewWSHandler(hub *websocket.Hub) *WSHandler {
	return &WSHandler{
		hub: hub,
		upgrader: gorillaWS.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Self-hosted single-user app; allow all origins.
				return true
			},
		},
	}
}

// HandleWebSocket upgrades the HTTP connection to a WebSocket connection.
// The JWT auth middleware must run before this handler to populate user_id.
func (h *WSHandler) HandleWebSocket(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, ok := userID.(string)
	if !ok || uid == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{Error: codeUnauthorized, Message: "missing user identity"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
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
