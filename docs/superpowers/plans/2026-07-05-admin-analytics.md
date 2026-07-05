# Admin Analytics + Watch-Session Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin "分析 Analytics" page backed by a new watch-session (heartbeat) pipeline that records real accumulated watch time and precise daily trends.

**Architecture:** A new `watch_sessions` table is UPSERTed by a client-generated `session_id` on a 15s heartbeat (additive to, never replacing, the existing `watch_history`). A read-only `GET /api/admin/analytics` aggregates it into a windowed summary (KPIs, daily trend, top videos, top tags). Frontend renders it with zero-dependency inline-SVG charts. Strict Handler → Service → Repository layering, interface-first, hand-written mocks.

**Tech Stack:** Go 1.22+ (gin, pgx/v5, log/slog), PostgreSQL 16, React 18 + TypeScript (Vite, vitest), Casbin RBAC.

## Global Constraints

- Go files ≤ 300 lines; functions ≤ 50 lines. Split when exceeded.
- Layering: Handler parses HTTP + logs + responds; Service holds business logic; Repository runs SQL. No cross-layer leaks (no SQL in handler, no `*gin.Context` in service/repo).
- Errors: wrap with `%w` + lowercase context, no bare `return err`; log + HTTP only in handler; use `errors.Is`/`errors.As`.
- SQL: keywords UPPERCASE, identifiers snake_case, parameterized (`$1`), each query a `const` at file top.
- Migrations: `NNN_description.up.sql` / `.down.sql`; down fully reversible. Next number = `015`.
- `log/slog` structured fields only (no `fmt.Sprintf` into messages).
- Mocks hand-written in `internal/mock/`, no third-party mock frameworks. No new third-party deps anywhere (frontend charts = inline SVG).
- Repository interfaces defined in `repository` package (match existing convention); service structs hold interface-typed repo fields; handlers hold concrete `*service.X` pointers.
- Response envelope: success `{ "data": ... }` (`model.SuccessResponse`); error `model.ErrorResponse{Error, Message}`. HTTP codes: 200 GET ok, 204 no-body, 400 validation, 401 unauth, 403 Casbin, 404 missing, 500 internal.
- Business params (`days`, `limit`, heartbeat cadence) are request-tunable, not env-only.
- Frontend: axios interceptor already unwraps `{data}`; API functions return `res.data` directly. Async effects use a cleanup flag; retry/counters use `useRef`.
- Done condition: `task verify` green + `task test-integration` green (DB/streaming touched) + PR CI green.

**Verified schema facts (do not re-derive):**
- `videos(id UUID, title VARCHAR, thumbnail_key VARCHAR, duration_seconds INT, ...)`.
- `tags(id SERIAL/INT, name VARCHAR, category VARCHAR)` — **tag id is INT, not UUID**.
- `video_tags(video_id UUID, tag_id INT, PRIMARY KEY(video_id, tag_id))`.
- `watch_history` stays untouched.
- Casbin: `admin` already has `/api/*` all verbs; `viewer` needs an explicit per-route line.
- Repo constructors take `*pgxpool.Pool`. `pgx.ErrNoRows` → `model.ErrNotFound`.

---

## File Structure

**Backend — create:**
- `migrations/015_create_watch_sessions.up.sql`, `.down.sql`
- `internal/model/watch_session.go` — `WatchSession`, `HeartbeatInput`
- `internal/model/analytics.go` — `AnalyticsSummary`, `DailyPoint`, `TopVideo`, `TopTag`, `AnalyticsQuery`
- `internal/repository/watch_session_repo.go` — `WatchSessionRepository` iface + impl (UPSERT)
- `internal/repository/analytics_repo.go` — `AnalyticsRepository` iface + impl (aggregations)
- `internal/service/watch_session_service.go` — `WatchSessionService` (clamp + upsert)
- `internal/service/analytics_service.go` — `AnalyticsService` (assemble summary, zero-fill daily)
- `internal/handler/watch_session_handler.go` — heartbeat endpoint
- `internal/handler/analytics_handler.go` — analytics endpoint
- `internal/mock/watch_session_repo_mock.go`, `internal/mock/analytics_repo_mock.go`
- Tests alongside: `*_service_test.go`, `*_handler_test.go`, `*_repo_test.go` (repo tests are integration, gated).

**Backend — modify:**
- `cmd/server/main.go` — wire repos/services/handlers + register 2 routes
- `casbin/policy.csv` — add viewer heartbeat line

**Frontend — create:**
- `web/src/api/watchSession.ts` — `postHeartbeat`
- `web/src/api/analytics.ts` — `getAnalytics` + types
- `web/src/components/admin/charts/StatTile.tsx`
- `web/src/components/admin/charts/AreaChart.tsx`
- `web/src/components/admin/charts/BarChart.tsx`
- `web/src/components/admin/charts/scale.ts` — pure scale helpers (unit-tested)
- `web/src/components/admin/charts/scale.test.ts`
- `web/src/pages/admin/AnalyticsPage.tsx`
- `web/src/pages/admin/AnalyticsPage.test.tsx`
- `web/src/lib/heartbeat.ts` — `clampDelta` pure helper
- `web/src/lib/heartbeat.test.ts`

**Frontend — modify:**
- `web/src/pages/PlayerPage.tsx` — heartbeat integration
- `web/src/App.tsx` — add `/admin/analytics` route
- `web/src/lib/adminNav.ts` — flip `analytics` to `enabled:true`

**Integration:**
- `scripts/test_all.sh` (or the test-runner suite it calls) — heartbeat → analytics e2e assertion. Inspect the file first and follow its existing structure.

---

## Task 1: Migration + models

**Files:**
- Create: `migrations/015_create_watch_sessions.up.sql`, `migrations/015_create_watch_sessions.down.sql`
- Create: `internal/model/watch_session.go`
- Create: `internal/model/analytics.go`

**Interfaces:**
- Produces: `model.WatchSession`, `model.HeartbeatInput`, `model.AnalyticsSummary`, `model.DailyPoint`, `model.TopVideo`, `model.TopTag`, `model.AnalyticsQuery`.

- [ ] **Step 1: Write the up migration**

`migrations/015_create_watch_sessions.up.sql`:

```sql
CREATE TABLE watch_sessions (
    id                     UUID PRIMARY KEY,
    user_id                UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_id               UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    watched_seconds        INT NOT NULL DEFAULT 0,
    max_progress_seconds   INT NOT NULL DEFAULT 0,
    video_duration_seconds INT NOT NULL DEFAULT 0,
    started_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_heartbeat_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_watch_sessions_started ON watch_sessions(started_at DESC);
CREATE INDEX idx_watch_sessions_video   ON watch_sessions(video_id);
CREATE INDEX idx_watch_sessions_user    ON watch_sessions(user_id);
```

- [ ] **Step 2: Write the down migration**

`migrations/015_create_watch_sessions.down.sql`:

```sql
DROP TABLE watch_sessions;
```

- [ ] **Step 3: Write the watch_session model**

`internal/model/watch_session.go`:

```go
package model

import "time"

// WatchSession is one continuous viewing of a video, accumulated via heartbeats.
type WatchSession struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	VideoID              string    `json:"video_id"`
	WatchedSeconds       int       `json:"watched_seconds"`
	MaxProgressSeconds   int       `json:"max_progress_seconds"`
	VideoDurationSeconds int       `json:"video_duration_seconds"`
	StartedAt            time.Time `json:"started_at"`
	LastHeartbeatAt      time.Time `json:"last_heartbeat_at"`
}

// HeartbeatInput is one heartbeat's accumulated playback delta for a session.
type HeartbeatInput struct {
	SessionID       string
	UserID          string
	VideoID         string
	PlayedDelta     int
	PositionSeconds int
}
```

- [ ] **Step 4: Write the analytics model**

`internal/model/analytics.go`:

```go
package model

// AnalyticsQuery holds the tunable window/limit for an analytics request.
type AnalyticsQuery struct {
	Days  int
	Limit int
}

// DailyPoint is one calendar day of the trend (zero-filled for empty days).
type DailyPoint struct {
	Date       string  `json:"date"` // YYYY-MM-DD
	Views      int     `json:"views"`
	WatchHours float64 `json:"watch_hours"`
}

// TopVideo is one row of the most-watched leaderboard within the window.
type TopVideo struct {
	VideoID      string  `json:"video_id"`
	Title        string  `json:"title"`
	ThumbnailKey string  `json:"thumbnail_key"`
	Views        int     `json:"views"`
	WatchHours   float64 `json:"watch_hours"`
}

// TopTag is one tag's view count within the window. Tag IDs are integers.
type TopTag struct {
	TagID int    `json:"tag_id"`
	Name  string `json:"name"`
	Views int    `json:"views"`
}

// AnalyticsSummary is the full payload returned by GET /api/admin/analytics.
type AnalyticsSummary struct {
	RangeDays         int          `json:"range_days"`
	TotalViews        int          `json:"total_views"`
	TotalWatchHours   float64      `json:"total_watch_hours"`
	AvgCompletionRate float64      `json:"avg_completion_rate"`
	ActiveUsers       int          `json:"active_users"`
	DailyTrend        []DailyPoint `json:"daily_trend"`
	TopVideos         []TopVideo   `json:"top_videos"`
	TopTags           []TopTag     `json:"top_tags"`
}
```

