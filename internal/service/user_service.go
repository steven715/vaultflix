package service

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

var ErrCannotDisableAdmin = errors.New("cannot disable admin account")

type UserService struct {
	userRepo repository.UserRepository
}

func NewUserService(userRepo repository.UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
}

func (s *UserService) List(ctx context.Context) ([]model.User, error) {
	users, err := s.userRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	return users, nil
}

func (s *UserService) Create(ctx context.Context, username, password, role string) (*model.User, error) {
	_, err := s.userRepo.GetByUsername(ctx, username)
	if err == nil {
		return nil, ErrUsernameAlreadyExists
	}
	if !errors.Is(err, model.ErrNotFound) {
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &model.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

func (s *UserService) Disable(ctx context.Context, id string) error {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if user.Role == "admin" {
		return ErrCannotDisableAdmin
	}

	if err := s.userRepo.DisableUser(ctx, id); err != nil {
		return fmt.Errorf("failed to disable user %s: %w", id, err)
	}

	return nil
}

func (s *UserService) ResetPassword(ctx context.Context, id, newPassword string) error {
	_, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := s.userRepo.UpdatePassword(ctx, id, string(hash)); err != nil {
		return fmt.Errorf("failed to update password for user %s: %w", id, err)
	}

	return nil
}
