package handler

import (
	"errors"
	"log/slog"
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
		slog.Error("failed to list users", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to list users",
		})
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
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "username, password, and role (admin/viewer) are required",
		})
		return
	}

	user, err := h.userService.Create(c.Request.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		if errors.Is(err, service.ErrUsernameAlreadyExists) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "username already exists",
			})
			return
		}
		slog.Error("failed to create user", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to create user",
		})
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
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "user not found",
			})
			return
		}
		if errors.Is(err, model.ErrCannotDisableAdmin) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: "cannot disable admin account",
			})
			return
		}
		slog.Error("failed to disable user", "user_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to disable user",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *UserHandler) Enable(c *gin.Context) {
	id := c.Param("id")

	err := h.userService.Enable(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "user not found",
			})
			return
		}
		slog.Error("failed to enable user", "user_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to enable user",
		})
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
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "password is required",
		})
		return
	}

	err := h.userService.ResetPassword(c.Request.Context(), id, req.Password)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "user not found",
			})
			return
		}
		slog.Error("failed to reset password", "user_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to reset password",
		})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{"message": "password updated"},
	})
}