- [ ] **Step 5: Verify it compiles**

Run: `go build ./...`
Expected: success (no test yet — models + migration only).

- [ ] **Step 6: Commit**

```bash
git add migrations/015_create_watch_sessions.up.sql migrations/015_create_watch_sessions.down.sql internal/model/watch_session.go internal/model/analytics.go
git commit -m "feat: add watch_sessions migration and analytics models"
```

---

## Task 2: WatchSessionRepository (UPSERT)

**Files:**
- Create: `internal/repository/watch_session_repo.go`
- Create: `internal/mock/watch_session_repo_mock.go`
- Test: `internal/repository/watch_session_repo_test.go` (integration-gated, see Step 1 note)

**Interfaces:**
- Consumes: `model.HeartbeatInput`.
- Produces: `repository.WatchSessionRepository` interface with `Upsert(ctx, in model.HeartbeatInput) error` — returns `model.ErrNotFound` when the video does not exist (so no session row can be created/updated).

- [ ] **Step 1: Write the mock (used by service tests in Task 3)**

`internal/mock/watch_session_repo_mock.go`:

```go
package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

type WatchSessionRepository struct {
	UpsertFunc func(ctx context.Context, in model.HeartbeatInput) error
}

func (m *WatchSessionRepository) Upsert(ctx context.Context, in model.HeartbeatInput) error {
	if m.UpsertFunc == nil {
		return fmt.Errorf("mock: UpsertFunc not set")
	}
	return m.UpsertFunc(ctx, in)
}
```

- [ ] **Step 2: Write the repository interface + implementation**

`internal/repository/watch_session_repo.go`:

```go
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// WatchSessionRepository persists heartbeat-accumulated watch sessions.
//
// Upsert inserts a new session (snapshotting the video's duration) or, on
// session_id conflict, adds the delta to watched_seconds and advances
// max_progress_seconds. It returns model.ErrNotFound when the video does not
// exist, so a heartbeat for a deleted/unknown video affects no rows.
type WatchSessionRepository interface {
	Upsert(ctx context.Context, in model.HeartbeatInput) error
}

// The INSERT ... SELECT sources video_duration_seconds from videos; if the
// video is missing the SELECT yields no row, nothing is inserted, the ON
// CONFLICT clause never fires, and RowsAffected == 0 → ErrNotFound.
const queryUpsertWatchSession = `
    INSERT INTO watch_sessions
        (id, user_id, video_id, watched_seconds, max_progress_seconds, video_duration_seconds)
    SELECT $1, $2, $3, $4, $5, v.duration_seconds
    FROM videos v
    WHERE v.id = $3
    ON CONFLICT (id) DO UPDATE SET
        watched_seconds      = watch_sessions.watched_seconds + EXCLUDED.watched_seconds,
        max_progress_seconds = GREATEST(watch_sessions.max_progress_seconds, EXCLUDED.max_progress_seconds),
        last_heartbeat_at    = NOW()
`

type watchSessionRepository struct {
	pool *pgxpool.Pool
}

func NewWatchSessionRepository(pool *pgxpool.Pool) WatchSessionRepository {
	return &watchSessionRepository{pool: pool}
}

func (r *watchSessionRepository) Upsert(ctx context.Context, in model.HeartbeatInput) error {
	result, err := r.pool.Exec(ctx, queryUpsertWatchSession,
		in.SessionID, in.UserID, in.VideoID, in.PlayedDelta, in.PositionSeconds)
	if err != nil {
		return fmt.Errorf("failed to upsert watch session %s: %w", in.SessionID, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}
```

- [ ] **Step 3: Write the integration repo test (gated)**

> Note: repo tests need a real DB. Check an existing `*_repo_test.go` for the build tag / skip guard this repo uses (e.g. `testing.Short()` skip or an integration build tag) and mirror it exactly. If repo tests here run only under `task test-integration`, place the assertion there instead and keep this file consistent with siblings.

`internal/repository/watch_session_repo_test.go` (mirror sibling guard):

```go
package repository

// Follow the sibling repo-test harness (same DB setup helper, same skip guard).
// Assert: first Upsert on a real video inserts a row with watched_seconds=delta
// and video_duration_seconds snapshotted; a second Upsert with the same
// session_id sums watched_seconds and takes GREATEST(max_progress); an Upsert
// with a random (missing) video_id returns model.ErrNotFound.
```

- [ ] **Step 4: Verify build + mock**

Run: `go build ./... && go vet ./internal/repository/... ./internal/mock/...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/watch_session_repo.go internal/mock/watch_session_repo_mock.go internal/repository/watch_session_repo_test.go
git commit -m "feat: add watch session repository with heartbeat upsert"
```

---

## Task 3: WatchSessionService (clamp + delegate)

**Files:**
- Create: `internal/service/watch_session_service.go`
- Test: `internal/service/watch_session_service_test.go`

**Interfaces:**
- Consumes: `repository.WatchSessionRepository`, `model.HeartbeatInput`.
- Produces: `service.WatchSessionService` (concrete) with:
  - `NewWatchSessionService(repo repository.WatchSessionRepository) *WatchSessionService`
  - `RecordHeartbeat(ctx, in model.HeartbeatInput) error` — clamps `PlayedDelta` to `[0, MaxHeartbeatDelta]` and `PositionSeconds` to `>= 0`, returns `model.ErrInvalidInput` when `SessionID`/`VideoID`/`UserID` empty or `PlayedDelta < 0`, propagates `model.ErrNotFound`.
  - const `MaxHeartbeatDelta = 22`.

> `model.ErrInvalidInput`: check `internal/model/errors.go`. If a suitable sentinel (e.g. `ErrInvalidInput` / `ErrValidation`) already exists, use it; otherwise add `ErrInvalidInput = errors.New("invalid input")` in that file within this task.

- [ ] **Step 1: Write the failing test**

`internal/service/watch_session_service_test.go`:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestRecordHeartbeat_ClampsDeltaToMax(t *testing.T) {
	var got model.HeartbeatInput
	repo := &mock.WatchSessionRepository{
		UpsertFunc: func(_ context.Context, in model.HeartbeatInput) error { got = in; return nil },
	}
	svc := NewWatchSessionService(repo)

	err := svc.RecordHeartbeat(context.Background(), model.HeartbeatInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1", PlayedDelta: 999, PositionSeconds: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PlayedDelta != MaxHeartbeatDelta {
		t.Fatalf("expected delta clamped to %d, got %d", MaxHeartbeatDelta, got.PlayedDelta)
	}
}

func TestRecordHeartbeat_RejectsNegativeDelta(t *testing.T) {
	repo := &mock.WatchSessionRepository{
		UpsertFunc: func(_ context.Context, _ model.HeartbeatInput) error { return nil },
	}
	svc := NewWatchSessionService(repo)
	err := svc.RecordHeartbeat(context.Background(), model.HeartbeatInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1", PlayedDelta: -1, PositionSeconds: 0,
	})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRecordHeartbeat_MissingVideo(t *testing.T) {
	repo := &mock.WatchSessionRepository{
		UpsertFunc: func(_ context.Context, _ model.HeartbeatInput) error { return model.ErrNotFound },
	}
	svc := NewWatchSessionService(repo)
	err := svc.RecordHeartbeat(context.Background(), model.HeartbeatInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1", PlayedDelta: 5, PositionSeconds: 5,
	})
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestRecordHeartbeat -v`
Expected: FAIL — `NewWatchSessionService` undefined.

- [ ] **Step 3: Write the implementation**

`internal/service/watch_session_service.go`:

```go
package service

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// MaxHeartbeatDelta caps one heartbeat's counted play time (15s cadence x1.5)
// so seeks and background tabs cannot inflate accumulated watch time.
const MaxHeartbeatDelta = 22

type WatchSessionService struct {
	repo repository.WatchSessionRepository
}

func NewWatchSessionService(repo repository.WatchSessionRepository) *WatchSessionService {
	return &WatchSessionService{repo: repo}
}

