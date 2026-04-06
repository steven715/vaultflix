# User Management Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admin can list, create, soft-delete (disable), and reset password for users.

**Architecture:** Extend existing User model with `disabled_at` column. Add `UserService` for admin-facing user operations (separate from `AuthService` which handles login/register). Add `UserHandler` for admin endpoints. Update `AuthService.Login` to reject disabled accounts. Frontend gets a new `UserManagePage` under `/admin/users`.

**Tech Stack:** Go (Gin, pgx, bcrypt), React 18 + TypeScript, PostgreSQL 16

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/model/user.go` | Add `DisabledAt` field |
| Modify | `internal/model/errors.go` | Add `ErrAccountDisabled` sentinel |
| Create | `migrations/008_add_users_disabled_at.up.sql` | Add `disabled_at` column |
| Create | `migrations/008_add_users_disabled_at.down.sql` | Remove `disabled_at` column |
| Modify | `internal/repository/user_repo.go` | Add `List`, `DisableUser`, `EnableUser`, `UpdatePassword` methods; update queries to include `disabled_at` |
| Create | `internal/mock/user_repo_mock.go` | Mock for `UserRepository` interface |
| Create | `internal/service/user_service.go` | Admin user operations: list, create, disable, reset password |
| Create | `internal/service/user_service_test.go` | Tests for `UserService` |
| Modify | `internal/service/auth_service.go` | Reject disabled accounts on login |
| Create | `internal/handler/user_handler.go` | HTTP handlers for user management endpoints |
| Create | `internal/handler/user_handler_test.go` | Tests for `UserHandler` |
| Modify | `cmd/server/main.go` | Wire `UserService` + `UserHandler`, register routes |
| Modify | `web/src/types/index.ts` | Add `User` type |
| Modify | `web/src/api/admin.ts` | Add user management API functions |
| Create | `web/src/pages/admin/UserManagePage.tsx` | Admin user management page |
| Modify | `web/src/App.tsx` | Add route for `/admin/users` |

---

### Task 1: Database Migration — Add `disabled_at` Column

**Files:**
- Create: `migrations/008_add_users_disabled_at.up.sql`
- Create: `migrations/008_add_users_disabled_at.down.sql`

- [ ] **Step 1: Write up migration**

```sql
-- migrations/008_add_users_disabled_at.up.sql
ALTER TABLE users ADD COLUMN disabled_at TIMESTAMPTZ DEFAULT NULL;
```

- [ ] **Step 2: Write down migration**

```sql
-- migrations/008_add_users_disabled_at.down.sql
ALTER TABLE users DROP COLUMN disabled_at;
```

- [ ] **Step 3: Apply migration to running database**

```bash
docker exec -i vaultflix-postgres-1 psql -U vaultflix -d vaultflix < migrations/008_add_users_disabled_at.up.sql
```

Expected: `ALTER TABLE` with no errors.

- [ ] **Step 4: Commit**

```bash
git add migrations/008_add_users_disabled_at.up.sql migrations/008_add_users_disabled_at.down.sql
git commit -m "feat: add disabled_at column to users table for soft delete"
```

---

### Task 2: Model & Error Updates

**Files:**
- Modify: `internal/model/user.go`
- Modify: `internal/model/errors.go`

- [ ] **Step 1: Add `DisabledAt` field to User model**

In `internal/model/user.go`, add the `DisabledAt` field:

```go
package model

import "time"

type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	DisabledAt   *time.Time `json:"disabled_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
```

`DisabledAt` is `*time.Time` (pointer) — `nil` means active, non-nil means disabled.

- [ ] **Step 2: Add `ErrAccountDisabled` sentinel error**

In `internal/model/errors.go`, add:

```go
var (
	ErrNotFound        = errors.New("resource not found")
	ErrAlreadyExists   = errors.New("resource already exists")
	ErrConflict        = errors.New("resource conflict")
	ErrAccountDisabled = errors.New("account is disabled")
)
```

- [ ] **Step 3: Commit**

```bash
git add internal/model/user.go internal/model/errors.go
git commit -m "feat: add DisabledAt field to User model and ErrAccountDisabled sentinel"
```

