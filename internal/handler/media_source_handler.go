package handler

import (
	"errors"
	"log/slog"
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
		slog.Error("failed to list media sources", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to list media sources",
		})
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
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "label and mount_path are required",
		})
		return
	}

	source := &model.MediaSource{
		Label:     req.Label,
		MountPath: req.MountPath,
	}

	err := h.service.Create(c.Request.Context(), source)
	if err != nil {
		if errors.Is(err, model.ErrPathNotAllowed) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: err.Error(),
			})
			return
		}
		if errors.Is(err, model.ErrPathNotExist) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: err.Error(),
			})
			return
		}
		if errors.Is(err, model.ErrAlreadyExists) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error:   "already_exists",
				Message: "a media source with this mount_path already exists",
			})
			return
		}
		slog.Error("failed to create media source", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to create media source",
		})
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
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "label and enabled are required",
		})
		return
	}

	err := h.service.Update(c.Request.Context(), id, req.Label, *req.Enabled)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "media source not found",
			})
			return
		}
		slog.Error("failed to update media source", "error", err, "media_source_id", id)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to update media source",
		})
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
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "media source not found",
			})
			return
		}
		slog.Error("failed to delete media source", "error", err, "media_source_id", id)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to delete media source",
		})
		return
	}

	c.Status(http.StatusNoContent)
}
