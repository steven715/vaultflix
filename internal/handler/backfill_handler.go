package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

// backfillJobs runs Backfill Jobs (implemented by *service.BackfillRunner).
type backfillJobs interface {
	// Start returns model.ErrInvalidInput (wrapped) for an unknown kind and
	// model.ErrConflict while any Backfill Job is running.
	Start(ctx context.Context, kind model.BackfillKind, userID string) (*model.BackfillJob, error)
	// Active returns the latest job, or nil when none has started.
	Active(ctx context.Context) *model.BackfillJob
	// Cancel returns model.ErrNotFound when jobID is not the latest job.
	Cancel(ctx context.Context, jobID string) error
}

// BackfillHandler exposes admin endpoints to run Backfill Jobs of every kind.
// All routes live under /api/admin/* and are gated to the admin role by the
// Casbin RBAC middleware in cmd/server/main.go.
type BackfillHandler struct {
	jobs backfillJobs
}

func NewBackfillHandler(jobs backfillJobs) *BackfillHandler {
	return &BackfillHandler{jobs: jobs}
}

type startBackfillRequest struct {
	Kind model.BackfillKind `json:"kind" binding:"required"`
}

// Start launches a Backfill Job of the requested kind. Returns 202 Accepted
// with the job id, 400 for a missing or unknown kind, or 409 Conflict when
// another Backfill Job is already running.
// POST /api/admin/backfill-jobs {"kind": "preview"|"codec"|"keyframe"|"code"}
func (h *BackfillHandler) Start(c *gin.Context) {
	var req startBackfillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "kind is required"})
		return
	}
	job, err := h.jobs.Start(c.Request.Context(), req.Kind, c.GetString("user_id"))
	if err != nil {
		if errors.Is(err, model.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "unknown backfill kind"})
			return
		}
		if errors.Is(err, model.ErrConflict) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error:   "backfill_in_progress",
				Message: "另一個 backfill 任務正在執行",
			})
			return
		}
		slog.Error("failed to start backfill", "error", err, "kind", req.Kind)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "啟動 backfill 失敗",
		})
		return
	}
	c.JSON(http.StatusAccepted, model.SuccessResponse{Data: gin.H{"job_id": job.ID, "kind": job.Kind}})
}

// GetActive returns the most recent Backfill Job of any kind (running or finished). The
// frontend uses this to restore the progress panel after a page reload.
// Responds with {"data": null} when no backfill has ever started in this
// process.
func (h *BackfillHandler) GetActive(c *gin.Context) {
	job := h.jobs.Active(c.Request.Context())
	c.JSON(http.StatusOK, model.SuccessResponse{Data: job})
}

// Cancel asks the running job with the URL :id to stop after the current
// video finishes. Idempotent against finished jobs, 404 when :id matches no
// known job.
func (h *BackfillHandler) Cancel(c *gin.Context) {
	jobID := c.Param("id")
	if err := h.jobs.Cancel(c.Request.Context(), jobID); err != nil {
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