// RecordHeartbeat validates and clamps a heartbeat, then upserts its session.
func (s *WatchSessionService) RecordHeartbeat(ctx context.Context, in model.HeartbeatInput) error {
	if in.SessionID == "" || in.UserID == "" || in.VideoID == "" || in.PlayedDelta < 0 {
		return fmt.Errorf("heartbeat requires session/user/video and non-negative delta: %w", model.ErrInvalidInput)
	}
	if in.PlayedDelta > MaxHeartbeatDelta {
		in.PlayedDelta = MaxHeartbeatDelta
	}
	if in.PositionSeconds < 0 {
		in.PositionSeconds = 0
	}
	if err := s.repo.Upsert(ctx, in); err != nil {
		return fmt.Errorf("failed to record heartbeat: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/service/ -run TestRecordHeartbeat -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/service/watch_session_service.go internal/service/watch_session_service_test.go internal/model/errors.go
git commit -m "feat: add watch session service with heartbeat validation and clamping"
```

---

## Task 4: WatchSessionHandler + route + casbin

**Files:**
- Create: `internal/handler/watch_session_handler.go`
- Test: `internal/handler/watch_session_handler_test.go`
- Modify: `cmd/server/main.go` (wire + route), `casbin/policy.csv`

**Interfaces:**
- Consumes: `*service.WatchSessionService`.
- Produces: `handler.WatchSessionHandler` with `NewWatchSessionHandler(svc *service.WatchSessionService)` and `Heartbeat(c *gin.Context)`. Route: `POST /api/watch-sessions/heartbeat` → 204.

- [ ] **Step 1: Write the failing handler test**

`internal/handler/watch_session_handler_test.go`:

```go
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

func setupWatchSessionRouter(repo *mock.WatchSessionRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewWatchSessionService(repo)
	h := NewWatchSessionHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "u1") })
	r.POST("/api/watch-sessions/heartbeat", h.Heartbeat)
	return r
}

func postJSON(r *gin.Engine, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/watch-sessions/heartbeat", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHeartbeat_OK(t *testing.T) {
	var got model.HeartbeatInput
	repo := &mock.WatchSessionRepository{
		UpsertFunc: func(_ context.Context, in model.HeartbeatInput) error { got = in; return nil },
	}
	r := setupWatchSessionRouter(repo)
	w := postJSON(r, `{"session_id":"s1","video_id":"v1","played_delta":15,"position_seconds":42}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d (%s)", w.Code, w.Body.String())
	}
	if got.UserID != "u1" || got.VideoID != "v1" || got.PlayedDelta != 15 {
		t.Fatalf("unexpected heartbeat forwarded: %+v", got)
	}
}

func TestHeartbeat_MissingFields_400(t *testing.T) {
	repo := &mock.WatchSessionRepository{UpsertFunc: func(_ context.Context, _ model.HeartbeatInput) error { return nil }}
	r := setupWatchSessionRouter(repo)
	w := postJSON(r, `{"session_id":"s1"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var resp model.ErrorResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Fatalf("expected error body")
	}
}

func TestHeartbeat_VideoNotFound_404(t *testing.T) {
	repo := &mock.WatchSessionRepository{
		UpsertFunc: func(_ context.Context, _ model.HeartbeatInput) error { return model.ErrNotFound },
	}
	r := setupWatchSessionRouter(repo)
	w := postJSON(r, `{"session_id":"s1","video_id":"vX","played_delta":5,"position_seconds":5}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/handler/ -run TestHeartbeat -v`
Expected: FAIL — `NewWatchSessionHandler` undefined.

- [ ] **Step 3: Write the handler**

`internal/handler/watch_session_handler.go`:

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

type WatchSessionHandler struct {
	service *service.WatchSessionService
}

func NewWatchSessionHandler(svc *service.WatchSessionService) *WatchSessionHandler {
	return &WatchSessionHandler{service: svc}
}

type heartbeatRequest struct {
	SessionID       string `json:"session_id" binding:"required"`
	VideoID         string `json:"video_id" binding:"required"`
	PlayedDelta     int    `json:"played_delta"`
	PositionSeconds int    `json:"position_seconds"`
}

// Heartbeat records one accumulated playback delta for a viewing session.
func (h *WatchSessionHandler) Heartbeat(c *gin.Context) {
	userID := c.GetString("user_id")

	var req heartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "session_id and video_id are required",
		})
		return
	}

	err := h.service.RecordHeartbeat(c.Request.Context(), model.HeartbeatInput{
		SessionID:       req.SessionID,
		UserID:          userID,
		VideoID:         req.VideoID,
		PlayedDelta:     req.PlayedDelta,
		PositionSeconds: req.PositionSeconds,
	})
	if err != nil {
		if errors.Is(err, model.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid heartbeat"})
			return
		}
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "video not found"})
			return
		}
		slog.Error("failed to record heartbeat", "error", err, "user_id", userID, "video_id", req.VideoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to record heartbeat"})
		return
	}

	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/handler/ -run TestHeartbeat -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Wire into main.go**

In `cmd/server/main.go`, next to the other repo/service/handler constructors (around lines 130/142/176):

```go
watchSessionRepo := repository.NewWatchSessionRepository(pool)
watchSessionService := service.NewWatchSessionService(watchSessionRepo)
watchSessionHandler := handler.NewWatchSessionHandler(watchSessionService)
```

In the `api` route group, under the "Watch history endpoints" block:

```go
		// Watch session heartbeat (accumulated real watch time)
		api.POST("/watch-sessions/heartbeat", watchSessionHandler.Heartbeat)
```

- [ ] **Step 6: Add casbin viewer policy**

Append to `casbin/policy.csv` (in the viewer block):

```csv
p, viewer, /api/watch-sessions/heartbeat, POST
```

- [ ] **Step 7: Verify build + tests**

Run: `go build ./... && go test ./internal/handler/ ./internal/service/ -run 'Heartbeat|RecordHeartbeat'`
Expected: success + PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/handler/watch_session_handler.go internal/handler/watch_session_handler_test.go cmd/server/main.go casbin/policy.csv
git commit -m "feat: add heartbeat endpoint with viewer casbin policy"
```

---

## Task 5: AnalyticsRepository (aggregations)

**Files:**
- Create: `internal/repository/analytics_repo.go`
- Create: `internal/mock/analytics_repo_mock.go`
- Test: `internal/repository/analytics_repo_test.go` (integration-gated, mirror siblings)

**Interfaces:**
- Consumes: `model.TopVideo`, `model.TopTag`.
- Produces: `repository.AnalyticsRepository` interface:
  - `KPIs(ctx, days int) (totalViews int, totalWatchedSeconds int64, avgCompletion float64, activeUsers int, err error)`
  - `DailyRaw(ctx, days int) (map[string]model.DailyRawRow, error)` — keyed `YYYY-MM-DD`; missing days absent (service fills).
  - `TopVideos(ctx, days, limit int) ([]model.TopVideo, error)`
  - `TopTags(ctx, days, limit int) ([]model.TopTag, error)`
- Also add to `internal/model/analytics.go`:

```go
// DailyRawRow is one present day from the DB before zero-fill (seconds, not hours).
type DailyRawRow struct {
	Views          int
	WatchedSeconds int64
}
```

> View threshold `watched_seconds >= 10` is applied inside every count via `FILTER`. `make_interval(days => $1)` windows on `started_at`.

- [ ] **Step 1: Write the mock**

`internal/mock/analytics_repo_mock.go`:

```go
package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

type AnalyticsRepository struct {
	KPIsFunc       func(ctx context.Context, days int) (int, int64, float64, int, error)
	DailyRawFunc   func(ctx context.Context, days int) (map[string]model.DailyRawRow, error)
	TopVideosFunc  func(ctx context.Context, days, limit int) ([]model.TopVideo, error)
	TopTagsFunc    func(ctx context.Context, days, limit int) ([]model.TopTag, error)
}

func (m *AnalyticsRepository) KPIs(ctx context.Context, days int) (int, int64, float64, int, error) {
	if m.KPIsFunc == nil {
		return 0, 0, 0, 0, fmt.Errorf("mock: KPIsFunc not set")
	}
	return m.KPIsFunc(ctx, days)
}

func (m *AnalyticsRepository) DailyRaw(ctx context.Context, days int) (map[string]model.DailyRawRow, error) {
	if m.DailyRawFunc == nil {
		return nil, fmt.Errorf("mock: DailyRawFunc not set")
	}
	return m.DailyRawFunc(ctx, days)
}

func (m *AnalyticsRepository) TopVideos(ctx context.Context, days, limit int) ([]model.TopVideo, error) {
	if m.TopVideosFunc == nil {
		return nil, fmt.Errorf("mock: TopVideosFunc not set")
	}
	return m.TopVideosFunc(ctx, days, limit)
}

func (m *AnalyticsRepository) TopTags(ctx context.Context, days, limit int) ([]model.TopTag, error) {
	if m.TopTagsFunc == nil {
		return nil, fmt.Errorf("mock: TopTagsFunc not set")
	}
	return m.TopTagsFunc(ctx, days, limit)
}
```

- [ ] **Step 2: Write the repository**

`internal/repository/analytics_repo.go`:

```go
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// AnalyticsRepository runs read-only aggregations over watch_sessions.
// The window is the trailing `days` on started_at; a "view" is a session with
// watched_seconds >= 10.
type AnalyticsRepository interface {
	KPIs(ctx context.Context, days int) (totalViews int, totalWatchedSeconds int64, avgCompletion float64, activeUsers int, err error)
	DailyRaw(ctx context.Context, days int) (map[string]model.DailyRawRow, error)
	TopVideos(ctx context.Context, days, limit int) ([]model.TopVideo, error)
	TopTags(ctx context.Context, days, limit int) ([]model.TopTag, error)
}

