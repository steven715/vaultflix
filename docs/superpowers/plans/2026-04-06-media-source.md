# Phase 8: Media Source 管理 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build CRUD API for managing media sources (local disk directories mounted into the container), including path validation, so that Admin users can configure where videos live on disk.

**Architecture:** New `media_sources` table with CRUD through the standard Handler → Service → Repository layered architecture. Service layer validates mount paths (prefix check + filesystem existence). Videos table gets `source_id` FK and `file_path` column for Phase 9.

**Tech Stack:** Go 1.22+, Gin, pgx/v5, PostgreSQL 16, Casbin

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `migrations/009_create_media_sources.up.sql` | Create media_sources table, add columns to videos |
| Create | `migrations/009_create_media_sources.down.sql` | Reverse migration |
| Create | `internal/model/media_source.go` | MediaSource struct |
| Modify | `internal/model/errors.go` | Add ErrPathNotAllowed, ErrPathNotExist |
| Create | `internal/repository/media_source_repo.go` | Interface + pgxpool implementation |
| Create | `internal/mock/media_source_repo_mock.go` | Mock for testing |
| Create | `internal/service/media_source_service.go` | Business logic + path validation |
| Create | `internal/service/media_source_service_test.go` | Service unit tests |
| Create | `internal/handler/media_source_handler.go` | HTTP endpoints |
| Create | `internal/handler/media_source_handler_test.go` | Handler unit tests |
| Modify | `cmd/server/main.go` | DI wiring + route registration |

---

### Task 1: Migration

**Files:**
- Create: `migrations/009_create_media_sources.up.sql`
- Create: `migrations/009_create_media_sources.down.sql`

- [ ] **Step 1: Write up migration**

```sql
CREATE TABLE media_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    label VARCHAR(255) NOT NULL,
    mount_path VARCHAR(1024) NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE videos
    ADD COLUMN source_id UUID REFERENCES media_sources(id) ON DELETE SET NULL,
    ADD COLUMN file_path VARCHAR(2048);

COMMENT ON COLUMN videos.source_id IS '影片所屬的 media source，source 刪除時設為 NULL';
COMMENT ON COLUMN videos.file_path IS '相對於 media source mount_path 的檔案路徑';
COMMENT ON COLUMN videos.minio_object_key IS '舊欄位，重構完成後不再使用，僅用於遷移期間';
```

- [ ] **Step 2: Write down migration**

```sql
ALTER TABLE videos DROP COLUMN IF EXISTS file_path;
ALTER TABLE videos DROP COLUMN IF EXISTS source_id;
DROP TABLE IF EXISTS media_sources;
```

Note: `source_id` must be dropped before `file_path` because `source_id` has a FK constraint referencing `media_sources`. The table drop must come after the column drops.

- [ ] **Step 3: Commit**

```bash
git add migrations/009_create_media_sources.up.sql migrations/009_create_media_sources.down.sql
git commit -m "feat: add migration 009 for media_sources table and videos columns"
```

---

### Task 2: Model + Sentinel Errors

**Files:**
- Create: `internal/model/media_source.go`
- Modify: `internal/model/errors.go`

- [ ] **Step 1: Create MediaSource model**

Create `internal/model/media_source.go`:

```go
package model

import "time"

type MediaSource struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	MountPath string    `json:"mount_path"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
```

- [ ] **Step 2: Add sentinel errors to `internal/model/errors.go`**

Add these two errors after the existing ones:

```go
ErrPathNotAllowed = errors.New("path is not within allowed mount prefix")
ErrPathNotExist   = errors.New("path does not exist on filesystem")
```

The full var block becomes:

```go
var (
	ErrNotFound           = errors.New("resource not found")
	ErrAlreadyExists      = errors.New("resource already exists")
	ErrConflict           = errors.New("resource conflict")
	ErrAccountDisabled    = errors.New("account is disabled")
	ErrCannotDisableAdmin = errors.New("cannot disable admin account")
	ErrPathNotAllowed     = errors.New("path is not within allowed mount prefix")
	ErrPathNotExist       = errors.New("path does not exist on filesystem")
)
```

- [ ] **Step 3: Commit**

```bash
git add internal/model/media_source.go internal/model/errors.go
git commit -m "feat: add MediaSource model and path sentinel errors"
```

---

### Task 3: Repository (Interface + Implementation + Mock)

**Files:**
- Create: `internal/repository/media_source_repo.go`
- Create: `internal/mock/media_source_repo_mock.go`

- [ ] **Step 1: Create repository with interface + implementation**

Create `internal/repository/media_source_repo.go`:

```go
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// MediaSourceRepository manages media source persistence.
//
// List returns all media sources (no pagination, expected count is small).
// FindByID returns model.ErrNotFound when the source does not exist.
// Create returns model.ErrAlreadyExists when mount_path is duplicated.
// Update returns model.ErrNotFound when the source does not exist.
// Delete returns model.ErrNotFound when the source does not exist.
type MediaSourceRepository interface {
	List(ctx context.Context) ([]model.MediaSource, error)
	FindByID(ctx context.Context, id string) (*model.MediaSource, error)
	Create(ctx context.Context, source *model.MediaSource) error
	Update(ctx context.Context, source *model.MediaSource) error
	Delete(ctx context.Context, id string) error
}

