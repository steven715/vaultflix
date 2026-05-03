package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

// BackfillHandler exposes admin endpoints to manage the preview-backfill job.
// All routes live under /api/admin/* and are gated to the admin role by the
// Casbin RBAC middleware in cmd/server/main.go.
type BackfillHandler struct {
	svc *service.BackfillService
}

func NewBackfillHandler(svc *service.BackfillService) *BackfillHandler {
	return &BackfillHandler{svc: svc}
}

// Start launches a new backfill job. Returns 202 Accepted with the job id on
// success, or 409 Conflict when another backfill is already running.
func (h *BackfillHandler) Start(c *gin.Context) {
	userID := c.GetString("user_id")
	job, err := h.svc.StartAsync(userID)
	if err != nil {
		if errors.Is(err, model.ErrConflict) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error:   "backfill_in_progress",
				Message: "另一個 backfill 任務正在執行",
			})
			return
		}
		slog.Error("failed to start backfill", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "啟動 backfill 失敗",
		})
		return
	}
	c.JSON(http.StatusAccepted, model.SuccessResponse{Data: gin.H{"job_id": job.ID}})
}

// GetActive returns the most recent backfill job (running or finished). The
// frontend uses this to restore the progress panel after a page reload.
// Responds with {"data": null} when no backfill has ever started in this
// process.
func (h *BackfillHandler) GetActive(c *gin.Context) {
	job := h.svc.GetActiveJob()
	c.JSON(http.StatusOK, model.SuccessResponse{Data: job})
}

// Cancel asks the running job with the URL :id to stop after the current
// video finishes. Idempotent against finished jobs, 404 when :id matches no
// known job.
func (h *BackfillHandler) Cancel(c *gin.Context) {
	jobID := c.Param("id")
	if err := h.svc.Cancel(jobID); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "backfill job not found",
			})
			return
		}
		slog.Error("failed to cancel backfill", "error", err, "job_id", jobID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "取消 backfill 失敗",
		})
		return
	}
	c.Status(http.StatusNoContent)
}