const queryAnalyticsKPIs = `
    SELECT
        COUNT(*) FILTER (WHERE watched_seconds >= 10) AS total_views,
        COALESCE(SUM(watched_seconds), 0) AS total_watched_seconds,
        COALESCE(AVG(LEAST(max_progress_seconds::float / video_duration_seconds, 1.0))
            FILTER (WHERE video_duration_seconds > 0 AND watched_seconds >= 10), 0) AS avg_completion,
        COUNT(DISTINCT user_id) FILTER (WHERE watched_seconds >= 10) AS active_users
    FROM watch_sessions
    WHERE started_at >= NOW() - make_interval(days => $1)
`

const queryAnalyticsDaily = `
    SELECT to_char(started_at::date, 'YYYY-MM-DD') AS day,
           COUNT(*) FILTER (WHERE watched_seconds >= 10) AS views,
           COALESCE(SUM(watched_seconds), 0) AS watched_seconds
    FROM watch_sessions
    WHERE started_at >= NOW() - make_interval(days => $1)
    GROUP BY started_at::date
`

const queryAnalyticsTopVideos = `
    SELECT v.id, v.title, v.thumbnail_key,
           COUNT(*) FILTER (WHERE ws.watched_seconds >= 10) AS views,
           COALESCE(SUM(ws.watched_seconds), 0) AS watched_seconds
    FROM watch_sessions ws
    JOIN videos v ON v.id = ws.video_id
    WHERE ws.started_at >= NOW() - make_interval(days => $1)
    GROUP BY v.id, v.title, v.thumbnail_key
    HAVING SUM(ws.watched_seconds) > 0
    ORDER BY SUM(ws.watched_seconds) DESC
    LIMIT $2
`

const queryAnalyticsTopTags = `
    SELECT t.id, t.name,
           COUNT(*) FILTER (WHERE ws.watched_seconds >= 10) AS views
    FROM watch_sessions ws
    JOIN video_tags vt ON vt.video_id = ws.video_id
    JOIN tags t ON t.id = vt.tag_id
    WHERE ws.started_at >= NOW() - make_interval(days => $1)
    GROUP BY t.id, t.name
    HAVING COUNT(*) FILTER (WHERE ws.watched_seconds >= 10) > 0
    ORDER BY views DESC
    LIMIT $2
`

type analyticsRepository struct {
	pool *pgxpool.Pool
}

func NewAnalyticsRepository(pool *pgxpool.Pool) AnalyticsRepository {
	return &analyticsRepository{pool: pool}
}

func (r *analyticsRepository) KPIs(ctx context.Context, days int) (int, int64, float64, int, error) {
	var views, active int
	var watched int64
	var avg float64
	err := r.pool.QueryRow(ctx, queryAnalyticsKPIs, days).Scan(&views, &watched, &avg, &active)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to query analytics kpis: %w", err)
	}
	return views, watched, avg, active, nil
}

func (r *analyticsRepository) DailyRaw(ctx context.Context, days int) (map[string]model.DailyRawRow, error) {
	rows, err := r.pool.Query(ctx, queryAnalyticsDaily, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query analytics daily: %w", err)
	}
	defer rows.Close()

	out := make(map[string]model.DailyRawRow)
	for rows.Next() {
		var day string
		var row model.DailyRawRow
		if err := rows.Scan(&day, &row.Views, &row.WatchedSeconds); err != nil {
			return nil, fmt.Errorf("failed to scan daily row: %w", err)
		}
		out[day] = row
	}
	return out, nil
}

func (r *analyticsRepository) TopVideos(ctx context.Context, days, limit int) ([]model.TopVideo, error) {
	rows, err := r.pool.Query(ctx, queryAnalyticsTopVideos, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top videos: %w", err)
	}
	defer rows.Close()

	out := []model.TopVideo{}
	for rows.Next() {
		var v model.TopVideo
		var watched int64
		if err := rows.Scan(&v.VideoID, &v.Title, &v.ThumbnailKey, &v.Views, &watched); err != nil {
			return nil, fmt.Errorf("failed to scan top video: %w", err)
		}
		v.WatchHours = float64(watched) / 3600.0
		out = append(out, v)
	}
	return out, nil
}

func (r *analyticsRepository) TopTags(ctx context.Context, days, limit int) ([]model.TopTag, error) {
	rows, err := r.pool.Query(ctx, queryAnalyticsTopTags, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top tags: %w", err)
	}
	defer rows.Close()

	out := []model.TopTag{}
	for rows.Next() {
		var t model.TopTag
		if err := rows.Scan(&t.TagID, &t.Name, &t.Views); err != nil {
			return nil, fmt.Errorf("failed to scan top tag: %w", err)
		}
		out = append(out, t)
	}
	return out, nil
}
```

> `WatchHours` rounding to 1 decimal happens in the service (Task 6) for the summary-level totals; per-row hours here stay full-precision and the frontend formats. If you prefer, round in the service's assembly step for consistency — keep it in ONE place.

- [ ] **Step 3: Write the integration repo test (gated)**

Mirror sibling repo-test harness. Seed 1 video + N sessions across dates, assert KPI counts respect the `>=10` threshold, daily map has only present days, top videos ordered by watched seconds, top tags ordered by views.

- [ ] **Step 4: Verify build**

Run: `go build ./... && go vet ./internal/repository/... ./internal/mock/...`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/analytics_repo.go internal/mock/analytics_repo_mock.go internal/model/analytics.go internal/repository/analytics_repo_test.go
git commit -m "feat: add analytics repository aggregations over watch_sessions"
```

---

## Task 6: AnalyticsService (assemble + zero-fill daily)

**Files:**
- Create: `internal/service/analytics_service.go`
- Test: `internal/service/analytics_service_test.go`

**Interfaces:**
- Consumes: `repository.AnalyticsRepository`, `model.*`.
- Produces: `service.AnalyticsService` (concrete):
  - `NewAnalyticsService(repo repository.AnalyticsRepository) *AnalyticsService`
  - `Summary(ctx, q model.AnalyticsQuery) (*model.AnalyticsSummary, error)` — builds the full summary; `DailyTrend` is exactly `q.Days` points ending today (oldest first), empty days zero-filled; `TotalWatchHours` rounded to 1 decimal; `AvgCompletionRate` passed through.
  - Uses `time.Now()` to compute the calendar date range for zero-fill (local server tz — matches `started_at::date`).

- [ ] **Step 1: Write the failing test**

`internal/service/analytics_service_test.go`:

```go
package service

import (
	"context"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestSummary_ZeroFillsDailyTrend(t *testing.T) {
	repo := &mock.AnalyticsRepository{
		KPIsFunc: func(_ context.Context, _ int) (int, int64, float64, int, error) {
			return 5, 7200, 0.5, 2, nil // 7200s = 2.0h
		},
		DailyRawFunc: func(_ context.Context, _ int) (map[string]model.DailyRawRow, error) {
			return map[string]model.DailyRawRow{}, nil // no days present
		},
		TopVideosFunc: func(_ context.Context, _, _ int) ([]model.TopVideo, error) { return []model.TopVideo{}, nil },
		TopTagsFunc:   func(_ context.Context, _, _ int) ([]model.TopTag, error) { return []model.TopTag{}, nil },
	}
	svc := NewAnalyticsService(repo)

	got, err := svc.Summary(context.Background(), model.AnalyticsQuery{Days: 7, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.DailyTrend) != 7 {
		t.Fatalf("expected 7 daily points, got %d", len(got.DailyTrend))
	}
	for _, p := range got.DailyTrend {
		if p.Views != 0 || p.WatchHours != 0 {
			t.Fatalf("expected zero-filled day, got %+v", p)
		}
	}
	if got.TotalWatchHours != 2.0 {
		t.Fatalf("expected 2.0 hours, got %v", got.TotalWatchHours)
	}
	if got.RangeDays != 7 {
		t.Fatalf("expected range_days 7, got %d", got.RangeDays)
	}
}

func TestSummary_MergesPresentDay(t *testing.T) {
	// The most recent day (today) carries data; verify it lands in the last slot.
	repo := &mock.AnalyticsRepository{
		KPIsFunc: func(_ context.Context, _ int) (int, int64, float64, int, error) { return 1, 1800, 0.9, 1, nil },
		DailyRawFunc: func(_ context.Context, _ int) (map[string]model.DailyRawRow, error) {
			today := todayDateString() // helper below, same tz as service
			return map[string]model.DailyRawRow{today: {Views: 3, WatchedSeconds: 1800}}, nil
		},
		TopVideosFunc: func(_ context.Context, _, _ int) ([]model.TopVideo, error) { return []model.TopVideo{}, nil },
		TopTagsFunc:   func(_ context.Context, _, _ int) ([]model.TopTag, error) { return []model.TopTag{}, nil },
	}
	svc := NewAnalyticsService(repo)
	got, err := svc.Summary(context.Background(), model.AnalyticsQuery{Days: 3, Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := got.DailyTrend[len(got.DailyTrend)-1]
	if last.Views != 3 || last.WatchHours != 0.5 {
		t.Fatalf("expected today merged (3 views, 0.5h), got %+v", last)
	}
}
```