const queryListMediaSources = `
    SELECT id, label, mount_path, enabled, created_at, updated_at
    FROM media_sources
    ORDER BY created_at
`

const queryFindMediaSourceByID = `
    SELECT id, label, mount_path, enabled, created_at, updated_at
    FROM media_sources
    WHERE id = $1
`

const queryCreateMediaSource = `
    INSERT INTO media_sources (label, mount_path)
    VALUES ($1, $2)
    RETURNING id, enabled, created_at, updated_at
`

const queryUpdateMediaSource = `
    UPDATE media_sources
    SET label = $1, enabled = $2, updated_at = NOW()
    WHERE id = $3
`

const queryDeleteMediaSource = `
    DELETE FROM media_sources WHERE id = $1
`

type mediaSourceRepository struct {
	pool *pgxpool.Pool
}

func NewMediaSourceRepository(pool *pgxpool.Pool) MediaSourceRepository {
	return &mediaSourceRepository{pool: pool}
}

func (r *mediaSourceRepository) List(ctx context.Context) ([]model.MediaSource, error) {
	rows, err := r.pool.Query(ctx, queryListMediaSources)
	if err != nil {
		return nil, fmt.Errorf("failed to list media sources: %w", err)
	}
	defer rows.Close()

	var sources []model.MediaSource
	for rows.Next() {
		var s model.MediaSource
		if err := rows.Scan(&s.ID, &s.Label, &s.MountPath, &s.Enabled, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan media source: %w", err)
		}
		sources = append(sources, s)
	}

	if sources == nil {
		sources = []model.MediaSource{}
	}

	return sources, nil
}

func (r *mediaSourceRepository) FindByID(ctx context.Context, id string) (*model.MediaSource, error) {
	var s model.MediaSource
	err := r.pool.QueryRow(ctx, queryFindMediaSourceByID, id).Scan(
		&s.ID, &s.Label, &s.MountPath, &s.Enabled, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("failed to find media source %s: %w", id, err)
	}
	return &s, nil
}

func (r *mediaSourceRepository) Create(ctx context.Context, source *model.MediaSource) error {
	err := r.pool.QueryRow(ctx, queryCreateMediaSource, source.Label, source.MountPath).Scan(
		&source.ID, &source.Enabled, &source.CreatedAt, &source.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create media source: %w", err)
	}
	return nil
}

