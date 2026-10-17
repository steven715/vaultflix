package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

type VideoHandler struct {
	importService      *service.ImportService
	videoService       *service.VideoService
	mediaSourceService *service.MediaSourceService
	// xaccelPrefix, when non-empty, makes Stream() offload byte serving to an
	// upstream accel-capable proxy (nginx) via X-Accel-Redirect instead of
	// streaming the file through this process. Empty → direct http.ServeFile.
	xaccelPrefix string
}

func NewVideoHandler(importService *service.ImportService, videoService *service.VideoService, mediaSourceService *service.MediaSourceService, xaccelPrefix string) *VideoHandler {
	return &VideoHandler{
		importService:      importService,
		videoService:       videoService,
		mediaSourceService: mediaSourceService,
		xaccelPrefix:       xaccelPrefix,
	}
}

type importRequest struct {
	SourceID string `json:"source_id" binding:"required"`
}

func (h *VideoHandler) Import(c *gin.Context) {
	var req importRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "source_id is required")
		return
	}

	ctx := c.Request.Context()

	job, err := h.importService.StartAsync(ctx, req.SourceID, c.GetString("user_id"))
	if err != nil {
		writeError(c, err, "start import")
		return
	}

	c.JSON(http.StatusAccepted, model.SuccessResponse{Data: job})
}

func (h *VideoHandler) List(c *gin.Context) {
	filter, err := service.ParseVideoFilter(c.Query)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	videos, total, err := h.videoService.List(c.Request.Context(), filter)
	if err != nil {
		writeError(c, err, "list videos")
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
		writeError(c, err, "get video", "video_id", id)
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
		badRequest(c, "title is required")
		return
	}

	input := model.UpdateVideoInput{
		Title:       req.Title,
		Description: req.Description,
	}

	video, err := h.videoService.Update(c.Request.Context(), id, input)
	if err != nil {
		writeError(c, err, "update video", "video_id", id)
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
		writeError(c, err, "delete video", "video_id", id)
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

	// Only where the bytes are: a playing <video> makes many Range requests,
	// and none of them needs the detail page's Tags or image URLs.
	src, err := h.videoService.ResolvePlayback(ctx, videoID)
	if err != nil {
		writeError(c, err, "resolve video file", "video_id", videoID)
		return
	}

	// Legacy mode: video stored in MinIO
	if src.IsLegacy() {
		presignedURL, err := h.videoService.GetPresignedURL(ctx, src.LegacyObjectKey)
		if err != nil {
			writeError(c, err, "generate stream url", "video_id", videoID)
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, presignedURL)
		return
	}
	cleanPath := src.Path

	// Offload mode: hand the byte path to nginx via X-Accel-Redirect so video
	// bytes never traverse this process. Go has already done auth, path safety
	// and the existence check (ResolveFile); nginx owns Content-Type (by
	// extension) and native Range.
	if h.xaccelPrefix != "" {
		rel := strings.TrimPrefix(cleanPath, h.mediaSourceService.MountPrefix())
		if rel == cleanPath {
			// Defensive: path is not under the shared mount prefix, so nginx's
			// `alias /mnt/host/` could not resolve it. Fall back to direct serve.
			slog.Warn("xaccel path not under mount prefix, falling back to direct serve", "path", cleanPath)
		} else {
			c.Header("X-Accel-Redirect", h.xaccelPrefix+encodePathSegments(rel))
			c.Status(http.StatusOK) // empty body, nginx takes over
			return
		}
	}

	c.Header("Content-Type", src.MimeType)
	http.ServeFile(c.Writer, c.Request, cleanPath)
}

// encodePathSegments URL-encodes each path segment while preserving the slash
// separators, so filenames containing spaces or non-ASCII (e.g. Chinese)
// characters produce a valid X-Accel-Redirect value. nginx unescapes the value
// before resolving the file; url.PathEscape encodes spaces as %20 (not +),
// matching nginx's expectation.
func encodePathSegments(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
