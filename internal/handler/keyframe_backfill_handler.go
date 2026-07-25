package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

// keyframeBackfiller 執行 keyframe 邊界表 backfill(由 *service.KeyframeService 實作)。
type keyframeBackfiller interface {
	RunBackfill(ctx context.Context) (int, int, error)
}

// KeyframeBackfillHandler handles admin-triggered keyframe-index backfill requests.
type KeyframeBackfillHandler struct {
	svc keyframeBackfiller
}

// NewKeyframeBackfillHandler 建立 KeyframeBackfillHandler。
func NewKeyframeBackfillHandler(svc keyframeBackfiller) *KeyframeBackfillHandler {
	return &KeyframeBackfillHandler{svc: svc}
}

// Run 同步 backfill 所有缺 keyframe 邊界表的 remux 影片(admin only)。
// 注意:全量約 85 分鐘 I/O,呼叫端(curl/Admin UI)自行決定 timeout。
func (h *KeyframeBackfillHandler) Run(c *gin.Context) {
	processed, failed, err := h.svc.RunBackfill(c.Request.Context())
	if err != nil {
		slog.Error("keyframe backfill failed", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "keyframe backfill failed",
		})
		return
	}
	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{"processed": processed, "failed": failed},
	})
}
