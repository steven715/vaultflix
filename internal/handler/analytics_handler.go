package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

const (
	defaultAnalyticsDays  = 30
	minAnalyticsDays      = 1
	maxAnalyticsDays      = 365
	defaultAnalyticsLimit = 10
	minAnalyticsLimit     = 1
	maxAnalyticsLimit     = 50
)

type AnalyticsHandler struct {
	service *service.AnalyticsService
}

func NewAnalyticsHandler(svc *service.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{service: svc}
}

// Get returns the windowed analytics summary. days/limit are request-tunable.
func (h *AnalyticsHandler) Get(c *gin.Context) {
	q := model.AnalyticsQuery{
		Days:  clampParam(c.Query("days"), defaultAnalyticsDays, minAnalyticsDays, maxAnalyticsDays),
		Limit: clampParam(c.Query("limit"), defaultAnalyticsLimit, minAnalyticsLimit, maxAnalyticsLimit),
	}

	summary, err := h.service.Summary(c.Request.Context(), q)
	if err != nil {
		slog.Error("failed to build analytics summary", "error", err, "days", q.Days, "limit", q.Limit)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to build analytics"})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{Data: summary})
}

// clampParam parses raw and clamps to [min,max]; invalid/empty → def.
func clampParam(raw string, def, min, max int) int {
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