---

### Task 3: Repository — Extend `UserRepository` Interface and Implementation

**Files:**
- Modify: `internal/repository/user_repo.go`

- [ ] **Step 1: Update interface with new methods**

Add `List`, `DisableUser`, `EnableUser`, `UpdatePassword` to the `UserRepository` interface:

```go
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
```

- [ ] **Step 2: Update existing queries to include `disabled_at`**

Update the const queries at the top of the file. `queryCreateUser` needs to SELECT `disabled_at`, and `queryGetUserByUsername`/`queryGetUserByID` need to include it:

```go
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
```

- [ ] **Step 3: Add new query constants**

```go
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
```

- [ ] **Step 4: Update existing Scan calls to include `disabled_at`**

In `Create` method:
```go
func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	row := r.pool.QueryRow(ctx, queryCreateUser, user.Username, user.PasswordHash, user.Role)

	err := row.Scan(&user.ID, &user.Username, &user.Role, &user.DisabledAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}
```

In `GetByUsername` method, update Scan:
```go
err := r.pool.QueryRow(ctx, queryGetUserByUsername, username).Scan(
    &user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.DisabledAt, &user.CreatedAt, &user.UpdatedAt,
)
```

In `GetByID` method, update Scan:
```go
err := r.pool.QueryRow(ctx, queryGetUserByID, id).Scan(
    &user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.DisabledAt, &user.CreatedAt, &user.UpdatedAt,
)
```

- [ ] **Step 5: Implement new methods**

```go
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
```

- [ ] **Step 6: Verify compilation**

```bash
docker compose exec api go build ./...
```

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add internal/repository/user_repo.go
git commit -m "feat: extend UserRepository with List, DisableUser, EnableUser, UpdatePassword"
```

---

### Task 4: User Repository Mock

**Files:**
- Create: `internal/mock/user_repo_mock.go`

- [ ] **Step 1: Create mock matching updated interface**

```go
package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

type UserRepository struct {
	CreateFunc         func(ctx context.Context, user *model.User) error
	GetByUsernameFunc  func(ctx context.Context, username string) (*model.User, error)
	GetByIDFunc        func(ctx context.Context, id string) (*model.User, error)
	CountUsersFunc     func(ctx context.Context) (int64, error)
	ListFunc           func(ctx context.Context) ([]model.User, error)
	DisableUserFunc    func(ctx context.Context, id string) error
	EnableUserFunc     func(ctx context.Context, id string) error
	UpdatePasswordFunc func(ctx context.Context, id string, passwordHash string) error
}

func (m *UserRepository) Create(ctx context.Context, user *model.User) error {
	if m.CreateFunc == nil {
		return fmt.Errorf("mock: CreateFunc not set")
	}
	return m.CreateFunc(ctx, user)
}

func (m *UserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	if m.GetByUsernameFunc == nil {
		return nil, fmt.Errorf("mock: GetByUsernameFunc not set")
	}
	return m.GetByUsernameFunc(ctx, username)
}

func (m *UserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	if m.GetByIDFunc == nil {
		return nil, fmt.Errorf("mock: GetByIDFunc not set")
	}
	return m.GetByIDFunc(ctx, id)
}

func (m *UserRepository) CountUsers(ctx context.Context) (int64, error) {
	if m.CountUsersFunc == nil {
		return 0, fmt.Errorf("mock: CountUsersFunc not set")
	}
	return m.CountUsersFunc(ctx)
}

func (m *UserRepository) List(ctx context.Context) ([]model.User, error) {
	if m.ListFunc == nil {
		return nil, fmt.Errorf("mock: ListFunc not set")
	}
	return m.ListFunc(ctx)
}

func (m *UserRepository) DisableUser(ctx context.Context, id string) error {
	if m.DisableUserFunc == nil {
		return fmt.Errorf("mock: DisableUserFunc not set")
	}
	return m.DisableUserFunc(ctx, id)
}

