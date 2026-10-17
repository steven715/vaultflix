package handler

import (
	"context"
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
		writeError(c, err, "list tags")
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
		badRequest(c, "name is required")
		return
	}

	tag := &model.Tag{
		Name:     req.Name,
		Category: req.Category,
	}

	if err := h.tags.Create(c.Request.Context(), tag); err != nil {
		writeError(c, err, "create tag")
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
		badRequest(c, "tag_id is required")
		return
	}

	created, err := h.tags.AddVideoTag(c.Request.Context(), videoID, req.TagID)
	if err != nil {
		// A NotFoundError says whether the Video or the Tag is missing.
		writeError(c, err, "add video tag", "video_id", videoID, "tag_id", req.TagID)
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
		badRequest(c, "invalid tag id")
		return
	}

	if err := h.tags.RemoveVideoTag(c.Request.Context(), videoID, tagID); err != nil {
		writeError(c, err, "remove tag from video", "video_id", videoID, "tag_id", tagID)
		return
	}

	c.Status(http.StatusNoContent)
}
