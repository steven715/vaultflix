package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

func (h *VideoHandler) GetImportJob(c *gin.Context) {
	jobID := c.Param("id")
	job, err := h.importService.GetJob(jobID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "import job not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to get import job",
		})
		return
	}
	c.JSON(http.StatusOK, model.SuccessResponse{Data: job})
}

func (h *VideoHandler) GetActiveImportJob(c *gin.Context) {
	job := h.importService.GetActiveJob()
	c.JSON(http.StatusOK, model.SuccessResponse{Data: job})
}

// CancelImportJob asks the running Import Job :id to stop after its current
// file; files already imported stay. 204 on success (also for a finished
// Job), 404 when :id is not the latest Import Job.
// POST /api/import-jobs/:id/cancel
func (h *VideoHandler) CancelImportJob(c *gin.Context) {
	jobID := c.Param("id")
	if err := h.importService.Cancel(c.Request.Context(), jobID); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "import job not found"})
			return
		}
		slog.Error("failed to cancel import", "error", err, "job_id", jobID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "取消匯入失敗"})
		return
	}
	c.Status(http.StatusNoContent)
}
