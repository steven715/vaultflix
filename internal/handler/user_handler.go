package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

type UserHandler struct {
	userService *service.UserService
}

func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

func (h *UserHandler) List(c *gin.Context) {
	users, err := h.userService.List(c.Request.Context())
	if err != nil {
		writeError(c, err, "list users")
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{Data: users})
}

type createUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required,oneof=admin viewer"`
}

func (h *UserHandler) Create(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "username, password, and role (admin/viewer) are required")
		return
	}

	user, err := h.userService.Create(c.Request.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		writeError(c, err, "create user")
		return
	}

	c.JSON(http.StatusCreated, model.SuccessResponse{
		Data: gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		},
	})
}

func (h *UserHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	err := h.userService.Disable(c.Request.Context(), id)
	if err != nil {
		writeError(c, err, "disable user", "user_id", id)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *UserHandler) Enable(c *gin.Context) {
	id := c.Param("id")

	err := h.userService.Enable(c.Request.Context(), id)
	if err != nil {
		writeError(c, err, "enable user", "user_id", id)
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{"message": "user enabled"},
	})
}

type resetPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	id := c.Param("id")

	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "password is required")
		return
	}

	err := h.userService.ResetPassword(c.Request.Context(), id, req.Password)
	if err != nil {
		writeError(c, err, "reset password", "user_id", id)
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{"message": "password updated"},
	})
}
