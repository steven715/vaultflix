package middleware

import (
	"net/http"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

func CasbinRBAC(enforcer *casbin.Enforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, model.ErrorResponse{
				Error:   "forbidden",
				Message: "insufficient permissions",
			})
			return
		}

		allowed, err := enforcer.Enforce(role, c.Request.URL.Path, c.Request.Method)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to check permissions",
			})
			return
		}

		if !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, model.ErrorResponse{
				Error:   "forbidden",
				Message: "insufficient permissions",
			})
			return
		}

		c.Next()
	}
}
