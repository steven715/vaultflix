package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

// UserStatusChecker looks up a user to verify they are still active.
// It returns a non-nil error (e.g. model.ErrNotFound) when the user no longer
// exists. The repository's UserRepository satisfies this contract.
type UserStatusChecker interface {
	GetByID(ctx context.Context, id string) (*model.User, error)
}

// RequireActiveUser rejects requests whose authenticated user has been disabled
// or deleted since their JWT was issued. It must run after JWTAuth, which sets
// "user_id" in the gin context. Without this, a disabled user's already-issued
// token stays valid until natural expiry, defeating account disablement for
// incident response.
//
// Trade-off: this performs one user lookup per authenticated request. For a
// personal-scale deployment the cost is negligible; a higher-traffic system
// should cache disabled user_ids or use short-lived access tokens.
func RequireActiveUser(checker UserStatusChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid token subject",
			})
			return
		}

		user, err := checker.GetByID(c.Request.Context(), userID)
		if err != nil || user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "account no longer valid",
			})
			return
		}

		if user.DisabledAt != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "account is disabled",
			})
			return
		}

		c.Next()
	}
}
