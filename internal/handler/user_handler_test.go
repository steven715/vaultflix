package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

func setupUserRouter(userService *service.UserService) *gin.Engine {
	r := gin.New()
	h := NewUserHandler(userService)
	r.GET("/api/users", h.List)
	r.POST("/api/users", h.Create)
	r.DELETE("/api/users/:id", h.Delete)
	r.PUT("/api/users/:id/password", h.ResetPassword)
	return r
}

func TestUserHandler_List(t *testing.T) {
	repo := &mock.UserRepository{
		ListFunc: func(ctx context.Context) ([]model.User, error) {
			return []model.User{
				{ID: "u1", Username: "admin", Role: "admin"},
				{ID: "u2", Username: "viewer1", Role: "viewer"},
			}, nil
		},
	}
	svc := service.NewUserService(repo)
	r := setupUserRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp model.SuccessResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	users, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected data to be array, got %T", resp.Data)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestUserHandler_Create_Success(t *testing.T) {
	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return nil, model.ErrNotFound
		},
		CreateFunc: func(ctx context.Context, user *model.User) error {
			user.ID = "new-id"
			return nil
		},
	}
	svc := service.NewUserService(repo)
	r := setupUserRouter(svc)

	body := `{"username":"newuser","password":"pass123","role":"viewer"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUserHandler_Create_MissingFields(t *testing.T) {
	svc := service.NewUserService(&mock.UserRepository{})
	r := setupUserRouter(svc)

	body := `{"username":"newuser"}`
	req := httptest.NewRequest(http.MethodPost, "/api/users", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUserHandler_Delete_Success(t *testing.T) {
	repo := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Role: "viewer"}, nil
		},
		DisableUserFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}
	svc := service.NewUserService(repo)
	r := setupUserRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/u1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUserHandler_Delete_NotFound(t *testing.T) {
	repo := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return nil, model.ErrNotFound
		},
	}
	svc := service.NewUserService(repo)
	r := setupUserRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUserHandler_ResetPassword_Success(t *testing.T) {
	repo := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
		UpdatePasswordFunc: func(ctx context.Context, id string, hash string) error {
			return nil
		},
	}
	svc := service.NewUserService(repo)
	r := setupUserRouter(svc)

	body := `{"password":"newpassword"}`
	req := httptest.NewRequest(http.MethodPut, "/api/users/u1/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUserHandler_ResetPassword_MissingPassword(t *testing.T) {
	svc := service.NewUserService(&mock.UserRepository{})
	r := setupUserRouter(svc)

	body := `{}`
	req := httptest.NewRequest(http.MethodPut, "/api/users/u1/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
