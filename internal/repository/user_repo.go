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
// DisableUser and EnableUser return model.ErrNotFound when the user does not exist.
// UpdatePassword returns model.ErrNotFound when the user does not exist.
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	GetByID(ctx context.Context, id string) (*model.User, error)
	CountUsers(ctx context.Context) (int64, error)
	List(ctx context.Context) ([]model.User, error)
	DisableUser(ctx context.Context, id string) error
	EnableUser(ctx context.Context, id string) error
	UpdatePassword(ctx context.Context, id string, passwordHash string) error
}

const queryCreateUser = `
    INSERT INTO users (username, password_hash, role)
    VALUES ($1, $2, $3)
    RETURNING id, username, role, disabled_at, created_at, updated_at
`

const queryGetUserByUsername = `
    SELECT id, username, password_hash, role, disabled_at, created_at, updated_at
    FROM users
    WHERE username = $1
`

const queryGetUserByID = `
    SELECT id, username, password_hash, role, disabled_at, created_at, updated_at
    FROM users
    WHERE id = $1
`

const queryCountUsers = `
    SELECT COUNT(*) FROM users
`

const queryListUsers = `
    SELECT id, username, role, disabled_at, created_at, updated_at
    FROM users
    ORDER BY created_at ASC
`

const queryDisableUser = `
    UPDATE users SET disabled_at = NOW(), updated_at = NOW()
    WHERE id = $1 AND disabled_at IS NULL
`

const queryEnableUser = `
    UPDATE users SET disabled_at = NULL, updated_at = NOW()
    WHERE id = $1 AND disabled_at IS NOT NULL
`

const queryUpdatePassword = `
    UPDATE users SET password_hash = $2, updated_at = NOW()
    WHERE id = $1
`

type userRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userRepository{pool: pool}
}

func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	row := r.pool.QueryRow(ctx, queryCreateUser, user.Username, user.PasswordHash, user.Role)

	err := row.Scan(&user.ID, &user.Username, &user.Role, &user.DisabledAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User

	err := r.pool.QueryRow(ctx, queryGetUserByUsername, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.DisabledAt, &user.CreatedAt, &user.UpdatedAt,
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
		&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.DisabledAt, &user.CreatedAt, &user.UpdatedAt,
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

func (r *userRepository) List(ctx context.Context) ([]model.User, error) {
	rows, err := r.pool.Query(ctx, queryListUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.DisabledAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user row: %w", err)
		}
		users = append(users, u)
	}

	return users, nil
}

func (r *userRepository) DisableUser(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDisableUser, id)
	if err != nil {
		return fmt.Errorf("failed to disable user %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *userRepository) EnableUser(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryEnableUser, id)
	if err != nil {
		return fmt.Errorf("failed to enable user %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *userRepository) UpdatePassword(ctx context.Context, id string, passwordHash string) error {
	result, err := r.pool.Exec(ctx, queryUpdatePassword, id, passwordHash)
	if err != nil {
		return fmt.Errorf("failed to update password for user %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}
