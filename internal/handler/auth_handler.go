package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type registerRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "username and password are required")
		return
	}

	user, err := h.authService.Register(c.Request.Context(), req.Username, req.Password, model.RoleViewer)
	if err != nil {
		writeError(c, err, "register user")
		return
	}

	c.JSON(http.StatusCreated, model.SuccessResponse{
		Data: gin.H{
			"id":       user.ID,
			"username": user.Username,
		},
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "username and password are required")
		return
	}

	token, err := h.authService.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		writeError(c, err, "login")
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{
			"token": token,
		},
	})
}

// StreamToken issues a short-lived, scope-limited token for streaming the video
// identified by the :id path param. The caller is authenticated via the normal
// Authorization header (this route runs behind JWTAuth), so the long-lived
// login token never needs to appear in the streaming URL.
func (h *AuthHandler) StreamToken(c *gin.Context) {
	videoID := c.Param("id")
	userID := c.GetString("user_id")
	username := c.GetString("username")
	role := c.GetString("role")

	token, expiresIn, err := h.authService.GenerateStreamToken(userID, username, role, videoID)
	if err != nil {
		writeError(c, err, "issue stream token")
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{"token": token, "expires_in": expiresIn},
	})
}

func (h *AuthHandler) Me(c *gin.Context) {
	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{
			"id":       c.GetString("user_id"),
			"username": c.GetString("username"),
			"role":     c.GetString("role"),
		},
	})
}
