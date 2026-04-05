package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

type HistoryHandler struct {
	historySvc service.WatchHistoryService
}

func NewHistoryHandler(historySvc service.WatchHistoryService) *HistoryHandler {
	return &HistoryHandler{historySvc: historySvc}
}

type saveProgressRequest struct {
	VideoID         string `json:"video_id" binding:"required"`
	ProgressSeconds int    `json:"progress_seconds"`
}

func (h *HistoryHandler) SaveProgress(c *gin.Context) {
	userID := c.GetString("user_id")

	var req saveProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "video_id is required",
		})
		return
	}

	if req.ProgressSeconds < 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "progress_seconds must be non-negative",
		})
		return
	}

	err := h.historySvc.SaveProgress(c.Request.Context(), userID, req.VideoID, req.ProgressSeconds)
	if err != nil {
		slog.Error("failed to save watch progress",
			"error", err,
			"user_id", userID,
			"video_id", req.VideoID,
		)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to save watch progress",
		})
		return
	}

	c.JSON(http.StatusCreated, model.SuccessResponse{
		Data: gin.H{"status": "ok"},
	})
}

func (h *HistoryHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")

	page := 1
	if raw := c.Query("page"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "page must be a positive integer",
			})
			return
		}
		page = parsed
	}

	pageSize := 20
	if raw := c.Query("page_size"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "page_size must be between 1 and 100",
			})
			return
		}
		pageSize = parsed
	}

	items, total, err := h.historySvc.List(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		slog.Error("failed to list watch history", "error", err, "user_id", userID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to list watch history",
		})
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse{
		Data:     items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}
