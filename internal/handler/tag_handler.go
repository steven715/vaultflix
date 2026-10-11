package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

// tagService is what TagHandler needs (implemented by *service.TagService).
type tagService interface {
	// List returns model.ErrInvalidInput (wrapped) for an unknown category.
	List(ctx context.Context, category string) ([]model.TagWithCount, error)
	// Create returns model.ErrInvalidInput or model.ErrAlreadyExists (wrapped).
	Create(ctx context.Context, tag *model.Tag) error
	// AddVideoTag returns a *model.NotFoundError naming the missing resource.
	AddVideoTag(ctx context.Context, videoID string, tagID int) (created bool, err error)
	// RemoveVideoTag returns model.ErrNotFound (wrapped) when not tagged.
	RemoveVideoTag(ctx context.Context, videoID string, tagID int) error
}

// badCategory is the 400 body for a category outside the allowed list.
func badCategory() model.ErrorResponse {
	return model.ErrorResponse{
		Error:   "bad_request",
		Message: "category must be one of: genre, actor, studio, custom",
	}
}

type TagHandler struct {
	tags tagService
}

func NewTagHandler(tags tagService) *TagHandler {
	return &TagHandler{tags: tags}
}

func (h *TagHandler) List(c *gin.Context) {
	category := c.Query("category")
	tags, err := h.tags.List(c.Request.Context(), category)
	if err != nil {
		if errors.Is(err, model.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, badCategory())
			return
		}
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

	tag := &model.Tag{
		Name:     req.Name,
		Category: req.Category,
	}

	if err := h.tags.Create(c.Request.Context(), tag); err != nil {
		if errors.Is(err, model.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, badCategory())
			return
		}
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

	created, err := h.tags.AddVideoTag(c.Request.Context(), videoID, req.TagID)
	if err != nil {
		var missing *model.NotFoundError
		if errors.As(err, &missing) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: missing.Error()})
			return
		}
		if errors.Is(err, model.ErrNotFound) { // removed between the checks and the insert
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "video or tag not found"})
			return
		}
		slog.Error("failed to add video tag", "error", err, "video_id", videoID, "tag_id", req.TagID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to add tag to video",
		})
		return
	}

	// Tagging again is idempotent: 200 instead of 201.
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	c.JSON(status, model.SuccessResponse{
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

	if err := h.tags.RemoveVideoTag(c.Request.Context(), videoID, tagID); err != nil {
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
