package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// UserRepository defines the contract for user data access.
// GetByUsername and GetByID return model.ErrNotFound when the user does not exist.
// Create returns model.ErrAlreadyExists when the username is taken.
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	GetByID(ctx context.Context, id string) (*model.User, error)
	CountUsers(ctx context.Context) (int64, error)
}

const queryCreateUser = `
    INSERT INTO users (username, password_hash, role)
    VALUES ($1, $2, $3)
    RETURNING id, username, role, created_at, updated_at
`

const queryGetUserByUsername = `
    SELECT id, username, password_hash, role, created_at, updated_at
    FROM users
    WHERE username = $1
`

const queryGetUserByID = `
    SELECT id, username, password_hash, role, created_at, updated_at
    FROM users
    WHERE id = $1
`

const queryCountUsers = `
    SELECT COUNT(*) FROM users
`

type userRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userRepository{pool: pool}
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	row := r.pool.QueryRow(ctx, queryCreateUser, user.Username, user.PasswordHash, user.Role)

	err := row.Scan(&user.ID, &user.Username, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User

	err := r.pool.QueryRow(ctx, queryGetUserByUsername, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by username %s: %w", username, err)
	}

	return &user, nil
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	var user model.User

	err := r.pool.QueryRow(ctx, queryGetUserByID, id).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by id %s: %w", id, err)
	}

	return &user, nil
}

func (r *userRepository) CountUsers(ctx context.Context) (int64, error) {
	var count int64

	err := r.pool.QueryRow(ctx, queryCountUsers).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}

	return count, nil
}
