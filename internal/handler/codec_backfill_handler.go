package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

// CodecBackfillHandler handles admin-triggered codec backfill requests.
type CodecBackfillHandler struct {
	svc *service.CodecBackfillService
}

// NewCodecBackfillHandler creates a CodecBackfillHandler.
func NewCodecBackfillHandler(svc *service.CodecBackfillService) *CodecBackfillHandler {
	return &CodecBackfillHandler{svc: svc}
}

// Run synchronously backfills codecs for videos missing them (admin only).
func (h *CodecBackfillHandler) Run(c *gin.Context) {
	processed, failed, err := h.svc.Run(c.Request.Context())
	if err != nil {
		slog.Error("codec backfill failed", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "codec backfill failed",
		})
		return
	}
	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{"processed": processed, "failed": failed},
	})
}