func (r *mediaSourceRepository) Update(ctx context.Context, source *model.MediaSource) error {
	result, err := r.pool.Exec(ctx, queryUpdateMediaSource, source.Label, source.Enabled, source.ID)
	if err != nil {
		return fmt.Errorf("failed to update media source %s: %w", source.ID, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *mediaSourceRepository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDeleteMediaSource, id)
	if err != nil {
		return fmt.Errorf("failed to delete media source %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}
```

- [ ] **Step 2: Create mock**

Create `internal/mock/media_source_repo_mock.go`:

```go
package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

type MediaSourceRepository struct {
	ListFunc     func(ctx context.Context) ([]model.MediaSource, error)
	FindByIDFunc func(ctx context.Context, id string) (*model.MediaSource, error)
	CreateFunc   func(ctx context.Context, source *model.MediaSource) error
	UpdateFunc   func(ctx context.Context, source *model.MediaSource) error
	DeleteFunc   func(ctx context.Context, id string) error
}

func (m *MediaSourceRepository) List(ctx context.Context) ([]model.MediaSource, error) {
	if m.ListFunc == nil {
		return nil, fmt.Errorf("mock: ListFunc not set")
	}
	return m.ListFunc(ctx)
}

func (m *MediaSourceRepository) FindByID(ctx context.Context, id string) (*model.MediaSource, error) {
	if m.FindByIDFunc == nil {
		return nil, fmt.Errorf("mock: FindByIDFunc not set")
	}
	return m.FindByIDFunc(ctx, id)
}

func (m *MediaSourceRepository) Create(ctx context.Context, source *model.MediaSource) error {
	if m.CreateFunc == nil {
		return fmt.Errorf("mock: CreateFunc not set")
	}
	return m.CreateFunc(ctx, source)
}

func (m *MediaSourceRepository) Update(ctx context.Context, source *model.MediaSource) error {
	if m.UpdateFunc == nil {
		return fmt.Errorf("mock: UpdateFunc not set")
	}
	return m.UpdateFunc(ctx, source)
}

func (m *MediaSourceRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc == nil {
		return fmt.Errorf("mock: DeleteFunc not set")
	}
	return m.DeleteFunc(ctx, id)
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/repository/media_source_repo.go internal/mock/media_source_repo_mock.go
git commit -m "feat: add MediaSourceRepository interface, implementation, and mock"
```

---

### Task 4: Service + Tests

**Files:**
- Create: `internal/service/media_source_service.go`
- Create: `internal/service/media_source_service_test.go`

- [ ] **Step 1: Write service tests**

Create `internal/service/media_source_service_test.go`:

```go
package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

// helper: create a temp dir structure for path validation tests.
// Returns the mount prefix (e.g., "/tmp/xxx/mnt/host/") and a valid subdir path.
func setupTempMount(t *testing.T) (prefix string, validPath string) {
	t.Helper()
	base := t.TempDir()
	mountHost := filepath.Join(base, "mnt", "host")
	subDir := filepath.Join(mountHost, "Videos")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	// prefix must end with separator to match filepath.Clean behavior
	return mountHost + string(filepath.Separator), subDir
}

func TestCreateMediaSource_Success(t *testing.T) {
	prefix, validPath := setupTempMount(t)

	repo := &mock.MediaSourceRepository{
		CreateFunc: func(ctx context.Context, source *model.MediaSource) error {
			source.ID = "ms-1"
			return nil
		},
	}
	svc := NewMediaSourceService(repo, prefix)

	source := &model.MediaSource{Label: "Videos", MountPath: validPath}
	err := svc.Create(context.Background(), source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source.ID != "ms-1" {
		t.Errorf("expected id ms-1, got %s", source.ID)
	}
}

func TestCreateMediaSource_PathOutsidePrefix(t *testing.T) {
	prefix, _ := setupTempMount(t)

	svc := NewMediaSourceService(&mock.MediaSourceRepository{}, prefix)

	source := &model.MediaSource{Label: "Bad", MountPath: "/etc/passwd"}
	err := svc.Create(context.Background(), source)
	if !errors.Is(err, model.ErrPathNotAllowed) {
		t.Fatalf("expected ErrPathNotAllowed, got %v", err)
	}
}

func TestCreateMediaSource_PathContainsDotDot(t *testing.T) {
	prefix, validPath := setupTempMount(t)

	svc := NewMediaSourceService(&mock.MediaSourceRepository{}, prefix)

	// path with .. that resolves outside prefix
	dotdotPath := validPath + "/../../../etc"
	source := &model.MediaSource{Label: "Bad", MountPath: dotdotPath}
	err := svc.Create(context.Background(), source)
	if !errors.Is(err, model.ErrPathNotAllowed) {
		t.Fatalf("expected ErrPathNotAllowed, got %v", err)
	}
}

func TestCreateMediaSource_PathNotExist(t *testing.T) {
	prefix, _ := setupTempMount(t)

	svc := NewMediaSourceService(&mock.MediaSourceRepository{}, prefix)

	source := &model.MediaSource{Label: "Gone", MountPath: filepath.Join(prefix, "nonexistent")}
	err := svc.Create(context.Background(), source)
	if !errors.Is(err, model.ErrPathNotExist) {
		t.Fatalf("expected ErrPathNotExist, got %v", err)
	}
}

func TestCreateMediaSource_PathNotDirectory(t *testing.T) {
	prefix, validPath := setupTempMount(t)

	// create a file inside the valid dir
	filePath := filepath.Join(validPath, "video.mp4")
	if err := os.WriteFile(filePath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	svc := NewMediaSourceService(&mock.MediaSourceRepository{}, prefix)

	source := &model.MediaSource{Label: "File", MountPath: filePath}
	err := svc.Create(context.Background(), source)
	if !errors.Is(err, model.ErrPathNotAllowed) {
		t.Fatalf("expected ErrPathNotAllowed, got %v", err)
	}
}

func TestCreateMediaSource_DuplicatePath(t *testing.T) {
	prefix, validPath := setupTempMount(t)

	repo := &mock.MediaSourceRepository{
		CreateFunc: func(ctx context.Context, source *model.MediaSource) error {
			return model.ErrAlreadyExists
		},
	}
	svc := NewMediaSourceService(repo, prefix)

	source := &model.MediaSource{Label: "Dup", MountPath: validPath}
	err := svc.Create(context.Background(), source)
	if !errors.Is(err, model.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestUpdateMediaSource_Success(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return &model.MediaSource{ID: id, Label: "Old", MountPath: "/mnt/host/X", Enabled: true}, nil
		},
		UpdateFunc: func(ctx context.Context, source *model.MediaSource) error {
			return nil
		},
	}
	svc := NewMediaSourceService(repo, "/mnt/host/")

	err := svc.Update(context.Background(), "ms-1", "New Label", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateMediaSource_NotFound(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return nil, model.ErrNotFound
		},
	}
	svc := NewMediaSourceService(repo, "/mnt/host/")

	err := svc.Update(context.Background(), "nope", "Label", true)
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteMediaSource_Success(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}
	svc := NewMediaSourceService(repo, "/mnt/host/")

	err := svc.Delete(context.Background(), "ms-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteMediaSource_NotFound(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			return model.ErrNotFound
		},
	}
	svc := NewMediaSourceService(repo, "/mnt/host/")

	err := svc.Delete(context.Background(), "nope")
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `docker compose exec vaultflix-api go test ./internal/service/ -run TestCreateMediaSource -v 2>&1 | head -30`

Expected: compilation error — `NewMediaSourceService` not defined.

**Important:** All `go test` commands must run inside the `vaultflix-api` container via `docker compose exec`. Do NOT run `go test` on the host.

- [ ] **Step 3: Write service implementation**

Create `internal/service/media_source_service.go`:

```go
package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// AllowedMountPrefix is the production mount prefix for media sources.
const AllowedMountPrefix = "/mnt/host/"

type MediaSourceService struct {
	repo        repository.MediaSourceRepository
	mountPrefix string
}

func NewMediaSourceService(repo repository.MediaSourceRepository, mountPrefix string) *MediaSourceService {
	return &MediaSourceService{repo: repo, mountPrefix: mountPrefix}
}

// ValidateMountPath validates that path is within the allowed mount prefix,
// contains no path traversal components, exists on the filesystem, and is a directory.
func (s *MediaSourceService) ValidateMountPath(path string) error {
	cleaned := filepath.Clean(path)
	if !strings.HasPrefix(cleaned, strings.TrimSuffix(s.mountPrefix, string(filepath.Separator))) {
		return model.ErrPathNotAllowed
	}
	if cleaned != path {
		return fmt.Errorf("path contains invalid components: %w", model.ErrPathNotAllowed)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return fmt.Errorf("%s: %w", cleaned, model.ErrPathNotExist)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %w", cleaned, model.ErrPathNotAllowed)
	}
	return nil
}

func (s *MediaSourceService) List(ctx context.Context) ([]model.MediaSource, error) {
	sources, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list media sources: %w", err)
	}
	return sources, nil
}

func (s *MediaSourceService) GetByID(ctx context.Context, id string) (*model.MediaSource, error) {
	source, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get media source %s: %w", id, err)
	}
	return source, nil
}

func (s *MediaSourceService) Create(ctx context.Context, source *model.MediaSource) error {
	if err := s.ValidateMountPath(source.MountPath); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, source); err != nil {
		return fmt.Errorf("failed to create media source: %w", err)
	}
	return nil
}

func (s *MediaSourceService) Update(ctx context.Context, id string, label string, enabled bool) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get media source %s: %w", id, err)
	}
	existing.Label = label
	existing.Enabled = enabled
	if err := s.repo.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update media source %s: %w", id, err)
	}
	return nil
}

func (s *MediaSourceService) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete media source %s: %w", id, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `docker compose exec vaultflix-api go test ./internal/service/ -run "TestCreateMediaSource|TestUpdateMediaSource|TestDeleteMediaSource" -v`

Expected: all 10 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/media_source_service.go internal/service/media_source_service_test.go
git commit -m "feat: add MediaSourceService with path validation and tests"
```

---

### Task 5: Handler + Tests

**Files:**
- Create: `internal/handler/media_source_handler.go`
- Create: `internal/handler/media_source_handler_test.go`

- [ ] **Step 1: Write handler tests**

Create `internal/handler/media_source_handler_test.go`:

```go
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

func setupMediaSourceRouter(svc *service.MediaSourceService) *gin.Engine {
	r := gin.New()
	h := NewMediaSourceHandler(svc)
	r.GET("/api/media-sources", h.List)
	r.POST("/api/media-sources", h.Create)
	r.PUT("/api/media-sources/:id", h.Update)
	r.DELETE("/api/media-sources/:id", h.Delete)
	return r
}

func TestMediaSourceHandler_List_Empty(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		ListFunc: func(ctx context.Context) ([]model.MediaSource, error) {
			return []model.MediaSource{}, nil
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/media-sources", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp model.SuccessResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	sources, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected data to be array, got %T", resp.Data)
	}
	if len(sources) != 0 {
		t.Errorf("expected empty array, got %d items", len(sources))
	}
}

func TestMediaSourceHandler_Create_Success(t *testing.T) {
	// Create real temp dir so path validation passes
	prefix, validPath := setupTempMountForHandler(t)

	repo := &mock.MediaSourceRepository{
		CreateFunc: func(ctx context.Context, source *model.MediaSource) error {
			source.ID = "ms-1"
			source.Enabled = true
			source.CreatedAt = time.Now()
			source.UpdatedAt = time.Now()
			return nil
		},
	}
	svc := service.NewMediaSourceService(repo, prefix)
	r := setupMediaSourceRouter(svc)

	body := `{"label":"Videos","mount_path":"` + validPath + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/media-sources", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMediaSourceHandler_Create_MissingFields(t *testing.T) {
	svc := service.NewMediaSourceService(&mock.MediaSourceRepository{}, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	body := `{"label":"Videos"}`
	req := httptest.NewRequest(http.MethodPost, "/api/media-sources", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestMediaSourceHandler_Create_DuplicatePath(t *testing.T) {
	prefix, validPath := setupTempMountForHandler(t)

	repo := &mock.MediaSourceRepository{
		CreateFunc: func(ctx context.Context, source *model.MediaSource) error {
			return model.ErrAlreadyExists
		},
	}
	svc := service.NewMediaSourceService(repo, prefix)
	r := setupMediaSourceRouter(svc)

	body := `{"label":"Dup","mount_path":"` + validPath + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/media-sources", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMediaSourceHandler_Update_Success(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return &model.MediaSource{ID: id, Label: "Old", Enabled: true}, nil
		},
		UpdateFunc: func(ctx context.Context, source *model.MediaSource) error {
			return nil
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	body := `{"label":"New","enabled":false}`
	req := httptest.NewRequest(http.MethodPut, "/api/media-sources/ms-1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMediaSourceHandler_Update_NotFound(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
			return nil, model.ErrNotFound
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	body := `{"label":"X","enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/api/media-sources/nope", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMediaSourceHandler_Delete_Success(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/media-sources/ms-1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMediaSourceHandler_Delete_NotFound(t *testing.T) {
	repo := &mock.MediaSourceRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			return model.ErrNotFound
		},
	}
	svc := service.NewMediaSourceService(repo, "/mnt/host/")
	r := setupMediaSourceRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/api/media-sources/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
```

Add a helper at the top of the test file (after imports) for creating temp directories in handler tests:

```go
func setupTempMountForHandler(t *testing.T) (prefix string, validPath string) {
	t.Helper()
	base := t.TempDir()
	mountHost := filepath.Join(base, "mnt", "host")
	subDir := filepath.Join(mountHost, "Videos")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create temp dirs: %v", err)
	}
	return mountHost + string(filepath.Separator), subDir
}
```

This requires adding `"os"` and `"path/filepath"` to the imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `docker compose exec vaultflix-api go test ./internal/handler/ -run TestMediaSourceHandler -v 2>&1 | head -20`

Expected: compilation error — `NewMediaSourceHandler` not defined.

- [ ] **Step 3: Write handler implementation**

Create `internal/handler/media_source_handler.go`:

```go
package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

type MediaSourceHandler struct {
	service *service.MediaSourceService
}

func NewMediaSourceHandler(service *service.MediaSourceService) *MediaSourceHandler {
	return &MediaSourceHandler{service: service}
}

func (h *MediaSourceHandler) List(c *gin.Context) {
	sources, err := h.service.List(c.Request.Context())
	if err != nil {
		slog.Error("failed to list media sources", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to list media sources",
		})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{Data: sources})
}

type createMediaSourceRequest struct {
	Label     string `json:"label" binding:"required"`
	MountPath string `json:"mount_path" binding:"required"`
}

func (h *MediaSourceHandler) Create(c *gin.Context) {
	var req createMediaSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "label and mount_path are required",
		})
		return
	}

	source := &model.MediaSource{
		Label:     req.Label,
		MountPath: req.MountPath,
	}

	err := h.service.Create(c.Request.Context(), source)
	if err != nil {
		if errors.Is(err, model.ErrPathNotAllowed) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: err.Error(),
			})
			return
		}
		if errors.Is(err, model.ErrPathNotExist) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Error:   "bad_request",
				Message: err.Error(),
			})
			return
		}
		if errors.Is(err, model.ErrAlreadyExists) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error:   "already_exists",
				Message: "a media source with this mount_path already exists",
			})
			return
		}
		slog.Error("failed to create media source", "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to create media source",
		})
		return
	}

	c.JSON(http.StatusCreated, model.SuccessResponse{Data: source})
}

type updateMediaSourceRequest struct {
	Label   string `json:"label" binding:"required"`
	Enabled *bool  `json:"enabled" binding:"required"`
}

func (h *MediaSourceHandler) Update(c *gin.Context) {
	id := c.Param("id")

	var req updateMediaSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "label and enabled are required",
		})
		return
	}

	err := h.service.Update(c.Request.Context(), id, req.Label, *req.Enabled)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "media source not found",
			})
			return
		}
		slog.Error("failed to update media source", "error", err, "media_source_id", id)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to update media source",
		})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: gin.H{"message": "media source updated"},
	})
}

func (h *MediaSourceHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	err := h.service.Delete(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "media source not found",
			})
			return
		}
		slog.Error("failed to delete media source", "error", err, "media_source_id", id)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to delete media source",
		})
		return
	}

	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `docker compose exec vaultflix-api go test ./internal/handler/ -run TestMediaSourceHandler -v`

Expected: all 8 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/media_source_handler.go internal/handler/media_source_handler_test.go
git commit -m "feat: add MediaSourceHandler with CRUD endpoints and tests"
```

---

### Task 6: DI Wiring + Route Registration

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Add DI wiring**

In `cmd/server/main.go`, after the line `recRepo := repository.NewRecommendationRepository(pool)` (line 94), add:

```go
mediaSourceRepo := repository.NewMediaSourceRepository(pool)
```

After the line `recService := service.NewRecommendationService(recRepo, videoRepo, minioService)` (line 104), add:

```go
mediaSourceService := service.NewMediaSourceService(mediaSourceRepo, service.AllowedMountPrefix)
```

After the line `userHandler := handler.NewUserHandler(userService)` (line 115), add:

```go
mediaSourceHandler := handler.NewMediaSourceHandler(mediaSourceService)
```

- [ ] **Step 2: Add route registration**

After the recommendations routes block (after line 171), add:

```go
		// Media source endpoints (admin only, enforced by Casbin)
		api.GET("/media-sources", mediaSourceHandler.List)
		api.POST("/media-sources", mediaSourceHandler.Create)
		api.PUT("/media-sources/:id", mediaSourceHandler.Update)
		api.DELETE("/media-sources/:id", mediaSourceHandler.Delete)
```

- [ ] **Step 3: Run full test suite**

Run: `docker compose exec vaultflix-api go test ./... -v 2>&1 | tail -30`

Expected: all tests PASS, no compilation errors.

- [ ] **Step 4: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire MediaSource DI and register routes in main.go"
```

---

### Task 7: Full Test Run + Verification

- [ ] **Step 1: Run full test suite**

Run: `docker compose exec vaultflix-api go test ./... -count=1 -v`

Expected: ALL tests pass. Pay attention to any new failures in existing tests.

- [ ] **Step 2: Verify compilation**

Run: `docker compose exec vaultflix-api go build ./cmd/server/`

Expected: clean build, no errors.

- [ ] **Step 3: List assumptions**

Document any assumptions made during implementation that deviate from or weren't specified in the spec.
