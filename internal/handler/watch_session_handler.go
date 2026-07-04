package handler

import (
	"errors"
	"log/slog"
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
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "session_id and video_id are required",
		})
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
		if errors.Is(err, model.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid heartbeat"})
			return
		}
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "video not found"})
			return
		}
		slog.Error("failed to record heartbeat", "error", err, "user_id", userID, "video_id", req.VideoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to record heartbeat"})
		return
	}

	c.Status(http.StatusNoContent)
}