> Add a tiny test helper `todayDateString()` in the test file mirroring the service's date formatting (`time.Now().Format("2006-01-02")`).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/service/ -run TestSummary -v`
Expected: FAIL — `NewAnalyticsService` undefined.

- [ ] **Step 3: Write the implementation**

`internal/service/analytics_service.go`:

```go
package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

type AnalyticsService struct {
	repo repository.AnalyticsRepository
}

func NewAnalyticsService(repo repository.AnalyticsRepository) *AnalyticsService {
	return &AnalyticsService{repo: repo}
}

// Summary assembles the windowed analytics payload with a zero-filled daily
// trend of exactly q.Days points (oldest first, ending today).
func (s *AnalyticsService) Summary(ctx context.Context, q model.AnalyticsQuery) (*model.AnalyticsSummary, error) {
	views, watchedSeconds, avgCompletion, activeUsers, err := s.repo.KPIs(ctx, q.Days)
	if err != nil {
		return nil, fmt.Errorf("failed to get kpis: %w", err)
	}
	dailyRaw, err := s.repo.DailyRaw(ctx, q.Days)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily trend: %w", err)
	}
	topVideos, err := s.repo.TopVideos(ctx, q.Days, q.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top videos: %w", err)
	}
	topTags, err := s.repo.TopTags(ctx, q.Days, q.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top tags: %w", err)
	}

	return &model.AnalyticsSummary{
		RangeDays:         q.Days,
		TotalViews:        views,
		TotalWatchHours:   round1(float64(watchedSeconds) / 3600.0),
		AvgCompletionRate: avgCompletion,
		ActiveUsers:       activeUsers,
		DailyTrend:        buildDailyTrend(q.Days, dailyRaw),
		TopVideos:         topVideos,
		TopTags:           topTags,
	}, nil
}

// buildDailyTrend produces days points ending today, merging present rows.
func buildDailyTrend(days int, raw map[string]model.DailyRawRow) []model.DailyPoint {
	trend := make([]model.DailyPoint, 0, days)
	today := time.Now()
	for i := days - 1; i >= 0; i-- {
		date := today.AddDate(0, 0, -i).Format("2006-01-02")
		p := model.DailyPoint{Date: date}
		if row, ok := raw[date]; ok {
			p.Views = row.Views
			p.WatchHours = round1(float64(row.WatchedSeconds) / 3600.0)
		}
		trend = append(trend, p)
	}
	return trend
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/service/ -run TestSummary -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/service/analytics_service.go internal/service/analytics_service_test.go
git commit -m "feat: add analytics service with zero-filled daily trend assembly"
```

---

## Task 7: AnalyticsHandler + route + param clamping

**Files:**
- Create: `internal/handler/analytics_handler.go`
- Test: `internal/handler/analytics_handler_test.go`
- Modify: `cmd/server/main.go`

**Interfaces:**
- Consumes: `*service.AnalyticsService`.
- Produces: `handler.AnalyticsHandler` with `NewAnalyticsHandler(svc *service.AnalyticsService)` and `Get(c *gin.Context)`. Route: `GET /api/admin/analytics?days=&limit=` → 200 `{data: AnalyticsSummary}`. Clamp: `days` default 30, range `[1,365]`; `limit` default 10, range `[1,50]`; non-numeric → default.

- [ ] **Step 1: Write the failing test**

`internal/handler/analytics_handler_test.go`:

