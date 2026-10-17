package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

// EnrichmentHandler exposes endpoints for triggering per-video enrichment,
// managing metadata suggestions, and controlling batch enrichment jobs.
type EnrichmentHandler struct {
	svc *service.EnrichmentService
}

// NewEnrichmentHandler constructs an EnrichmentHandler.
func NewEnrichmentHandler(svc *service.EnrichmentService) *EnrichmentHandler {
	return &EnrichmentHandler{svc: svc}
}

// EnrichVideo triggers synchronous enrichment for a single video.
// Returns 202 Accepted on success, 422 when the filename yields no code,
// 404 when the video is not found, 500 otherwise.
//
// NOTE: Phase 1 runs enrichment synchronously inside the HTTP handler.
// For videos with multiple scrapers this can take several seconds. A future
// phase should move this to a background goroutine with job-tracking so the
// handler can return 202 immediately and the client polls for progress via WS.
func (h *EnrichmentHandler) EnrichVideo(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")

	if err := h.svc.EnrichVideo(c.Request.Context(), id, userID); err != nil {
		writeError(c, err, "enrich video", "video_id", id)
		return
	}
	c.JSON(http.StatusAccepted, model.SuccessResponse{Data: gin.H{"video_id": id}})
}

// ListSuggestions returns all staged metadata suggestions for a video.
// 200 with an array (possibly empty); 500 on repo error.
func (h *EnrichmentHandler) ListSuggestions(c *gin.Context) {
	id := c.Param("id")
	suggestions, err := h.svc.ListSuggestions(c.Request.Context(), id)
	if err != nil {
		writeError(c, err, "list suggestions", "video_id", id)
		return
	}
	c.JSON(http.StatusOK, model.SuccessResponse{Data: suggestions})
}

// AcceptSuggestion applies a staged suggestion's metadata to the video.
// An empty request body is treated as a zero-value override (no overrides applied).
// Returns 200 on success, 400 for malformed JSON, 404 if suggestion not found, 500 otherwise.
func (h *EnrichmentHandler) AcceptSuggestion(c *gin.Context) {
	id := c.Param("id")
	sid := c.Param("sid")

	var ov model.SuggestionOverride
	if err := c.ShouldBindJSON(&ov); err != nil && !errors.Is(err, io.EOF) {
		badRequest(c, "request body is not valid JSON")
		return
	}

	if err := h.svc.AcceptSuggestion(c.Request.Context(), id, sid, ov); err != nil {
		writeError(c, err, "accept suggestion", "video_id", id, "suggestion_id", sid)
		return
	}
	c.JSON(http.StatusOK, model.SuccessResponse{Data: gin.H{"status": "accepted"}})
}

// RejectSuggestion deletes a staged suggestion.
// Returns 204 No Content on success, 404 if not found, 500 otherwise.
func (h *EnrichmentHandler) RejectSuggestion(c *gin.Context) {
	id := c.Param("id")
	sid := c.Param("sid")

	if err := h.svc.RejectSuggestion(c.Request.Context(), id, sid); err != nil {
		writeError(c, err, "reject suggestion", "video_id", id, "suggestion_id", sid)
		return
	}
	c.Status(http.StatusNoContent)
}

// StartBatch launches a background batch enrichment job.
// Optional JSON body: {"status":"pending","auto_accept":false,"limit":0} — empty body defaults to pending, no auto-accept, no cap.
// Returns 202 Accepted with job_id on success, 409 Conflict when a job is running, 500 otherwise.
func (h *EnrichmentHandler) StartBatch(c *gin.Context) {
	userID := c.GetString("user_id")

	var req model.EnrichBatchRequest
	// Empty body is fine; ignore EOF.
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		badRequest(c, "request body is not valid JSON")
		return
	}

	job, err := h.svc.StartBatchAsync(c.Request.Context(), req, userID)
	if err != nil {
		writeError(c, err, "start batch enrichment")
		return
	}
	c.JSON(http.StatusAccepted, model.SuccessResponse{Data: gin.H{"job_id": job.ID}})
}

// ActiveJob returns a snapshot of the most recent batch enrichment job.
// Returns {"data": null} when no batch has ever started.
func (h *EnrichmentHandler) ActiveJob(c *gin.Context) {
	job := h.svc.ActiveJob()
	c.JSON(http.StatusOK, model.SuccessResponse{Data: job})
}

// CancelBatch requests the running job to stop after the current video finishes.
// Returns 204 No Content on success, 404 if no such job, 500 otherwise.
func (h *EnrichmentHandler) CancelBatch(c *gin.Context) {
	jobID := c.Param("jid")

	if err := h.svc.CancelBatch(jobID); err != nil {
		writeError(c, err, "cancel enrichment job", "job_id", jobID)
		return
	}
	c.Status(http.StatusNoContent)
}
