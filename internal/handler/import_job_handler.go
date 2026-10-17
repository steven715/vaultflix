package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

func (h *VideoHandler) GetImportJob(c *gin.Context) {
	jobID := c.Param("id")
	job, err := h.importService.GetJob(jobID)
	if err != nil {
		writeError(c, err, "get import job", "job_id", jobID)
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
		writeError(c, err, "cancel import job", "job_id", jobID)
		return
	}
	c.Status(http.StatusNoContent)
}
