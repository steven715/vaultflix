package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

var allowedCategories = map[string]bool{
	"genre":  true,
	"actor":  true,
	"studio": true,
	"custom": true,
}

type TagHandler struct {
	tagRepo   repository.TagRepository
	videoRepo repository.VideoRepository
}

func NewTagHandler(tagRepo repository.TagRepository, videoRepo repository.VideoRepository) *TagHandler {
	return &TagHandler{
		tagRepo:   tagRepo,
		videoRepo: videoRepo,
	}
}

func (h *TagHandler) List(c *gin.Context) {
	category := c.Query("category")
	if category != "" && !allowedCategories[category] {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "category must be one of: genre, actor, studio, custom",
		})
		return
	}

	tags, err := h.tagRepo.List(c.Request.Context(), category)
	if err != nil {
		slog.Error("failed to list tags", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to list tags",
		})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: tags,
	})
}

type createTagRequest struct {
	Name     string `json:"name" binding:"required"`
	Category string `json:"category"`
}

func (h *TagHandler) Create(c *gin.Context) {
	var req createTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "name is required",
		})
		return
	}

	if req.Category == "" {
		req.Category = "custom"
	}

	if !allowedCategories[req.Category] {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "category must be one of: genre, actor, studio, custom",
		})
		return
	}

	tag := &model.Tag{
		Name:     req.Name,
		Category: req.Category,
	}

	if err := h.tagRepo.Create(c.Request.Context(), tag); err != nil {
		if errors.Is(err, model.ErrAlreadyExists) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error:   "conflict",
				Message: "tag name already exists",
			})
			return
		}
		slog.Error("failed to create tag", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to create tag",
		})
		return
	}

	c.JSON(http.StatusCreated, model.SuccessResponse{
		Data: tag,
	})
}

type addVideoTagRequest struct {
	TagID int `json:"tag_id" binding:"required"`
}

func (h *TagHandler) AddVideoTag(c *gin.Context) {
	videoID := c.Param("id")

	var req addVideoTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "tag_id is required",
		})
		return
	}

	// Verify video exists
	if _, err := h.videoRepo.GetByID(c.Request.Context(), videoID); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "video not found",
			})
			return
		}
		slog.Error("failed to check video", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to add tag to video",
		})
		return
	}

	// Verify tag exists
	if _, err := h.tagRepo.GetByID(c.Request.Context(), req.TagID); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "tag not found",
			})
			return
		}
		slog.Error("failed to check tag", "error", err, "tag_id", req.TagID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to add tag to video",
		})
		return
	}

	if err := h.tagRepo.AddVideoTag(c.Request.Context(), videoID, req.TagID); err != nil {
		if errors.Is(err, model.ErrConflict) {
			// Silently succeed — idempotent behavior
			c.JSON(http.StatusOK, model.SuccessResponse{
				Data: gin.H{"video_id": videoID, "tag_id": req.TagID},
			})
			return
		}
		slog.Error("failed to add video tag", "error", err, "video_id", videoID, "tag_id", req.TagID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to add tag to video",
		})
		return
	}

	c.JSON(http.StatusCreated, model.SuccessResponse{
		Data: gin.H{"video_id": videoID, "tag_id": req.TagID},
	})
}

func (h *TagHandler) RemoveVideoTag(c *gin.Context) {
	videoID := c.Param("id")
	tagIDStr := c.Param("tagId")

	tagID, err := strconv.Atoi(tagIDStr)
	if err != nil || tagID < 1 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "invalid tag id",
		})
		return
	}

	if err := h.tagRepo.RemoveVideoTag(c.Request.Context(), videoID, tagID); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "video tag relation not found",
			})
			return
		}
		slog.Error("failed to remove video tag", "error", err, "video_id", videoID, "tag_id", tagID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to remove tag from video",
		})
		return
	}

	c.Status(http.StatusNoContent)
}
