package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

type MediaSourceHandler struct {
	service *service.MediaSourceService
}

func NewMediaSourceHandler(service *service.MediaSourceService) *MediaSourceHandler {
	return &MediaSourceHandler{service: service}
}

func (h *MediaSourceHandler) List(c *gin.Context) {
	sources, err := h.service.List(c.Request.Context())
	if err != nil {
		writeError(c, err, "list media sources")
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{Data: sources})
}

type createMediaSourceRequest struct {
	Label     string `json:"label" binding:"required"`
	MountPath string `json:"mount_path" binding:"required"`
}

func (h *MediaSourceHandler) Create(c *gin.Context) {
	var req createMediaSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "label and mount_path are required")
		return
	}

	source := &model.MediaSource{
		Label:     req.Label,
		MountPath: req.MountPath,
	}

	err := h.service.Create(c.Request.Context(), source)
	if err != nil {
		writeCreateSourceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, model.SuccessResponse{Data: source})
}

type updateMediaSourceRequest struct {
	Label   string `json:"label" binding:"required"`
	Enabled *bool  `json:"enabled" binding:"required"`
}

func (h *MediaSourceHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req updateMediaSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "label and enabled are required")
		return
	}

	err := h.service.Update(c.Request.Context(), id, req.Label, *req.Enabled)
	if err != nil {
		writeError(c, err, "update media source", "media_source_id", id)
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{"message": "media source updated"},
	})
}

func (h *MediaSourceHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	err := h.service.Delete(c.Request.Context(), id)
	if err != nil {
		writeError(c, err, "delete media source", "media_source_id", id)
		return
	}

	c.Status(http.StatusNoContent)
}

// writeCreateSourceError answers a refused new Media Source. Its mount path is
// the admin's own input, so a path outside the prefix or missing on disk is a
// 400 with its own code, not the 403/404 a resolved stream path gets.
func writeCreateSourceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrPathNotAllowed):
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: codePathNotAllowed, Message: "mount_path is outside the allowed mount prefix"})
	case errors.Is(err, model.ErrPathNotExist):
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: codePathNotExist, Message: "mount_path does not exist"})
	default:
		writeError(c, err, "create media source")
	}
}
