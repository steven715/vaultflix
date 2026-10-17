package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

type WatchSessionHandler struct {
	service *service.WatchSessionService
}

func NewWatchSessionHandler(svc *service.WatchSessionService) *WatchSessionHandler {
	return &WatchSessionHandler{service: svc}
}

type heartbeatRequest struct {
	SessionID       string `json:"session_id" binding:"required"`
	VideoID         string `json:"video_id" binding:"required"`
	PlayedDelta     int    `json:"played_delta"`
	PositionSeconds int    `json:"position_seconds"`
}

// Heartbeat records one accumulated playback delta for a viewing session.
func (h *WatchSessionHandler) Heartbeat(c *gin.Context) {
	userID := c.GetString("user_id")

	var req heartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "session_id and video_id are required")
		return
	}

	err := h.service.RecordHeartbeat(c.Request.Context(), model.HeartbeatInput{
		SessionID:       req.SessionID,
		UserID:          userID,
		VideoID:         req.VideoID,
		PlayedDelta:     req.PlayedDelta,
		PositionSeconds: req.PositionSeconds,
	})
	if err != nil {
		writeError(c, err, "record heartbeat", "user_id", userID, "video_id", req.VideoID)
		return
	}

	c.Status(http.StatusNoContent)
}
