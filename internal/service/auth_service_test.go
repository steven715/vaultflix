package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

	svc := NewAuthService(repo, "test-secret", 24, 60)
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

	svc := NewAuthService(repo, "test-secret", 24, 60)
	token, err := svc.Login(context.Background(), "active-user", "password")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)

	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return &model.User{
				ID:           "u1",
				Username:     username,
				PasswordHash: string(hash),
				Role:         "viewer",
			}, nil
		},
	}

	svc := NewAuthService(repo, "test-secret", 24, 60)
	_, err := svc.Login(context.Background(), "user", "wrong-password")

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return nil, model.ErrNotFound
		},
	}

	svc := NewAuthService(repo, "test-secret", 24, 60)
	_, err := svc.Login(context.Background(), "ghost", "password")

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for unknown user, got %v", err)
	}
}

func TestRegister_Success(t *testing.T) {
	var created *model.User
	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return nil, model.ErrNotFound
		},
		CreateFunc: func(ctx context.Context, user *model.User) error {
			created = user
			return nil
		},
	}

	svc := NewAuthService(repo, "test-secret", 24, 60)
	user, err := svc.Register(context.Background(), "newuser", "password", "viewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user == nil || user.Username != "newuser" || user.Role != "viewer" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if created == nil || created.PasswordHash == "" || created.PasswordHash == "password" {
		t.Errorf("expected password to be hashed before create, got %+v", created)
	}
}

func TestGenerateStreamToken(t *testing.T) {
	svc := NewAuthService(&mock.UserRepository{}, "test-secret", 24, 30)

	tokenString, expiresIn, err := svc.GenerateStreamToken("u1", "alice", "viewer", "vid-9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expiresIn != 30*60 {
		t.Errorf("expected expires_in %d, got %d", 30*60, expiresIn)
	}

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("stream token failed to parse/validate: %v", err)
	}
	claims := token.Claims.(jwt.MapClaims)
	if claims["scope"] != model.StreamTokenScope {
		t.Errorf("expected scope %q, got %v", model.StreamTokenScope, claims["scope"])
	}
	if claims["video_id"] != "vid-9" {
		t.Errorf("expected video_id vid-9, got %v", claims["video_id"])
	}
	if claims["user_id"] != "u1" || claims["role"] != "viewer" {
		t.Errorf("unexpected identity claims: %v", claims)
	}
}

func TestRegister_AlreadyExists(t *testing.T) {
	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return &model.User{ID: "u1", Username: username}, nil
		},
	}

	svc := NewAuthService(repo, "test-secret", 24, 60)
	_, err := svc.Register(context.Background(), "taken", "password", "viewer")

	if !errors.Is(err, ErrUsernameAlreadyExists) {
		t.Fatalf("expected ErrUsernameAlreadyExists, got %v", err)
	}
}

func TestResetPassword_Success(t *testing.T) {
	oldHash, _ := bcrypt.GenerateFromPassword([]byte("forgotten"), bcrypt.DefaultCost)

	var gotID, gotHash string
	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return &model.User{
				ID:           "u1",
				Username:     username,
				PasswordHash: string(oldHash),
				Role:         "admin",
			}, nil
		},
		UpdatePasswordFunc: func(ctx context.Context, id string, passwordHash string) error {
			gotID, gotHash = id, passwordHash
			return nil
		},
	}

	svc := NewAuthService(repo, "test-secret", 24, 60)
	if err := svc.ResetPassword(context.Background(), "admin", "brand-new-password"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if gotID != "u1" {
		t.Errorf("expected password update on user u1, got %q", gotID)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(gotHash), []byte("brand-new-password")); err != nil {
		t.Errorf("stored hash does not match the new password: %v", err)
	}
	if gotHash == string(oldHash) {
		t.Error("expected the stored hash to be replaced, got the old one")
	}
}

func TestResetPassword_UnknownUser(t *testing.T) {
	updateCalled := false
	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return nil, model.ErrNotFound
		},
		UpdatePasswordFunc: func(ctx context.Context, id string, passwordHash string) error {
			updateCalled = true
			return nil
		},
	}

	svc := NewAuthService(repo, "test-secret", 24, 60)
	err := svc.ResetPassword(context.Background(), "nobody", "irrelevant")

	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if updateCalled {
		t.Error("expected no password update when the user does not exist")
	}
}

func TestResetPassword_UpdateFails(t *testing.T) {
	repo := &mock.UserRepository{
		GetByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			return &model.User{ID: "u1", Username: username, Role: "admin"}, nil
		},
		UpdatePasswordFunc: func(ctx context.Context, id string, passwordHash string) error {
			return errors.New("db is down")
		},
	}

	svc := NewAuthService(repo, "test-secret", 24, 60)
	if err := svc.ResetPassword(context.Background(), "admin", "new"); err == nil {
		t.Fatal("expected an error when the repository update fails")
	}
}
