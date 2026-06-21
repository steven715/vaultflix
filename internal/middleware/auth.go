package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/steven/vaultflix/internal/model"
)

// streamRoutePath is the only route a scope=stream token may be used on.
const streamRoutePath = "/api/videos/:id/stream"

func JWTAuth(jwtSecret string) gin.HandlerFunc {
	secret := []byte(jwtSecret)

	return func(c *gin.Context) {
		var tokenString string

		// Priority 1: Authorization header
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// Priority 2: query parameter fallback for contexts where custom headers
		// cannot be set (e.g. <video src>, WebSocket upgrade, SSE, file download).
		// Trade-off: token appears in server access logs and browser history.
		// Acceptable for self-hosted use; production-grade systems should use
		// short-lived tokens or a separate cookie-based auth for streaming.
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "missing token",
			})
			return
		}

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return secret, nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid or expired token",
			})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid or expired token",
			})
			return
		}

		// Scope-limited stream tokens may be used ONLY on the streaming route
		// and ONLY for the video they were issued for. This bounds the damage
		// if such a token leaks via the URL: it cannot reach any other endpoint
		// nor stream a different video, and it expires quickly.
		if scope, _ := claims["scope"].(string); scope == model.StreamTokenScope {
			boundVideoID, _ := claims["video_id"].(string)
			if c.FullPath() != streamRoutePath || boundVideoID == "" || boundVideoID != c.Param("id") {
				c.AbortWithStatusJSON(http.StatusForbidden, model.ErrorResponse{
					Error:   "forbidden",
					Message: "stream token cannot access this resource",
				})
				return
			}
		}

		c.Set("user_id", claims["user_id"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])

		c.Next()
	}
}
