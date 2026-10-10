package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

// Route is one protected API route: how gin registers it and who may call it.
// The route table in cmd/server is the single place that says both, so
// registration, Role checks and the Stream Token's Token Scope can't drift
// (ADR-0013).
type Route struct {
	Method  string
	Path    string
	Handler gin.HandlerFunc
	// Viewer lets the viewer Role call the route; admin may call every route.
	Viewer bool
	// StreamToken lets a Stream Token (Token Scope "stream") call the route,
	// only for the Video it was issued for (the route's :id).
	StreamToken bool
}

// RegisterRoutes registers each route behind its own guard. JWTAuth must run
// before the group so the guard sees the caller's Role and Token Scope.
func RegisterRoutes(g gin.IRoutes, routes []Route) {
	for _, r := range routes {
		g.Handle(r.Method, r.Path, guard(r), r.Handler)
	}
}

// guard enforces a route's Role and Stream Token rules.
func guard(r Route) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(ctxTokenScope) == model.StreamTokenScope {
			// A leaked Stream Token reaches only stream routes, and only the
			// Video it was issued for.
			videoID := c.GetString(ctxTokenVideoID)
			if !r.StreamToken || videoID == "" || videoID != c.Param("id") {
				forbid(c, "stream token cannot access this resource")
				return
			}
		}
		switch c.GetString("role") {
		case model.RoleAdmin:
		case model.RoleViewer:
			if !r.Viewer {
				forbid(c, "insufficient permissions")
				return
			}
		default:
			forbid(c, "insufficient permissions")
			return
		}
		c.Next()
	}
}

func forbid(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusForbidden, model.ErrorResponse{Error: "forbidden", Message: message})
}
