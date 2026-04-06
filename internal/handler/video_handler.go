package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	importService      *service.ImportService
	videoService       *service.VideoService
	mediaSourceService *service.MediaSourceService
}

func NewVideoHandler(importService *service.ImportService, videoService *service.VideoService, mediaSourceService *service.MediaSourceService) *VideoHandler {
	return &VideoHandler{
		importService:      importService,
		videoService:       videoService,
		mediaSourceService: mediaSourceService,
	}
}

type importRequest struct {
	SourceID string `json:"source_id" binding:"required"`
}

func (h *VideoHandler) Import(c *gin.Context) {
	var req importRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "source_id is required",
		})
		return
	}

	ctx := c.Request.Context()

	source, err := h.mediaSourceService.GetByID(ctx, req.SourceID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "media source not found",
			})
			return
		}
		slog.Error("failed to get media source", "error", err, "source_id", req.SourceID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to get media source",
		})
		return
	}

	if !source.Enabled {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "media source is disabled",
		})
		return
	}

	result, err := h.importService.Run(ctx, source)
	if err != nil {
		slog.Error("video import failed", "error", err, "source_id", req.SourceID)
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

	userID := c.GetString("user_id")
	detail, err := h.videoService.GetByID(c.Request.Context(), id, userID)
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

// Stream serves a video file directly from disk using http.ServeFile.
// Supports HTTP Range Request (seeking), Content-Length, and If-Modified-Since (304).
// Authentication: JWT from Authorization header or ?token= query param.
func (h *VideoHandler) Stream(c *gin.Context) {
	ctx := c.Request.Context()
	videoID := c.Param("id")

	video, err := h.videoService.GetByID(ctx, videoID, "")
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "video not found",
			})
			return
		}
		slog.Error("failed to get video for streaming", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to get video",
		})
		return
	}

	// Legacy mode: video stored in MinIO (no source_id, has minio_object_key)
	if video.SourceID == nil && video.MinIOObjectKey != "" {
		presignedURL, err := h.videoService.GetPresignedURL(ctx, video.MinIOObjectKey)
		if err != nil {
			slog.Error("failed to generate presigned url for legacy video",
				"error", err, "video_id", videoID)
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to generate stream url",
			})
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, presignedURL)
		return
	}

	// New mode: video stored on local disk via media source
	if video.SourceID == nil || video.FilePath == nil {
		slog.Error("video has no source and no minio key", "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "video has no playable source",
		})
		return
	}

	source, err := h.mediaSourceService.GetByID(ctx, *video.SourceID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			slog.Error("media source not found for video",
				"video_id", videoID, "source_id", *video.SourceID)
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "media source not found",
			})
			return
		}
		slog.Error("failed to get media source", "error", err, "source_id", *video.SourceID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to get media source",
		})
		return
	}

	if !source.Enabled {
		c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{
			Error:   "source_unavailable",
			Message: "media source is currently disabled",
		})
		return
	}

	fullPath := filepath.Join(source.MountPath, *video.FilePath)
	cleanPath := filepath.Clean(fullPath)

	// Path traversal protection: resolved path must stay within the source's mount path.
	// Append separator to prevent prefix collision (e.g. /mnt/videos vs /mnt/videos-extra).
	cleanMount := filepath.Clean(source.MountPath)
	if !strings.HasPrefix(cleanPath, cleanMount+string(filepath.Separator)) && cleanPath != cleanMount {
		c.JSON(http.StatusForbidden, model.ErrorResponse{
			Error:   "path_not_allowed",
			Message: "resolved file path is outside allowed area",
		})
		return
	}

	if _, err := os.Stat(cleanPath); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "file_not_found",
				Message: "video file not found on disk (may have been moved or drive unmounted)",
			})
			return
		}
		slog.Error("failed to stat video file", "error", err, "path", cleanPath)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to access video file",
		})
		return
	}

	c.Header("Content-Type", video.MimeType)
	http.ServeFile(c.Writer, c.Request, cleanPath)
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
