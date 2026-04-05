package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

var allowedSortBy = map[string]bool{
	"created_at":       true,
	"title":            true,
	"duration_seconds": true,
	"file_size_bytes":  true,
}

type VideoHandler struct {
	importService *service.ImportService
	videoService  *service.VideoService
}

func NewVideoHandler(importService *service.ImportService, videoService *service.VideoService) *VideoHandler {
	return &VideoHandler{
		importService: importService,
		videoService:  videoService,
	}
}

type importRequest struct {
	SourceDir string `json:"source_dir" binding:"required"`
}

func (h *VideoHandler) Import(c *gin.Context) {
	var req importRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "source_dir is required",
		})
		return
	}

	result, err := h.importService.Run(c.Request.Context(), req.SourceDir)
	if err != nil {
		slog.Error("video import failed", "error", err, "source_dir", req.SourceDir)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "import_failed",
			Message: "failed to import videos",
		})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: result,
	})
}

func (h *VideoHandler) List(c *gin.Context) {
	filter, err := parseVideoFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: err.Error(),
		})
		return
	}

	videos, total, err := h.videoService.List(c.Request.Context(), filter)
	if err != nil {
		slog.Error("failed to list videos", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to list videos",
		})
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse{
		Data:     videos,
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	})
}

func (h *VideoHandler) GetByID(c *gin.Context) {
	id := c.Param("id")

	expiryMinutes := 120
	if raw := c.Query("url_expiry_minutes"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1440 {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "url_expiry_minutes must be between 1 and 1440",
			})
			return
		}
		expiryMinutes = parsed
	}

	expiry := time.Duration(expiryMinutes) * time.Minute

	detail, err := h.videoService.GetByID(c.Request.Context(), id, expiry)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "video not found",
			})
			return
		}
		slog.Error("failed to get video", "error", err, "video_id", id)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to get video",
		})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: detail,
	})
}

type updateVideoRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
}

func (h *VideoHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req updateVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "title is required",
		})
		return
	}

	input := model.UpdateVideoInput{
		Title:       req.Title,
		Description: req.Description,
	}

	video, err := h.videoService.Update(c.Request.Context(), id, input)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "video not found",
			})
			return
		}
		slog.Error("failed to update video", "error", err, "video_id", id)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to update video",
		})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: video,
	})
}

func (h *VideoHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	err := h.videoService.Delete(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "video not found",
			})
			return
		}
		slog.Error("failed to delete video", "error", err, "video_id", id)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to delete video",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

func parseVideoFilter(c *gin.Context) (model.VideoFilter, error) {
	filter := model.VideoFilter{
		Page:      1,
		PageSize:  20,
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	if raw := c.Query("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			return filter, errors.New("page must be a positive integer")
		}
		filter.Page = page
	}

	if raw := c.Query("page_size"); raw != "" {
		pageSize, err := strconv.Atoi(raw)
		if err != nil || pageSize < 1 || pageSize > 100 {
			return filter, errors.New("page_size must be between 1 and 100")
		}
		filter.PageSize = pageSize
	}

	if raw := c.Query("sort_by"); raw != "" {
		if !allowedSortBy[raw] {
			return filter, errors.New("sort_by must be one of: created_at, title, duration_seconds, file_size_bytes")
		}
		filter.SortBy = raw
	}

	if raw := c.Query("sort_order"); raw != "" {
		lower := strings.ToLower(raw)
		if lower != "asc" && lower != "desc" {
			return filter, errors.New("sort_order must be asc or desc")
		}
		filter.SortOrder = lower
	}

	filter.Query = c.Query("q")

	if raw := c.Query("tag_ids"); raw != "" {
		parts := strings.Split(raw, ",")
		for _, p := range parts {
			id, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || id < 1 {
				return filter, errors.New("tag_ids must be comma-separated positive integers")
			}
			filter.TagIDs = append(filter.TagIDs, id)
		}
	}

	return filter, nil
}
