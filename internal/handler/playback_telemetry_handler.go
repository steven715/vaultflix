package handler

import (
	"errors"
	"log/slog"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

const (
	defaultTelemetryDays = 30
	minTelemetryDays     = 1
	maxTelemetryDays     = 365
)

type PlaybackTelemetryHandler struct {
	service *service.PlaybackTelemetryService
}

func NewPlaybackTelemetryHandler(svc *service.PlaybackTelemetryService) *PlaybackTelemetryHandler {
	return &PlaybackTelemetryHandler{service: svc}
}

type telemetryRequest struct {
	SessionID        string   `json:"session_id" binding:"required"`
	VideoID          string   `json:"video_id" binding:"required"`
	PlayMode         string   `json:"play_mode" binding:"required"`
	TTFFMs           *float64 `json:"ttff_ms"`
	WatchedMs        int      `json:"watched_ms"`
	RebufferCount    int      `json:"rebuffer_count"`
	RebufferMs       int      `json:"rebuffer_ms"`
	AvgDownlinkMbps  *float64 `json:"avg_downlink_mbps"`
	FatalErrorFamily *string  `json:"fatal_error_family"`
}

// Record persists one playback session's terminal quality summary. Success is
// 204 (no body) — the write is an idempotent upsert, mirroring the heartbeat.
func (h *PlaybackTelemetryHandler) Record(c *gin.Context) {
	userID := c.GetString("user_id")

	var req telemetryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "session_id, video_id and play_mode are required",
		})
		return
	}

	var ttff *int
	if req.TTFFMs != nil {
		v := int(math.Round(*req.TTFFMs))
		ttff = &v
	}

	err := h.service.Record(c.Request.Context(), model.PlaybackTelemetryInput{
		SessionID:        req.SessionID,
		UserID:           userID,
		VideoID:          req.VideoID,
		PlayMode:         req.PlayMode,
		RemoteIP:         c.ClientIP(),
		TTFFMs:           ttff,
		WatchedMs:        req.WatchedMs,
		RebufferCount:    req.RebufferCount,
		RebufferMs:       req.RebufferMs,
		AvgDownlinkMbps:  req.AvgDownlinkMbps,
		FatalErrorFamily: req.FatalErrorFamily,
	})
	if err != nil {
		if errors.Is(err, model.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid telemetry"})
			return
		}
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "video not found"})
			return
		}
		slog.Error("failed to record playback telemetry", "error", err, "user_id", userID, "video_id", req.VideoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to record telemetry"})
		return
	}

	c.Status(http.StatusNoContent)
}

// Summary returns per-play_mode aggregates. days/scope are request-tunable.
func (h *PlaybackTelemetryHandler) Summary(c *gin.Context) {
	q := model.TelemetryQuery{
		Days:  clampParam(c.Query("days"), defaultTelemetryDays, minTelemetryDays, maxTelemetryDays),
		Scope: normalizeScope(c.Query("scope")),
	}

	summary, err := h.service.Summary(c.Request.Context(), q)
	if err != nil {
		slog.Error("failed to build telemetry summary", "error", err, "days", q.Days, "scope", q.Scope)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to build telemetry"})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{Data: summary})
}

// normalizeScope allows only known filters; anything else means "all scopes".
func normalizeScope(raw string) string {
	switch raw {
	case "lan", "external", "unknown":
		return raw
	default:
		return ""
	}
}
