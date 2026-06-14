package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

var (
	ErrUsernameAlreadyExists = errors.New("username already exists")
	ErrInvalidCredentials    = errors.New("invalid username or password")
)

type AuthService struct {
	userRepo      repository.UserRepository
	jwtSecret     []byte
	jwtExpHours   int
	streamExpMins int
}

func NewAuthService(userRepo repository.UserRepository, jwtSecret string, jwtExpHours, streamExpMins int) *AuthService {
	return &AuthService{
		userRepo:      userRepo,
		jwtSecret:     []byte(jwtSecret),
		jwtExpHours:   jwtExpHours,
		streamExpMins: streamExpMins,
	}
}

func (s *AuthService) Register(ctx context.Context, username, password, role string) (*model.User, error) {
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

func (s *AuthService) Login(ctx context.Context, username, password string) (string, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return "", ErrInvalidCredentials
		}
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	if user.DisabledAt != nil {
		return "", model.ErrAccountDisabled
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
		"exp":      now.Add(time.Duration(s.jwtExpHours) * time.Hour).Unix(),
		"iat":      now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to sign jwt token: %w", err)
	}

	return tokenString, nil
}

// GenerateStreamToken mints a short-lived, scope-limited token for streaming a
// single video. Unlike the login token it carries scope=stream and the bound
// video_id, and expires after StreamTokenExpiryMinutes. It exists so the full
// login JWT never has to travel in a <video> URL (where it would leak into
// access logs / browser history). Returns the token and its lifetime seconds.
func (s *AuthService) GenerateStreamToken(userID, username, role, videoID string) (string, int, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"role":     role,
		"scope":    model.StreamTokenScope,
		"video_id": videoID,
		"exp":      now.Add(time.Duration(s.streamExpMins) * time.Minute).Unix(),
		"iat":      now.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", 0, fmt.Errorf("failed to sign stream token: %w", err)
	}

	return tokenString, s.streamExpMins * 60, nil
}
