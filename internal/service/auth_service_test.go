package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestLogin_DisabledAccount(t *testing.T) {
	now := time.Now()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)

	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return &model.User{
				ID:           "u1",
				Username:     username,
				PasswordHash: string(hash),
				Role:         "viewer",
				DisabledAt:   &now,
			}, nil
		},
	}

	svc := NewAuthService(repo, "test-secret", 24)
	_, err := svc.Login(context.Background(), "disabled-user", "password")

	if !errors.Is(err, model.ErrAccountDisabled) {
		t.Fatalf("expected ErrAccountDisabled, got %v", err)
	}
}

func TestLogin_ActiveAccount(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)

	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return &model.User{
				ID:           "u1",
				Username:     username,
				PasswordHash: string(hash),
				Role:         "viewer",
				DisabledAt:   nil,
			}, nil
		},
	}

	svc := NewAuthService(repo, "test-secret", 24)
	token, err := svc.Login(context.Background(), "active-user", "password")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}
