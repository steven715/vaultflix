package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

type FavoriteHandler struct {
	favoriteSvc service.FavoriteService
}

func NewFavoriteHandler(favoriteSvc service.FavoriteService) *FavoriteHandler {
	return &FavoriteHandler{favoriteSvc: favoriteSvc}
}

type addFavoriteRequest struct {
	VideoID string `json:"video_id" binding:"required"`
}

func (h *FavoriteHandler) Add(c *gin.Context) {
	userID := c.GetString("user_id")

	var req addFavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "video_id is required")
		return
	}

	err := h.favoriteSvc.Add(c.Request.Context(), userID, req.VideoID)
	if err != nil {
		writeError(c, err, "add favorite", "user_id", userID, "video_id", req.VideoID)
		return
	}

	c.JSON(http.StatusCreated, model.SuccessResponse{
		Data: gin.H{"status": "ok"},
	})
}

func (h *FavoriteHandler) Remove(c *gin.Context) {
	userID := c.GetString("user_id")
	videoID := c.Param("videoId")

	err := h.favoriteSvc.Remove(c.Request.Context(), userID, videoID)
	if err != nil {
		writeError(c, err, "remove favorite", "user_id", userID, "video_id", videoID)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *FavoriteHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")

	page := 1
	if raw := c.Query("page"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			badRequest(c, "page must be a positive integer")
			return
		}
		page = parsed
	}

	pageSize := 20
	if raw := c.Query("page_size"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			badRequest(c, "page_size must be between 1 and 100")
			return
		}
		pageSize = parsed
	}

	items, total, err := h.favoriteSvc.List(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		writeError(c, err, "list favorites", "user_id", userID)
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse{
		Data:     items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}