func (m *UserRepository) EnableUser(ctx context.Context, id string) error {
	if m.EnableUserFunc == nil {
		return fmt.Errorf("mock: EnableUserFunc not set")
	}
	return m.EnableUserFunc(ctx, id)
}

func (m *UserRepository) UpdatePassword(ctx context.Context, id string, passwordHash string) error {
	if m.UpdatePasswordFunc == nil {
		return fmt.Errorf("mock: UpdatePasswordFunc not set")
	}
	return m.UpdatePasswordFunc(ctx, id, passwordHash)
}
```

- [ ] **Step 2: Verify compilation**

```bash
docker compose exec api go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/mock/user_repo_mock.go
git commit -m "feat: add UserRepository mock"
```

---

### Task 5: UserService — Business Logic

**Files:**
- Create: `internal/service/user_service.go`
- Create: `internal/service/user_service_test.go`

- [ ] **Step 1: Write failing tests for UserService**

```go
// internal/service/user_service_test.go
package service

import (
	"context"
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
	if err != ErrCannotDisableAdmin {
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
	if err != model.ErrNotFound {
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
	if err != model.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
docker compose exec api go test ./internal/service/ -run TestUserService -v
```

Expected: compilation error — `NewUserService` not defined.

- [ ] **Step 3: Implement UserService**

```go
// internal/service/user_service.go
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
docker compose exec api go test ./internal/service/ -run TestUserService -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/user_service.go internal/service/user_service_test.go
git commit -m "feat: add UserService with list, create, disable, reset password"
```

---

### Task 6: AuthService — Reject Disabled Accounts on Login

**Files:**
- Modify: `internal/service/auth_service.go`

- [ ] **Step 1: Add test for disabled account login rejection**

Append to an existing test file or create `internal/service/auth_service_test.go`:

```go
// internal/service/auth_service_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
docker compose exec api go test ./internal/service/ -run TestLogin_DisabledAccount -v
```

Expected: FAIL — disabled account check not implemented yet.

- [ ] **Step 3: Update `Login` method to check `DisabledAt`**

In `internal/service/auth_service.go`, update the `Login` method to check for disabled accounts after finding the user and before checking the password:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
docker compose exec api go test ./internal/service/ -run TestLogin -v
```

Expected: both `TestLogin_DisabledAccount` and `TestLogin_ActiveAccount` PASS.

- [ ] **Step 5: Update `AuthHandler.Login` to return 403 for disabled accounts**

In `internal/handler/auth_handler.go`, update the Login error handling to detect `ErrAccountDisabled`:

```go
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "username and password are required",
		})
		return
	}

	token, err := h.authService.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid username or password",
			})
			return
		}
		if errors.Is(err, model.ErrAccountDisabled) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{
				Error:   "forbidden",
				Message: "account is disabled",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to login",
		})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{
			"token": token,
		},
	})
}
```

- [ ] **Step 6: Verify compilation**

```bash
docker compose exec api go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add internal/service/auth_service.go internal/service/auth_service_test.go internal/handler/auth_handler.go
git commit -m "feat: reject disabled accounts on login with clear error message"
```

---

### Task 7: UserHandler — HTTP Endpoints

**Files:**
- Create: `internal/handler/user_handler.go`
- Create: `internal/handler/user_handler_test.go`

- [ ] **Step 1: Write failing tests for UserHandler**

```go
// internal/handler/user_handler_test.go
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
docker compose exec api go test ./internal/handler/ -run TestUserHandler -v
```

Expected: compilation error — `NewUserHandler` not defined.

- [ ] **Step 3: Implement UserHandler**

```go
// internal/handler/user_handler.go
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
		if errors.Is(err, service.ErrCannotDisableAdmin) {
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
docker compose exec api go test ./internal/handler/ -run TestUserHandler -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/user_handler.go internal/handler/user_handler_test.go
git commit -m "feat: add UserHandler with list, create, delete, reset password endpoints"
```

---

### Task 8: Wire Everything in `main.go`

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Add UserService, UserHandler, and register routes**

In `cmd/server/main.go`, add the following after the existing service/handler initialization:

After `authService` line:
```go
userService := service.NewUserService(userRepo)
```

After `recHandler` line:
```go
userHandler := handler.NewUserHandler(userService)
```

In the protected routes block, add user management endpoints:
```go
// User management endpoints (admin only, enforced by Casbin)
api.GET("/users", userHandler.List)
api.POST("/users", userHandler.Create)
api.DELETE("/users/:id", userHandler.Delete)
api.PUT("/users/:id/password", userHandler.ResetPassword)
```

- [ ] **Step 2: Verify compilation**

```bash
docker compose exec api go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire user management service and routes in main.go"
```

---

### Task 9: Frontend — Types and API Client

**Files:**
- Modify: `web/src/types/index.ts`
- Modify: `web/src/api/admin.ts`

- [ ] **Step 1: Add `User` type**

In `web/src/types/index.ts`, add at the end:

```typescript
export interface User {
  id: string
  username: string
  role: string
  disabled_at: string | null
  created_at: string
  updated_at: string
}
```

- [ ] **Step 2: Add user management API functions**

In `web/src/api/admin.ts`, add the import and functions:

Add `User` to the import:
```typescript
import type {
  ImportResult,
  Video,
  Tag,
  DailyRecommendation,
  RecommendationItem,
  User,
} from '../types'
```

Add the functions at the end of the file:

```typescript
export async function listUsers(): Promise<User[]> {
  const res = await client.get<User[]>('/users')
  return res.data
}

export async function createUser(username: string, password: string, role: string): Promise<User> {
  const res = await client.post<User>('/users', { username, password, role })
  return res.data
}

export async function deleteUser(id: string): Promise<void> {
  await client.delete(`/users/${id}`)
}

export async function resetUserPassword(id: string, password: string): Promise<void> {
  await client.put(`/users/${id}/password`, { password })
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/types/index.ts web/src/api/admin.ts
git commit -m "feat: add User type and admin API functions for user management"
```

---

### Task 10: Frontend — UserManagePage

**Files:**
- Create: `web/src/pages/admin/UserManagePage.tsx`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Create UserManagePage component**

```tsx
// web/src/pages/admin/UserManagePage.tsx
import { useState, useEffect } from 'react'
import { listUsers, createUser, deleteUser, resetUserPassword } from '../../api/admin'
import type { User } from '../../types'
import Header from '../../components/Header'

export default function UserManagePage() {
  const [users, setUsers] = useState<User[]>([])
  const [loading, setLoading] = useState(true)

  // Create user form
  const [showCreate, setShowCreate] = useState(false)
  const [newUsername, setNewUsername] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [newRole, setNewRole] = useState('viewer')
  const [creating, setCreating] = useState(false)

  // Reset password modal
  const [resetTarget, setResetTarget] = useState<User | null>(null)
  const [resetPass, setResetPass] = useState('')
  const [resetting, setResetting] = useState(false)

  // Delete confirm
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null)

  useEffect(() => {
    let cancelled = false
    const fetchUsers = async () => {
      try {
        const data = await listUsers()
        if (!cancelled) setUsers(data)
      } catch (err) {
        console.error('failed to list users', err)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    fetchUsers()
    return () => { cancelled = true }
  }, [])

  const handleCreate = async () => {
    if (!newUsername || !newPassword) return
    setCreating(true)
    try {
      await createUser(newUsername, newPassword, newRole)
      const data = await listUsers()
      setUsers(data)
      setShowCreate(false)
      setNewUsername('')
      setNewPassword('')
      setNewRole('viewer')
    } catch (err) {
      alert(err instanceof Error ? err.message : 'failed to create user')
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await deleteUser(id)
      setUsers(users.map(u => u.id === id ? { ...u, disabled_at: new Date().toISOString() } : u))
      setDeleteTarget(null)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'failed to disable user')
    }
  }

  const handleResetPassword = async () => {
    if (!resetTarget || !resetPass) return
    setResetting(true)
    try {
      await resetUserPassword(resetTarget.id, resetPass)
      setResetTarget(null)
      setResetPass('')
      alert('password updated')
    } catch (err) {
      alert(err instanceof Error ? err.message : 'failed to reset password')
    } finally {
      setResetting(false)
    }
  }

  return (
    <div style={{ minHeight: '100vh', backgroundColor: '#0f0f0f', color: '#fff' }}>
      <Header />
      <div style={{ maxWidth: 900, margin: '0 auto', padding: '2rem 1rem' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
          <h1 style={{ fontSize: '1.5rem', margin: 0 }}>User Management</h1>
          <button
            onClick={() => setShowCreate(true)}
            style={{ padding: '0.5rem 1rem', backgroundColor: '#e50914', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
          >
            Create User
          </button>
        </div>

        {loading ? (
          <p>Loading...</p>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #333', textAlign: 'left' }}>
                <th style={{ padding: '0.75rem' }}>Username</th>
                <th style={{ padding: '0.75rem' }}>Role</th>
                <th style={{ padding: '0.75rem' }}>Status</th>
                <th style={{ padding: '0.75rem' }}>Created</th>
                <th style={{ padding: '0.75rem' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.map(user => (
                <tr key={user.id} style={{ borderBottom: '1px solid #222' }}>
                  <td style={{ padding: '0.75rem' }}>{user.username}</td>
                  <td style={{ padding: '0.75rem' }}>
                    <span style={{
                      padding: '0.2rem 0.5rem',
                      borderRadius: 4,
                      fontSize: '0.85rem',
                      backgroundColor: user.role === 'admin' ? '#b45309' : '#1e40af',
                    }}>
                      {user.role}
                    </span>
                  </td>
                  <td style={{ padding: '0.75rem' }}>
                    {user.disabled_at ? (
                      <span style={{ color: '#ef4444' }}>Disabled</span>
                    ) : (
                      <span style={{ color: '#22c55e' }}>Active</span>
                    )}
                  </td>
                  <td style={{ padding: '0.75rem' }}>{new Date(user.created_at).toLocaleDateString()}</td>
                  <td style={{ padding: '0.75rem' }}>
                    <button
                      onClick={() => setResetTarget(user)}
                      style={{ marginRight: '0.5rem', padding: '0.3rem 0.6rem', backgroundColor: '#333', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                    >
                      Reset Password
                    </button>
                    {user.role !== 'admin' && !user.disabled_at && (
                      <button
                        onClick={() => setDeleteTarget(user)}
                        style={{ padding: '0.3rem 0.6rem', backgroundColor: '#7f1d1d', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                      >
                        Disable
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        {/* Create User Modal */}
        {showCreate && (
          <div style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50 }}>
            <div style={{ backgroundColor: '#1a1a1a', padding: '2rem', borderRadius: 8, width: 400 }}>
              <h2 style={{ marginTop: 0 }}>Create User</h2>
              <div style={{ marginBottom: '1rem' }}>
                <label style={{ display: 'block', marginBottom: '0.3rem', fontSize: '0.9rem' }}>Username</label>
                <input
                  value={newUsername}
                  onChange={e => setNewUsername(e.target.value)}
                  style={{ width: '100%', padding: '0.5rem', backgroundColor: '#333', color: '#fff', border: '1px solid #555', borderRadius: 4, boxSizing: 'border-box' }}
                />
              </div>
              <div style={{ marginBottom: '1rem' }}>
                <label style={{ display: 'block', marginBottom: '0.3rem', fontSize: '0.9rem' }}>Password</label>
                <input
                  type="password"
                  value={newPassword}
                  onChange={e => setNewPassword(e.target.value)}
                  style={{ width: '100%', padding: '0.5rem', backgroundColor: '#333', color: '#fff', border: '1px solid #555', borderRadius: 4, boxSizing: 'border-box' }}
                />
              </div>
              <div style={{ marginBottom: '1.5rem' }}>
                <label style={{ display: 'block', marginBottom: '0.3rem', fontSize: '0.9rem' }}>Role</label>
                <select
                  value={newRole}
                  onChange={e => setNewRole(e.target.value)}
                  style={{ width: '100%', padding: '0.5rem', backgroundColor: '#333', color: '#fff', border: '1px solid #555', borderRadius: 4 }}
                >
                  <option value="viewer">Viewer</option>
                  <option value="admin">Admin</option>
                </select>
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem' }}>
                <button
                  onClick={() => setShowCreate(false)}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#333', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleCreate}
                  disabled={creating || !newUsername || !newPassword}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#e50914', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', opacity: creating ? 0.5 : 1 }}
                >
                  {creating ? 'Creating...' : 'Create'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Reset Password Modal */}
        {resetTarget && (
          <div style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50 }}>
            <div style={{ backgroundColor: '#1a1a1a', padding: '2rem', borderRadius: 8, width: 400 }}>
              <h2 style={{ marginTop: 0 }}>Reset Password</h2>
              <p style={{ color: '#aaa' }}>User: {resetTarget.username}</p>
              <div style={{ marginBottom: '1.5rem' }}>
                <label style={{ display: 'block', marginBottom: '0.3rem', fontSize: '0.9rem' }}>New Password</label>
                <input
                  type="password"
                  value={resetPass}
                  onChange={e => setResetPass(e.target.value)}
                  style={{ width: '100%', padding: '0.5rem', backgroundColor: '#333', color: '#fff', border: '1px solid #555', borderRadius: 4, boxSizing: 'border-box' }}
                />
              </div>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem' }}>
                <button
                  onClick={() => { setResetTarget(null); setResetPass('') }}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#333', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleResetPassword}
                  disabled={resetting || !resetPass}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#e50914', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', opacity: resetting ? 0.5 : 1 }}
                >
                  {resetting ? 'Updating...' : 'Update Password'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Delete Confirm Modal */}
        {deleteTarget && (
          <div style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50 }}>
            <div style={{ backgroundColor: '#1a1a1a', padding: '2rem', borderRadius: 8, width: 400 }}>
              <h2 style={{ marginTop: 0 }}>Confirm Disable</h2>
              <p>Are you sure you want to disable <strong>{deleteTarget.username}</strong>?</p>
              <p style={{ color: '#aaa', fontSize: '0.9rem' }}>The user will not be able to log in. Their data (favorites, watch history) will be preserved.</p>
              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem' }}>
                <button
                  onClick={() => setDeleteTarget(null)}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#333', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                >
                  Cancel
                </button>
                <button
                  onClick={() => handleDelete(deleteTarget.id)}
                  style={{ padding: '0.5rem 1rem', backgroundColor: '#dc2626', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer' }}
                >
                  Disable User
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Add route in App.tsx**

In `web/src/App.tsx`, add the import and route:

Add import:
```typescript
import UserManagePage from './pages/admin/UserManagePage'
```

Add route inside the AdminRoute children, after the recommendations route:
```typescript
{ path: '/admin/users', element: <UserManagePage /> },
```

- [ ] **Step 3: Add navigation link in Header**

Check the Header component for existing admin nav links and add a "Users" link pointing to `/admin/users`. (Follow the same pattern as existing admin links.)

- [ ] **Step 4: Verify frontend builds**

```bash
docker compose exec frontend npm run build
```

Expected: build succeeds with no TypeScript errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/admin/UserManagePage.tsx web/src/App.tsx web/src/components/Header.tsx
git commit -m "feat: add UserManagePage with create, disable, and reset password UI"
```

---

### Task 11: End-to-End Verification

- [ ] **Step 1: Rebuild and restart containers**

```bash
docker compose up --build -d
```

- [ ] **Step 2: Apply migration**

```bash
docker exec -i vaultflix-postgres-1 psql -U vaultflix -d vaultflix < migrations/008_add_users_disabled_at.up.sql
```

- [ ] **Step 3: Run all backend tests**

```bash
docker compose exec api go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 4: Manual verification via browser**

1. Login as admin
2. Navigate to `/admin/users`
3. Verify user list shows existing users
4. Create a new viewer user
5. Reset the new user's password
6. Disable the new user
7. Attempt to login as the disabled user — expect "account is disabled" error
8. Verify admin cannot be disabled (button not shown)

- [ ] **Step 5: Commit any fixes if needed**
