package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestUserService_List(t *testing.T) {
	now := time.Now()
	users := []model.User{
		{ID: "u1", Username: "admin", Role: "admin", CreatedAt: now},
		{ID: "u2", Username: "viewer1", Role: "viewer", CreatedAt: now},
	}

	repo := &mock.UserRepository{
		ListFunc: func(ctx context.Context) ([]model.User, error) {
			return users, nil
		},
	}
	svc := NewUserService(repo)

	result, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 users, got %d", len(result))
	}
	if result[0].Username != "admin" {
		t.Errorf("expected first user admin, got %s", result[0].Username)
	}
}

func TestUserService_Create_Success(t *testing.T) {
	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return nil, model.ErrNotFound
		},
		CreateFunc: func(ctx context.Context, user *model.User) error {
			user.ID = "new-id"
			return nil
		},
	}
	svc := NewUserService(repo)

	user, err := svc.Create(context.Background(), "newuser", "password123", "viewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != "new-id" {
		t.Errorf("expected id new-id, got %s", user.ID)
	}
	if user.Username != "newuser" {
		t.Errorf("expected username newuser, got %s", user.Username)
	}
}

func TestUserService_Create_DuplicateUsername(t *testing.T) {
	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return &model.User{ID: "existing"}, nil
		},
	}
	svc := NewUserService(repo)

	_, err := svc.Create(context.Background(), "taken", "pass", "viewer")
	if err != ErrUsernameAlreadyExists {
		t.Fatalf("expected ErrUsernameAlreadyExists, got %v", err)
	}
}

func TestUserService_Disable_Success(t *testing.T) {
	repo := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Role: "viewer"}, nil
		},
		DisableUserFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}
	svc := NewUserService(repo)

	err := svc.Disable(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUserService_Disable_CannotDisableAdmin(t *testing.T) {
	repo := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Role: "admin"}, nil
		},
	}
	svc := NewUserService(repo)

	err := svc.Disable(context.Background(), "admin-id")
	if !errors.Is(err, model.ErrCannotDisableAdmin) {
		t.Fatalf("expected ErrCannotDisableAdmin, got %v", err)
	}
}

func TestUserService_Disable_UserNotFound(t *testing.T) {
	repo := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return nil, model.ErrNotFound
		},
	}
	svc := NewUserService(repo)

	err := svc.Disable(context.Background(), "nope")
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUserService_ResetPassword_Success(t *testing.T) {
	repo := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id}, nil
		},
		UpdatePasswordFunc: func(ctx context.Context, id string, hash string) error {
			if hash == "" {
				t.Error("expected non-empty password hash")
			}
			return nil
		},
	}
	svc := NewUserService(repo)

	err := svc.ResetPassword(context.Background(), "u1", "newpassword")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUserService_ResetPassword_UserNotFound(t *testing.T) {
	repo := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return nil, model.ErrNotFound
		},
	}
	svc := NewUserService(repo)

	err := svc.ResetPassword(context.Background(), "nope", "pass")
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
