package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

type RecommendationHandler struct {
	recSvc service.RecommendationService
}

func NewRecommendationHandler(recSvc service.RecommendationService) *RecommendationHandler {
	return &RecommendationHandler{recSvc: recSvc}
}

func (h *RecommendationHandler) GetToday(c *gin.Context) {
	userID := c.GetString("user_id")

	// Parse date from client (browser local date) with fallback to server UTC date
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if raw := c.Query("date"); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err == nil {
			today = parsed
		}
	}

	fallbackCount := 5
	if raw := c.Query("fallback_count"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 50 {
			badRequest(c, "fallback_count must be between 1 and 50")
			return
		}
		fallbackCount = parsed
	}

	items, err := h.recSvc.GetToday(c.Request.Context(), userID, today, fallbackCount)
	if err != nil {
		writeError(c, err, "get today's recommendations", "user_id", userID)
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: items,
	})
}

func (h *RecommendationHandler) ListByDate(c *gin.Context) {
	dateStr := c.Query("date")
	if dateStr == "" {
		badRequest(c, "date query parameter is required (YYYY-MM-DD)")
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		badRequest(c, "date must be in YYYY-MM-DD format")
		return
	}

	items, err := h.recSvc.ListByDate(c.Request.Context(), date)
	if err != nil {
		writeError(c, err, "list recommendations", "date", dateStr)
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: items,
	})
}

type createRecommendationRequest struct {
	VideoID       string `json:"video_id" binding:"required"`
	RecommendDate string `json:"recommend_date" binding:"required"`
	SortOrder     int    `json:"sort_order"`
}

func (h *RecommendationHandler) Create(c *gin.Context) {
	var req createRecommendationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "video_id and recommend_date are required")
		return
	}

	date, err := time.Parse("2006-01-02", req.RecommendDate)
	if err != nil {
		badRequest(c, "recommend_date must be in YYYY-MM-DD format")
		return
	}

	rec, err := h.recSvc.Create(c.Request.Context(), req.VideoID, date, req.SortOrder)
	if err != nil {
		writeError(c, err, "create recommendation", "video_id", req.VideoID)
		return
	}

	c.JSON(http.StatusCreated, model.SuccessResponse{
		Data: rec,
	})
}

type updateSortOrderRequest struct {
	SortOrder int `json:"sort_order" binding:"required,min=1"`
}

func (h *RecommendationHandler) UpdateSortOrder(c *gin.Context) {
	id := c.Param("id")

	var req updateSortOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "sort_order is required and must be >= 1")
		return
	}

	if err := h.recSvc.UpdateSortOrder(c.Request.Context(), id, req.SortOrder); err != nil {
		writeError(c, err, "update sort order", "recommendation_id", id)
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{Data: gin.H{"sort_order": req.SortOrder}})
}

func (h *RecommendationHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.recSvc.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err, "delete recommendation", "recommendation_id", id)
		return
	}

	c.Status(http.StatusNoContent)
}