```go
package handler

import (
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

func setupAnalyticsRouter(repo *mock.AnalyticsRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewAnalyticsService(repo)
	h := NewAnalyticsHandler(svc)
	r := gin.New()
	r.GET("/api/admin/analytics", h.Get)
	return r
}

func emptyAnalyticsRepo(capture *int) *mock.AnalyticsRepository {
	return &mock.AnalyticsRepository{
		KPIsFunc: func(_ context.Context, days int) (int, int64, float64, int, error) {
			if capture != nil {
				*capture = days
			}
			return 0, 0, 0, 0, nil
		},
		DailyRawFunc:  func(_ context.Context, _ int) (map[string]model.DailyRawRow, error) { return map[string]model.DailyRawRow{}, nil },
		TopVideosFunc: func(_ context.Context, _, _ int) ([]model.TopVideo, error) { return []model.TopVideo{}, nil },
		TopTagsFunc:   func(_ context.Context, _, _ int) ([]model.TopTag, error) { return []model.TopTag{}, nil },
	}
}

func TestAnalytics_DefaultDays(t *testing.T) {
	var gotDays int
	r := setupAnalyticsRouter(emptyAnalyticsRepo(&gotDays))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotDays != 30 {
		t.Fatalf("expected default days 30, got %d", gotDays)
	}
}

func TestAnalytics_ClampsDays(t *testing.T) {
	var gotDays int
	r := setupAnalyticsRouter(emptyAnalyticsRepo(&gotDays))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics?days=9999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if gotDays != 365 {
		t.Fatalf("expected clamped days 365, got %d", gotDays)
	}
}

func TestAnalytics_ReturnsSummaryShape(t *testing.T) {
	r := setupAnalyticsRouter(emptyAnalyticsRepo(nil))
	req := httptest.NewRequest(http.MethodGet, "/api/admin/analytics?days=7", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp struct {
		Data model.AnalyticsSummary `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if resp.Data.RangeDays != 7 || len(resp.Data.DailyTrend) != 7 {
		t.Fatalf("unexpected summary: %+v", resp.Data)
	}
	if resp.Data.TopVideos == nil || resp.Data.TopTags == nil {
		t.Fatalf("expected non-nil slices for empty top lists")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/handler/ -run TestAnalytics -v`
Expected: FAIL — `NewAnalyticsHandler` undefined.

- [ ] **Step 3: Write the handler**

`internal/handler/analytics_handler.go`:

```go
package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

const (
	defaultAnalyticsDays  = 30
	minAnalyticsDays      = 1
	maxAnalyticsDays      = 365
	defaultAnalyticsLimit = 10
	minAnalyticsLimit     = 1
	maxAnalyticsLimit     = 50
)

type AnalyticsHandler struct {
	service *service.AnalyticsService
}

func NewAnalyticsHandler(svc *service.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{service: svc}
}

// Get returns the windowed analytics summary. days/limit are request-tunable.
func (h *AnalyticsHandler) Get(c *gin.Context) {
	q := model.AnalyticsQuery{
		Days:  clampParam(c.Query("days"), defaultAnalyticsDays, minAnalyticsDays, maxAnalyticsDays),
		Limit: clampParam(c.Query("limit"), defaultAnalyticsLimit, minAnalyticsLimit, maxAnalyticsLimit),
	}

	summary, err := h.service.Summary(c.Request.Context(), q)
	if err != nil {
		slog.Error("failed to build analytics summary", "error", err, "days", q.Days, "limit", q.Limit)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to build analytics"})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{Data: summary})
}

// clampParam parses raw and clamps to [min,max]; invalid/empty → def.
func clampParam(raw string, def, min, max int) int {
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
```

> If a `clampParam`-style helper already exists in the handler package, reuse it and drop this copy (DRY). Grep before adding.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/handler/ -run TestAnalytics -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Wire into main.go**

Constructors (near the analytics/session wiring from Task 4):

```go
analyticsRepo := repository.NewAnalyticsRepository(pool)
analyticsService := service.NewAnalyticsService(analyticsRepo)
analyticsHandler := handler.NewAnalyticsHandler(analyticsService)
```

Route (in the admin block, near backfill endpoints):

```go
		// Analytics (admin only, enforced by Casbin)
		api.GET("/admin/analytics", analyticsHandler.Get)
```

- [ ] **Step 6: Full backend gate**

Run: `go build ./... && go test ./internal/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/handler/analytics_handler.go internal/handler/analytics_handler_test.go cmd/server/main.go
git commit -m "feat: add admin analytics endpoint with tunable days/limit"
```

---

## Task 8: Frontend heartbeat helper + API + PlayerPage integration

**Files:**
- Create: `web/src/lib/heartbeat.ts`, `web/src/lib/heartbeat.test.ts`
- Create: `web/src/api/watchSession.ts`
- Modify: `web/src/pages/PlayerPage.tsx`

**Interfaces:**
- Produces: `clampDelta(prev: number, next: number, cap?: number): number` (default cap 22), `postHeartbeat(payload: HeartbeatPayload): Promise<void>`, `HeartbeatPayload`.
- Consumes (PlayerPage): existing `videoRef`, `videoIDRef`, the leave/beacon path.

- [ ] **Step 1: Write the failing pure-helper test**

`web/src/lib/heartbeat.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { clampDelta } from './heartbeat'

describe('clampDelta', () => {
  it('returns positive elapsed play time', () => {
    expect(clampDelta(10, 22)).toBe(12)
  })
  it('caps a forward seek to the max (22)', () => {
    expect(clampDelta(10, 500)).toBe(22)
  })
  it('returns 0 for a backward seek (negative delta)', () => {
    expect(clampDelta(100, 40)).toBe(0)
  })
  it('returns 0 when position is unchanged', () => {
    expect(clampDelta(30, 30)).toBe(0)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && npx vitest run src/lib/heartbeat.test.ts`
Expected: FAIL — cannot find `./heartbeat`.

- [ ] **Step 3: Write the helper + API**

`web/src/lib/heartbeat.ts`:

```ts
// Max play-time counted per heartbeat (15s cadence x1.5). Keeps seeks and
// idle/background tabs from inflating accumulated watch time. Mirrors the
// backend service.MaxHeartbeatDelta.
export const MAX_HEARTBEAT_DELTA = 22

// clampDelta returns the actual seconds played between two currentTime samples,
// clamped to [0, cap]. Backward/zero movement (seek or pause) yields 0.
export function clampDelta(prev: number, next: number, cap = MAX_HEARTBEAT_DELTA): number {
  const delta = Math.floor(next) - Math.floor(prev)
  if (delta <= 0) return 0
  return Math.min(delta, cap)
}
```

`web/src/api/watchSession.ts`:

```ts
import client from './client'

export interface HeartbeatPayload {
  session_id: string
  video_id: string
  played_delta: number
  position_seconds: number
}

export async function postHeartbeat(payload: HeartbeatPayload): Promise<void> {
  await client.post('/watch-sessions/heartbeat', payload)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && npx vitest run src/lib/heartbeat.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Integrate heartbeat into PlayerPage**

In `web/src/pages/PlayerPage.tsx`:

Add imports:

```tsx
import { postHeartbeat } from '../api/watchSession'
import { clampDelta } from '../lib/heartbeat'
```

Add a constant near `PROGRESS_THROTTLE_MS`:

```tsx
const HEARTBEAT_INTERVAL_MS = 15_000
```

Add refs alongside the existing progress refs:

```tsx
  // Heartbeat (accumulated real watch time) — session regenerates per open.
  const sessionIdRef = useRef<string>('')
  const lastSampleSecondsRef = useRef(0)   // last currentTime we measured a delta from
  const pendingDeltaRef = useRef(0)        // play-seconds accumulated since last flush
```

When a video successfully loads (in the `fetchVideo` success block where `videoIDRef.current = data.id` is set), start a fresh session:

```tsx
        videoIDRef.current = data.id
        sessionIdRef.current = crypto.randomUUID()
        lastSampleSecondsRef.current = 0
        pendingDeltaRef.current = 0
```

In `handleTimeUpdate`, accumulate the played delta (add after the existing `reportProgress` call):

```tsx
  function handleTimeUpdate() {
    const el = videoRef.current
    if (!el) return
    reportProgress(el.currentTime)
    // Accumulate real play time for the heartbeat.
    pendingDeltaRef.current += clampDelta(lastSampleSecondsRef.current, el.currentTime)
    lastSampleSecondsRef.current = el.currentTime
  }
```

Add a flush function and a 15s interval effect:

```tsx
  const flushHeartbeat = useCallback((useBeacon = false) => {
    const vid = videoIDRef.current
    const sid = sessionIdRef.current
    const delta = pendingDeltaRef.current
    if (!vid || !sid || delta <= 0) return
    pendingDeltaRef.current = 0
    const el = videoRef.current
    const position = el ? Math.floor(el.currentTime) : 0
    const payload = { session_id: sid, video_id: vid, played_delta: delta, position_seconds: position }
    if (useBeacon) {
      const token = localStorage.getItem('token')
      fetch('/api/watch-sessions/heartbeat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
        body: JSON.stringify(payload),
        keepalive: true,
      }).catch(() => {})
      return
    }
    postHeartbeat(payload).catch((err) => console.warn('failed to send heartbeat', err))
  }, [])

  useEffect(() => {
    const timer = setInterval(() => flushHeartbeat(false), HEARTBEAT_INTERVAL_MS)
    return () => clearInterval(timer)
  }, [flushHeartbeat])
```

In the existing unmount cleanup (where `sendProgressBeacon()` is called), also flush the final heartbeat:

```tsx
    return () => {
      cancelled = true
      // Send final progress on unmount
      sendProgressBeacon()
      flushHeartbeat(true)
    }
```

> Note: `flushHeartbeat` is referenced in the `[id]` effect's cleanup. To avoid a stale closure, keep `flushHeartbeat` as a `useCallback` with empty deps (it reads only refs) — refs are stable so this is safe and satisfies the hooks lint rule. Do NOT add `flushHeartbeat` to the `[id]` dependency array (keep it `[id]`).

- [ ] **Step 6: Typecheck + lint + test**

Run: `cd web && npx tsc --noEmit && npx eslint src/pages/PlayerPage.tsx src/lib/heartbeat.ts src/api/watchSession.ts && npx vitest run src/lib/heartbeat.test.ts`
Expected: no type errors, no lint errors, tests PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/heartbeat.ts web/src/lib/heartbeat.test.ts web/src/api/watchSession.ts web/src/pages/PlayerPage.tsx
git commit -m "feat: report watch-session heartbeats from the player"
```

---

## Task 9: Analytics API client + chart scale helpers

**Files:**
- Create: `web/src/api/analytics.ts`
- Create: `web/src/components/admin/charts/scale.ts`, `scale.test.ts`

**Interfaces:**
- Produces (analytics.ts): `getAnalytics(days: number, limit?: number): Promise<AnalyticsSummary>` + TS types `AnalyticsSummary`, `DailyPoint`, `TopVideo`, `TopTag` matching the Go JSON tags exactly.
- Produces (scale.ts): `niceMax(max: number): number`, `linePath(points, w, h, max): string`, `areaPath(points, w, h, max): string`, `barWidths(values, max, fullWidth): number[]`.

- [ ] **Step 1: Write the failing scale test**

`web/src/components/admin/charts/scale.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { niceMax, barWidths } from './scale'

describe('niceMax', () => {
  it('returns 1 for all-zero data (avoids divide-by-zero)', () => {
    expect(niceMax(0)).toBe(1)
  })
  it('rounds up to a readable ceiling', () => {
    expect(niceMax(7)).toBeGreaterThanOrEqual(7)
    expect(niceMax(42)).toBeGreaterThanOrEqual(42)
  })
})

describe('barWidths', () => {
  it('scales the largest value to full width', () => {
    const w = barWidths([2, 4, 8], 8, 100)
    expect(w[2]).toBe(100)
    expect(w[0]).toBe(25)
  })
  it('handles an all-zero series without NaN', () => {
    const w = barWidths([0, 0], 0, 100)
    expect(w.every((x) => Number.isFinite(x))).toBe(true)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && npx vitest run src/components/admin/charts/scale.test.ts`
Expected: FAIL — cannot find `./scale`.

- [ ] **Step 3: Write scale helpers + API client**

`web/src/components/admin/charts/scale.ts`:

```ts
export interface XY { x: number; y: number }

// niceMax rounds an axis maximum up to a readable ceiling; never returns 0.
export function niceMax(max: number): number {
  if (max <= 0) return 1
  const pow = Math.pow(10, Math.floor(Math.log10(max)))
  const steps = [1, 2, 2.5, 5, 10]
  for (const s of steps) {
    const candidate = s * pow
    if (candidate >= max) return candidate
  }
  return 10 * pow
}

// Map values (oldest→newest) to evenly spaced x, inverted y within [0,h].
function toPoints(values: number[], w: number, h: number, max: number): XY[] {
  const n = values.length
  return values.map((v, i) => ({
    x: n <= 1 ? 0 : (i / (n - 1)) * w,
    y: h - (v / max) * h,
  }))
}

export function linePath(values: number[], w: number, h: number, max: number): string {
  const pts = toPoints(values, w, h, max)
  return pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' ')
}

export function areaPath(values: number[], w: number, h: number, max: number): string {
  const pts = toPoints(values, w, h, max)
  if (pts.length === 0) return ''
  const top = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' ')
  return `${top} L${pts[pts.length - 1].x.toFixed(2)},${h} L${pts[0].x.toFixed(2)},${h} Z`
}

// barWidths scales each value to a pixel width; all-zero → all zero (no NaN).
export function barWidths(values: number[], max: number, fullWidth: number): number[] {
  const m = max > 0 ? max : 0
  return values.map((v) => (m === 0 ? 0 : (v / m) * fullWidth))
}
```

`web/src/api/analytics.ts`:

```ts
import client from './client'

export interface DailyPoint {
  date: string
  views: number
  watch_hours: number
}
export interface TopVideo {
  video_id: string
  title: string
  thumbnail_key: string
  views: number
  watch_hours: number
}
export interface TopTag {
  tag_id: number
  name: string
  views: number
}
export interface AnalyticsSummary {
  range_days: number
  total_views: number
  total_watch_hours: number
  avg_completion_rate: number
  active_users: number
  daily_trend: DailyPoint[]
  top_videos: TopVideo[]
  top_tags: TopTag[]
}

export async function getAnalytics(days: number, limit = 10): Promise<AnalyticsSummary> {
  const res = await client.get<AnalyticsSummary>('/admin/analytics', { params: { days, limit } })
  return res.data
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && npx vitest run src/components/admin/charts/scale.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/api/analytics.ts web/src/components/admin/charts/scale.ts web/src/components/admin/charts/scale.test.ts
git commit -m "feat: add analytics api client and chart scale helpers"
```

---

## Task 10: Chart components (StatTile, AreaChart, BarChart)

**Files:**
- Create: `web/src/components/admin/charts/StatTile.tsx`
- Create: `web/src/components/admin/charts/AreaChart.tsx`
- Create: `web/src/components/admin/charts/BarChart.tsx`

**Interfaces:**
- Consumes: `scale.ts` helpers, existing CSS tokens (`--color-accent`, `--color-surface-up`, `--color-muted`, `--color-cream`, `--color-border`).
- Produces:
  - `StatTile({ label, value, hint? }: { label: string; value: string; hint?: string })`
  - `AreaChart({ points, valueKey, label }: { points: DailyPoint[]; valueKey: 'views' | 'watch_hours'; label: string })`
  - `BarChart({ rows }: { rows: { label: string; value: number; sub?: string }[] })`

> Design per dataviz skill: single-series area in the accent hue (sequential, one hue) with a crosshair+tooltip on hover; horizontal bars in one accent hue; no dual-axis; empty-state text; SVG has `role="img"` + `<title>`/`aria-label`; values use ink tokens, not the series color. Charts use `viewBox` + `preserveAspectRatio` and `width:100%` so they are responsive; wrap wide content in an `overflow-x:auto` container.

- [ ] **Step 1: Write StatTile**

`web/src/components/admin/charts/StatTile.tsx`:

```tsx
export default function StatTile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div
      style={{
        background: 'var(--color-surface-up)',
        border: '1px solid var(--color-border)',
        borderRadius: 12,
        padding: '16px 18px',
        display: 'flex',
        flexDirection: 'column',
        gap: 6,
      }}
    >
      <span style={{ fontSize: 13, color: 'var(--color-muted)' }}>{label}</span>
      <span style={{ fontSize: 30, fontWeight: 700, color: 'var(--color-cream)', lineHeight: 1.1 }}>{value}</span>
      {hint && <span style={{ fontSize: 12, color: 'var(--color-faint)' }}>{hint}</span>}
    </div>
  )
}
```

- [ ] **Step 2: Write AreaChart**

`web/src/components/admin/charts/AreaChart.tsx`:

```tsx
import { useState } from 'react'
import type { DailyPoint } from '../../../api/analytics'
import { areaPath, linePath, niceMax } from './scale'

const W = 640
const H = 200

export default function AreaChart({
  points,
  valueKey,
  label,
}: {
  points: DailyPoint[]
  valueKey: 'views' | 'watch_hours'
  label: string
}) {
  const [hover, setHover] = useState<number | null>(null)
  const values = points.map((p) => p[valueKey])
  const max = niceMax(Math.max(0, ...values))
  const hasData = values.some((v) => v > 0)

  if (!hasData) {
    return <EmptyChart label={label} />
  }

  const line = linePath(values, W, H, max)
  const area = areaPath(values, W, H, max)
  const n = values.length
  const xAt = (i: number) => (n <= 1 ? 0 : (i / (n - 1)) * W)

  return (
    <figure style={{ margin: 0 }}>
      <figcaption style={{ fontSize: 13, color: 'var(--color-muted)', marginBottom: 8 }}>{label}</figcaption>
      <div style={{ overflowX: 'auto' }}>
        <svg
          viewBox={`0 0 ${W} ${H}`}
          preserveAspectRatio="none"
          role="img"
          aria-label={label}
          style={{ width: '100%', height: 200, display: 'block' }}
          onMouseLeave={() => setHover(null)}
          onMouseMove={(e) => {
            const rect = (e.currentTarget as SVGSVGElement).getBoundingClientRect()
            const ratio = (e.clientX - rect.left) / rect.width
            setHover(Math.max(0, Math.min(n - 1, Math.round(ratio * (n - 1)))))
          }}
        >
          <title>{label}</title>
          <path d={area} fill="var(--color-accent)" fillOpacity={0.15} />
          <path d={line} fill="none" stroke="var(--color-accent)" strokeWidth={2} vectorEffect="non-scaling-stroke" />
          {hover !== null && (
            <line x1={xAt(hover)} y1={0} x2={xAt(hover)} y2={H} stroke="var(--color-border)" strokeWidth={1} vectorEffect="non-scaling-stroke" />
          )}
        </svg>
      </div>
      {hover !== null && (
        <div style={{ fontSize: 12, color: 'var(--color-cream)', marginTop: 6 }}>
          {points[hover].date} · {valueKey === 'watch_hours' ? `${points[hover].watch_hours} 小時` : `${points[hover].views} 次`}
        </div>
      )}
    </figure>
  )
}

function EmptyChart({ label }: { label: string }) {
  return (
    <figure style={{ margin: 0 }}>
      <figcaption style={{ fontSize: 13, color: 'var(--color-muted)', marginBottom: 8 }}>{label}</figcaption>
      <div
        style={{
          height: 200,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'var(--color-faint)',
          fontSize: 13,
          border: '1px dashed var(--color-border)',
          borderRadius: 12,
        }}
      >
        尚無觀看資料,開始播放後這裡就會有數據
      </div>
    </figure>
  )
}
```

- [ ] **Step 3: Write BarChart**

`web/src/components/admin/charts/BarChart.tsx`:

```tsx
import { barWidths } from './scale'

export interface BarRow {
  label: string
  value: number
  sub?: string
}

export default function BarChart({ rows }: { rows: BarRow[] }) {
  if (rows.length === 0) {
    return <div style={{ color: 'var(--color-faint)', fontSize: 13, padding: '12px 0' }}>尚無資料</div>
  }
  const max = Math.max(0, ...rows.map((r) => r.value))
  const widths = barWidths(rows.map((r) => r.value), max, 100)

  return (
    <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'flex', flexDirection: 'column', gap: 10 }}>
      {rows.map((r, i) => (
        <li key={`${r.label}-${i}`} style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, color: 'var(--color-cream)' }}>
            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '75%' }}>{r.label}</span>
            <span style={{ color: 'var(--color-muted)' }}>{r.sub ?? r.value}</span>
          </div>
          <div style={{ height: 8, background: 'var(--color-surface-up)', borderRadius: 4 }}>
            <div style={{ width: `${widths[i]}%`, height: 8, background: 'var(--color-accent)', borderRadius: 4 }} />
          </div>
        </li>
      ))}
    </ul>
  )
}
```

- [ ] **Step 4: Typecheck + lint**

Run: `cd web && npx tsc --noEmit && npx eslint src/components/admin/charts/`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/admin/charts/StatTile.tsx web/src/components/admin/charts/AreaChart.tsx web/src/components/admin/charts/BarChart.tsx
git commit -m "feat: add inline-svg stat tile, area chart, and bar chart components"
```

---

## Task 11: AnalyticsPage + route + nav enable

**Files:**
- Create: `web/src/pages/admin/AnalyticsPage.tsx`
- Create: `web/src/pages/admin/AnalyticsPage.test.tsx`
- Modify: `web/src/App.tsx`, `web/src/lib/adminNav.ts`

**Interfaces:**
- Consumes: `getAnalytics`, chart components, `useLocation` (per CLAUDE.md: window selector drives refetch via `days` state; a same-path re-nav isn't required here since data is deterministic by `days`).

- [ ] **Step 1: Look at an existing admin page for shell/pattern**

Read `web/src/pages/admin/MediaSourcePage.tsx` (or `UserManagePage.tsx`) to match the page container, heading, and loading/error conventions. Reuse them; do not invent a new shell.

- [ ] **Step 2: Write the failing page test**

`web/src/pages/admin/AnalyticsPage.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import AnalyticsPage from './AnalyticsPage'
import * as api from '../../api/analytics'

const empty: api.AnalyticsSummary = {
  range_days: 30,
  total_views: 0,
  total_watch_hours: 0,
  avg_completion_rate: 0,
  active_users: 0,
  daily_trend: Array.from({ length: 30 }, (_, i) => ({ date: `2026-06-${String(i + 1).padStart(2, '0')}`, views: 0, watch_hours: 0 })),
  top_videos: [],
  top_tags: [],
}

describe('AnalyticsPage', () => {
  beforeEach(() => vi.restoreAllMocks())

  it('renders KPI values from the API', async () => {
    vi.spyOn(api, 'getAnalytics').mockResolvedValue({ ...empty, total_views: 42, active_users: 3 })
    render(<MemoryRouter><AnalyticsPage /></MemoryRouter>)
    await waitFor(() => expect(screen.getByText('42')).toBeInTheDocument())
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('shows the empty chart state when there is no watch data', async () => {
    vi.spyOn(api, 'getAnalytics').mockResolvedValue(empty)
    render(<MemoryRouter><AnalyticsPage /></MemoryRouter>)
    await waitFor(() => expect(screen.getAllByText(/尚無觀看資料/).length).toBeGreaterThan(0))
  })
})
```

> Confirm the test setup matches sibling admin page tests (jsdom env, `@testing-library/jest-dom` import). If siblings import a shared test setup, mirror it.

- [ ] **Step 3: Run to verify it fails**

Run: `cd web && npx vitest run src/pages/admin/AnalyticsPage.test.tsx`
Expected: FAIL — cannot find `./AnalyticsPage`.

- [ ] **Step 4: Write the page**

`web/src/pages/admin/AnalyticsPage.tsx`:

```tsx
import { useEffect, useState } from 'react'
import { getAnalytics, type AnalyticsSummary } from '../../api/analytics'
import StatTile from '../../components/admin/charts/StatTile'
import AreaChart from '../../components/admin/charts/AreaChart'
import BarChart from '../../components/admin/charts/BarChart'

const RANGES = [7, 30, 90] as const

export default function AnalyticsPage() {
  const [days, setDays] = useState<number>(30)
  const [data, setData] = useState<AnalyticsSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getAnalytics(days)
      .then((res) => {
        if (cancelled) return
        setData(res)
        setError('')
      })
      .catch(() => !cancelled && setError('無法載入分析資料'))
      .finally(() => !cancelled && setLoading(false))
    return () => {
      cancelled = true
    }
  }, [days])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 12 }}>
        <h1 style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-cream)', margin: 0 }}>分析</h1>
        <div style={{ display: 'flex', gap: 6 }}>
          {RANGES.map((r) => (
            <button
              key={r}
              onClick={() => setDays(r)}
              style={{
                padding: '6px 12px',
                borderRadius: 8,
                border: '1px solid var(--color-border)',
                background: days === r ? 'var(--color-accent)' : 'transparent',
                color: days === r ? 'var(--color-accent-ink)' : 'var(--color-muted)',
                cursor: 'pointer',
                fontSize: 13,
              }}
            >
              近 {r} 天
            </button>
          ))}
        </div>
      </div>

      {error && <div style={{ color: 'var(--color-fav)' }}>{error}</div>}
      {loading && !data && <div style={{ color: 'var(--color-muted)' }}>載入中…</div>}

      {data && (
        <>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 12 }}>
            <StatTile label="總觀看次數" value={String(data.total_views)} />
            <StatTile label="觀看時長" value={`${data.total_watch_hours} 小時`} />
            <StatTile label="平均完播率" value={`${Math.round(data.avg_completion_rate * 100)}%`} />
            <StatTile label="活躍使用者" value={String(data.active_users)} />
          </div>

          <section style={panelStyle}>
            <AreaChart points={data.daily_trend} valueKey="watch_hours" label={`近 ${data.range_days} 天觀看時長`} />
          </section>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 16 }}>
            <section style={panelStyle}>
              <h2 style={panelTitle}>熱門影片</h2>
              <BarChart rows={data.top_videos.map((v) => ({ label: v.title, value: v.watch_hours, sub: `${v.watch_hours} 小時` }))} />
            </section>
            <section style={panelStyle}>
              <h2 style={panelTitle}>熱門標籤</h2>
              <BarChart rows={data.top_tags.map((t) => ({ label: t.name, value: t.views, sub: `${t.views} 次` }))} />
            </section>
          </div>
        </>
      )}
    </div>
  )
}

const panelStyle: React.CSSProperties = {
  background: 'var(--color-surface)',
  border: '1px solid var(--color-border)',
  borderRadius: 14,
  padding: 18,
}
const panelTitle: React.CSSProperties = { fontSize: 15, fontWeight: 600, color: 'var(--color-cream)', margin: '0 0 12px' }
```

- [ ] **Step 5: Run to verify it passes**

Run: `cd web && npx vitest run src/pages/admin/AnalyticsPage.test.tsx`
Expected: PASS (2 tests).

- [ ] **Step 6: Register the route**

In `web/src/App.tsx`: add the import near the other admin page imports:

```tsx
import AnalyticsPage from './pages/admin/AnalyticsPage'
```

Add the child route in the admin children array (next to `/admin/users`):

```tsx
                  { path: '/admin/analytics', element: <AnalyticsPage /> },
```

- [ ] **Step 7: Enable the nav item**

In `web/src/lib/adminNav.ts`, flip analytics to enabled and drop the "即將推出" comment nuance:

```ts
  { key: 'analytics', label: '分析', path: '/admin/analytics', enabled: true, icon: 'chart' },
```

Update the file's top comment from "6 項。analytics 尚未實作 → enabled:false" to reflect all six are live.

- [ ] **Step 8: Frontend gate**

Run: `cd web && npx tsc --noEmit && npx eslint src && npx vitest run`
Expected: no type/lint errors; all tests PASS.

- [ ] **Step 9: Commit**

```bash
git add web/src/pages/admin/AnalyticsPage.tsx web/src/pages/admin/AnalyticsPage.test.tsx web/src/App.tsx web/src/lib/adminNav.ts
git commit -m "feat: add admin analytics page and enable analytics nav"
```

---

## Task 12: Integration test (heartbeat → analytics e2e) + full verify

**Files:**
- Modify: `scripts/test_all.sh` (or the suite it drives — inspect first)

**Interfaces:**
- Consumes: running API from `task test-integration` stack.

- [ ] **Step 1: Inspect the integration harness**

Read `scripts/test_all.sh` and any helper it sources. Identify how it authenticates (admin + viewer tokens), how it creates/imports a video, and its assertion style (curl + jq). Mirror it — do not introduce a new framework.

- [ ] **Step 2: Add the e2e assertion**

After a video exists and a viewer token is available, add:

1. POST `/api/watch-sessions/heartbeat` with a fixed `session_id` (uuid), `played_delta: 15`, `position_seconds: 15` as the viewer — expect `204`.
2. POST the same `session_id` again with `played_delta: 15`, `position_seconds: 30` — expect `204` (accumulates).
3. GET `/api/admin/analytics?days=7` as admin — expect `200` and assert `data.total_views == 1`, `data.total_watch_hours` ≈ `0.0`–`0.1` (30s), `data.daily_trend` length `== 7`, and the video appears in `data.top_videos`.
4. Assert a viewer GET to `/api/admin/analytics` is `403` (Casbin).

Write the assertions in the harness's existing style (e.g. helper `assert_status`, `jq -e`).

- [ ] **Step 3: Run the fast gate**

Run: `task verify`
Expected: green (go vet/gofmt/go test + web tsc/eslint/vitest).

- [ ] **Step 4: Run the integration gate**

Run: `task test-integration`
Expected: green (includes the new heartbeat→analytics assertions).

> If `task test-integration` needs a clean stack, follow the project's documented flow (avoid the `down -v` volume pitfall noted in project memory). Migrations `015` apply automatically on API start.

- [ ] **Step 5: Commit**

```bash
git add scripts/test_all.sh
git commit -m "test: add heartbeat to analytics integration assertions"
```

---

## Self-Review

**Spec coverage:**
- watch_sessions table + fields → Task 1. ✅
- Heartbeat endpoint (viewer) + clamp + 404 + casbin → Tasks 3, 4. ✅
- Analytics endpoint (admin) + days/limit tunable → Tasks 5–7. ✅
- All 6 metrics (views, watch hours, completion, active users, daily trend, top videos, top tags) → Tasks 5–7 (repo/service) + 11 (render). ✅
- Client session_id + delta clamp + 15s heartbeat + beacon → Task 8. ✅
- Inline-SVG charts, no deps, dark tokens, empty state, a11y → Tasks 9–11. ✅
- Nav enable + route → Task 11. ✅
- No backfill / permanent retention → nothing to build (correctly absent). ✅
- Tests: unit (service/handler/vitest) + integration e2e → every task + Task 12. ✅

**Placeholder scan:** repo integration tests (Tasks 2, 5) and the integration harness edit (Task 12) intentionally defer to sibling conventions rather than inline full DB-harness code, because that harness is repo-specific and must be read first — each gives exact assertions to implement. No `TODO`/`TBD` in shipping code.

**Type consistency:** `HeartbeatInput`/`heartbeatRequest` field names align (session_id, video_id, played_delta, position_seconds). `AnalyticsSummary` Go JSON tags match the TS `AnalyticsSummary` interface field-for-field. `tag_id` is `int` in both Go and TS (SERIAL). `MaxHeartbeatDelta` (Go) = `MAX_HEARTBEAT_DELTA` (TS) = 22. Repo `DailyRaw` returns seconds; service converts to hours in one place (`round1`).

**Corrections applied vs spec:** spec §5.2 showed `top_tags.tag_id` as `"uuid"`; corrected to integer everywhere (tags.id is SERIAL).
