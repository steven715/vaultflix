# Playback Telemetry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist each playback session's terminal quality metrics (TTFF, rebuffer ratio, throughput, play_mode) into a new `playback_telemetry` table, with an admin aggregate endpoint, to build before/after streaming-performance baselines.

**Architecture:** Handler → Service → Repository, mirroring the existing `watch_sessions`/`analytics` layers. The frontend extends the already-built (but currently unmounted) `usePlaybackStats` hook with session-cumulative accumulators, mounts it in `PlayerPage`, and POSTs one summary on session end. The server derives `network_scope` from the request IP. One row per session (`session_id` UNIQUE → idempotent upsert).

**Tech Stack:** Go 1.24+ (gin, pgx v5), PostgreSQL 16 (golang-migrate), React 18 + TypeScript (vitest), bash integration suite.

**Design spec:** `docs/superpowers/specs/2026-07-06-playback-telemetry-design.md`

## Global Constraints

- Go: errors wrapped with `%w`; lowercase error messages, no trailing period; `log/slog` structured fields only.
- Strict layering: handler never writes SQL / touches MinIO; service never touches `*gin.Context`; repository never calls another repository.
- SQL: keywords uppercase, identifiers snake_case, parameterized (`$1`), each query a `const` at file top.
- Repository interfaces live in the `repository` package (matches `WatchSessionRepository`); service struct fields are interfaces.
- Mocks hand-written in `internal/mock/`; no third-party mock framework; no Go test connects to a live DB (repository SQL is covered by `scripts/test_*.sh`).
- Migrations: `NNN_description.up.sql` / `.down.sql`, `down` fully reversible.
- Success responses use `SuccessResponse{Data: ...}`; errors use `ErrorResponse{Error, Message}`. Status codes: 200 GET, 204 no-body write, 400 validation, 403 RBAC, 404 missing, 500 internal.
- Frontend: counters in `useRef` (never `useState`); no `setState` in effect cleanup; auto-retry/beacon paths bounded; page-leave writes use `keepalive` fetch.
- Go files ≤300 lines, functions ≤50 lines; imports in three groups (stdlib / third-party / project).
- Done = `task verify` green; because this adds a migration and touches the streaming path, also `task test-integration` green.

---

## File Structure

**Backend (create unless noted):**
- `migrations/017_create_playback_telemetry.up.sql` / `.down.sql`
- `internal/model/playback_telemetry.go` — `PlaybackTelemetryInput`, `TelemetryQuery`, `TelemetrySummary`, `PlayModeStats`
- `internal/repository/playback_telemetry_repo.go` — `PlaybackTelemetryRepository` interface + impl (Insert upsert, Aggregate)
- `internal/repository/playback_telemetry_repo_test.go` — structured-but-skipped (DB coverage deferred to integration), mirroring `watch_session_repo_test.go`
- `internal/mock/playback_telemetry_repo_mock.go`
- `internal/service/playback_telemetry_service.go` — `PlaybackTelemetryService` (Record, Summary), `classifyNetworkScope`
- `internal/service/playback_telemetry_service_test.go`
- `internal/handler/playback_telemetry_handler.go`
- `internal/handler/playback_telemetry_handler_test.go`
- `cmd/server/main.go` (modify) — wire repo/service/handler + 2 routes
- `casbin/policy.csv` (modify) — 1 viewer policy line

**Frontend (modify unless noted):**
- `web/src/utils/playbackStats.ts` — add `SessionSummary` type + `fatalFamilyOf`
- `web/src/utils/playbackStats.test.ts` — tests for `fatalFamilyOf`
- `web/src/hooks/usePlaybackStats.ts` — session accumulators + `getSessionSummary`; return `{ stats, getSessionSummary }`
- `web/src/api/telemetry.ts` (create) — `postPlaybackTelemetry` + `sendPlaybackTelemetryBeacon`
- `web/src/pages/PlayerPage.tsx` — mount hook, HUD toggle, `sendTelemetry` on session end
- `web/src/pages/PlayerPage.test.tsx` — assert telemetry beacon on unmount

**Integration:**
- `scripts/test_telemetry.sh` (create); `scripts/test_all.sh` (modify — add `telemetry` suite)

---

## Task 1: Migration — `playback_telemetry` table

**Files:**
- Create: `migrations/017_create_playback_telemetry.up.sql`
- Create: `migrations/017_create_playback_telemetry.down.sql`

**Interfaces:**
- Produces: table `playback_telemetry` with columns `id, user_id, video_id, session_id, play_mode, network_scope, ttff_ms, watched_ms, rebuffer_count, rebuffer_ms, avg_downlink_mbps, fatal_error_family, created_at`; unique index on `session_id`.

- [ ] **Step 1: Write the up migration**

`migrations/017_create_playback_telemetry.up.sql`:
```sql
CREATE TABLE playback_telemetry (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    video_id           UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    session_id         UUID NOT NULL,
    play_mode          TEXT NOT NULL,
    network_scope      TEXT NOT NULL,
    ttff_ms            INT,
    watched_ms         INT  NOT NULL DEFAULT 0,
    rebuffer_count     INT  NOT NULL DEFAULT 0,
    rebuffer_ms        INT  NOT NULL DEFAULT 0,
    avg_downlink_mbps  NUMERIC(8,2),
    fatal_error_family TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_playback_telemetry_session   ON playback_telemetry(session_id);
CREATE INDEX        idx_playback_telemetry_created   ON playback_telemetry(created_at DESC);
CREATE INDEX        idx_playback_telemetry_play_mode ON playback_telemetry(play_mode, created_at DESC);
```

- [ ] **Step 2: Write the down migration**

`migrations/017_create_playback_telemetry.down.sql`:
```sql
DROP TABLE IF EXISTS playback_telemetry;
```

- [ ] **Step 3: Verify the migration applies and reverses cleanly**

Run against a throwaway Postgres using the same `migrate/migrate` image compose uses:
```bash
docker compose up -d postgres
docker compose run --rm migrate up
docker compose run --rm migrate down 1
docker compose run --rm migrate up
```
Expected: `up` reports `17/u create_playback_telemetry`, `down 1` reverts it with no error, second `up` re-applies. (Authoritatively re-verified by `task test-integration` in Task 10.)

- [ ] **Step 4: Commit**

```bash
git add migrations/017_create_playback_telemetry.up.sql migrations/017_create_playback_telemetry.down.sql
git commit -m "feat: add playback_telemetry table (migration 017)"
```

---

## Task 2: Backend model, repository interface, mock, and service

Creates all shared model types, the repository contract + mock, and the fully unit-tested service (validation, clamping, IP classification, summary shaping). Repository SQL impl is Task 3.

**Files:**
- Create: `internal/model/playback_telemetry.go`
- Create: `internal/repository/playback_telemetry_repo.go` (interface only in this task — impl added in Task 3)
- Create: `internal/mock/playback_telemetry_repo_mock.go`
- Create: `internal/service/playback_telemetry_service.go`
- Test: `internal/service/playback_telemetry_service_test.go`

**Interfaces:**
- Produces:
  - `model.PlaybackTelemetryInput{ SessionID, UserID, VideoID, PlayMode, RemoteIP, NetworkScope string; TTFFMs *int; WatchedMs, RebufferCount, RebufferMs int; AvgDownlinkMbps *float64; FatalErrorFamily *string }`
  - `model.TelemetryQuery{ Days int; Scope string }`
  - `model.PlayModeStats{ PlayMode string; Sessions int64; TTFFP50Ms, TTFFP95Ms, RebufferRatio, AvgMbps *float64 }` (json tags: `play_mode, sessions, ttff_p50_ms, ttff_p95_ms, rebuffer_ratio, avg_mbps`)
  - `model.TelemetrySummary{ RangeDays int; Scope string; ByPlayMode []PlayModeStats }` (json: `range_days, scope, by_play_mode`)
  - `repository.PlaybackTelemetryRepository` interface: `Insert(ctx, model.PlaybackTelemetryInput) error`; `Aggregate(ctx, model.TelemetryQuery) ([]model.PlayModeStats, error)`
  - `service.NewPlaybackTelemetryService(repo repository.PlaybackTelemetryRepository) *service.PlaybackTelemetryService` with `Record(ctx, in) error` and `Summary(ctx, q) (*model.TelemetrySummary, error)`
- Consumes: `model.ErrInvalidInput`, `model.ErrNotFound`, `model.PlayModeDirect/Remux/Transcode`.

- [ ] **Step 1: Create the model file**

`internal/model/playback_telemetry.go`:
```go
package model

// PlaybackTelemetryInput is one viewing session's terminal quality summary,
// emitted once when the session ends. RemoteIP is the request source address;
// the service layer fills NetworkScope by classifying it.
type PlaybackTelemetryInput struct {
	SessionID        string
	UserID           string
	VideoID          string
	PlayMode         string
	RemoteIP         string
	NetworkScope     string
	TTFFMs           *int     // nil = never reached first frame
	WatchedMs        int      // actual playing time (rebuffer-ratio denominator)
	RebufferCount    int
	RebufferMs       int      // cumulative stall time (excludes initial buffer + seeks)
	AvgDownlinkMbps  *float64 // nil = no throughput samples
	FatalErrorFamily *string  // nil | "starved" | "codec"
}

// TelemetryQuery holds the tunable window and optional scope filter for an
// aggregate read. Scope == "" means all scopes.
type TelemetryQuery struct {
	Days  int
	Scope string
}

// PlayModeStats is one play_mode's aggregated quality metrics within the window.
// Pointer fields are null when no session contributed a value (e.g. no TTFF).
type PlayModeStats struct {
	PlayMode      string   `json:"play_mode"`
	Sessions      int64    `json:"sessions"`
	TTFFP50Ms     *float64 `json:"ttff_p50_ms"`
	TTFFP95Ms     *float64 `json:"ttff_p95_ms"`
	RebufferRatio *float64 `json:"rebuffer_ratio"`
	AvgMbps       *float64 `json:"avg_mbps"`
}

// TelemetrySummary is the payload returned by GET /api/admin/playback/telemetry.
type TelemetrySummary struct {
	RangeDays  int             `json:"range_days"`
	Scope      string          `json:"scope"`
	ByPlayMode []PlayModeStats `json:"by_play_mode"`
}
```

- [ ] **Step 2: Create the repository interface (impl comes in Task 3)**

`internal/repository/playback_telemetry_repo.go`:
```go
package repository

import (
	"context"

	"github.com/steven/vaultflix/internal/model"
)

// PlaybackTelemetryRepository persists per-session playback quality summaries
// and reads windowed aggregates over them.
type PlaybackTelemetryRepository interface {
	// Insert upserts one session summary keyed by session_id (last write wins),
	// so a page-leave beacon and an unmount POST for the same session collapse
	// to one row. It returns model.ErrNotFound when the referenced video does
	// not exist (no row is written).
	Insert(ctx context.Context, in model.PlaybackTelemetryInput) error
	// Aggregate returns per-play_mode metrics within the trailing q.Days window,
	// optionally filtered to q.Scope. An empty window yields an empty slice and
	// a nil error.
	Aggregate(ctx context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error)
}
```

- [ ] **Step 3: Create the mock**

`internal/mock/playback_telemetry_repo_mock.go`:
```go
package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

// PlaybackTelemetryRepository is a hand-written mock for
// repository.PlaybackTelemetryRepository. Set each Func field in tests.
type PlaybackTelemetryRepository struct {
	InsertFunc    func(ctx context.Context, in model.PlaybackTelemetryInput) error
	AggregateFunc func(ctx context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error)
}

func (m *PlaybackTelemetryRepository) Insert(ctx context.Context, in model.PlaybackTelemetryInput) error {
	if m.InsertFunc == nil {
		return fmt.Errorf("mock: InsertFunc not set")
	}
	return m.InsertFunc(ctx, in)
}

func (m *PlaybackTelemetryRepository) Aggregate(ctx context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error) {
	if m.AggregateFunc == nil {
		return nil, fmt.Errorf("mock: AggregateFunc not set")
	}
	return m.AggregateFunc(ctx, q)
}
```

- [ ] **Step 4: Write the failing service test**

`internal/service/playback_telemetry_service_test.go`:
```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func ptrInt(v int) *int { return &v }

func TestClassifyNetworkScope(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want string
	}{
		{"loopback v4", "127.0.0.1", "lan"},
		{"loopback v6", "::1", "lan"},
		{"rfc1918 10", "10.1.2.3", "lan"},
		{"rfc1918 172.16", "172.16.5.5", "lan"},
		{"rfc1918 172.31", "172.31.255.1", "lan"},
		{"rfc1918 192.168", "192.168.0.7", "lan"},
		{"ula v6", "fc00::1", "lan"},
		{"link-local v4", "169.254.1.1", "lan"},
		{"link-local v6", "fe80::1", "lan"},
		{"public v4", "8.8.8.8", "external"},
		{"public v6", "2606:4700:4700::1111", "external"},
		{"empty", "", "unknown"},
		{"garbage", "not-an-ip", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyNetworkScope(tc.ip); got != tc.want {
				t.Fatalf("classifyNetworkScope(%q) = %q, want %q", tc.ip, got, tc.want)
			}
		})
	}
}

func TestRecord_ValidUpsertsWithDerivedScope(t *testing.T) {
	var captured model.PlaybackTelemetryInput
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, in model.PlaybackTelemetryInput) error {
			captured = in
			return nil
		},
	}
	svc := NewPlaybackTelemetryService(repo)
	err := svc.Record(context.Background(), model.PlaybackTelemetryInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1",
		PlayMode: "remux", RemoteIP: "8.8.8.8", TTFFMs: ptrInt(1200), WatchedMs: 60000,
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if captured.NetworkScope != "external" {
		t.Fatalf("NetworkScope = %q, want external", captured.NetworkScope)
	}
}

func TestRecord_InvalidPlayMode(t *testing.T) {
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, _ model.PlaybackTelemetryInput) error {
			t.Fatal("Insert must not be called for invalid input")
			return nil
		},
	}
	svc := NewPlaybackTelemetryService(repo)
	err := svc.Record(context.Background(), model.PlaybackTelemetryInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1", PlayMode: "bogus", RemoteIP: "127.0.0.1",
	})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestRecord_MissingIDs(t *testing.T) {
	svc := NewPlaybackTelemetryService(&mock.PlaybackTelemetryRepository{})
	err := svc.Record(context.Background(), model.PlaybackTelemetryInput{PlayMode: "direct"})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestRecord_ClampsNegatives(t *testing.T) {
	var captured model.PlaybackTelemetryInput
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, in model.PlaybackTelemetryInput) error {
			captured = in
			return nil
		},
	}
	svc := NewPlaybackTelemetryService(repo)
	neg := -50
	err := svc.Record(context.Background(), model.PlaybackTelemetryInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1", PlayMode: "direct", RemoteIP: "10.0.0.1",
		WatchedMs: -10, RebufferCount: -1, RebufferMs: -5, TTFFMs: &neg,
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if captured.WatchedMs != 0 || captured.RebufferCount != 0 || captured.RebufferMs != 0 || *captured.TTFFMs != 0 {
		t.Fatalf("negatives not clamped: %+v", captured)
	}
}

func TestRecord_PropagatesNotFound(t *testing.T) {
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, _ model.PlaybackTelemetryInput) error {
			return model.ErrNotFound
		},
	}
	svc := NewPlaybackTelemetryService(repo)
	err := svc.Record(context.Background(), model.PlaybackTelemetryInput{
		SessionID: "s1", UserID: "u1", VideoID: "ghost", PlayMode: "direct", RemoteIP: "10.0.0.1",
	})
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestSummary_ShapesQueryResult(t *testing.T) {
	ratio := 0.05
	repo := &mock.PlaybackTelemetryRepository{
		AggregateFunc: func(_ context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error) {
			if q.Days != 7 || q.Scope != "external" {
				t.Fatalf("query not passed through: %+v", q)
			}
			return []model.PlayModeStats{{PlayMode: "remux", Sessions: 3, RebufferRatio: &ratio}}, nil
		},
	}
	svc := NewPlaybackTelemetryService(repo)
	out, err := svc.Summary(context.Background(), model.TelemetryQuery{Days: 7, Scope: "external"})
	if err != nil {
		t.Fatalf("Summary error: %v", err)
	}
	if out.RangeDays != 7 || out.Scope != "external" || len(out.ByPlayMode) != 1 || out.ByPlayMode[0].PlayMode != "remux" {
		t.Fatalf("unexpected summary: %+v", out)
	}
}
```

- [ ] **Step 5: Run the test to verify it fails**

Run: `go test ./internal/service/ -run 'TestClassifyNetworkScope|TestRecord|TestSummary' -v`
Expected: FAIL — `undefined: NewPlaybackTelemetryService` / `classifyNetworkScope`.

- [ ] **Step 6: Write the service implementation**

`internal/service/playback_telemetry_service.go`:
```go
package service

import (
	"context"
	"fmt"
	"net"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// validPlayModes is the closed set the ingest endpoint accepts.
var validPlayModes = map[string]bool{
	string(model.PlayModeDirect):    true,
	string(model.PlayModeRemux):     true,
	string(model.PlayModeTranscode): true,
}

type PlaybackTelemetryService struct {
	repo repository.PlaybackTelemetryRepository
}

func NewPlaybackTelemetryService(repo repository.PlaybackTelemetryRepository) *PlaybackTelemetryService {
	return &PlaybackTelemetryService{repo: repo}
}

// Record validates and clamps one session summary, derives NetworkScope from
// RemoteIP, then upserts it. Returns model.ErrInvalidInput for a missing id or
// unknown play_mode, and propagates model.ErrNotFound when the video is gone.
func (s *PlaybackTelemetryService) Record(ctx context.Context, in model.PlaybackTelemetryInput) error {
	if in.SessionID == "" || in.UserID == "" || in.VideoID == "" {
		return fmt.Errorf("telemetry requires session/user/video: %w", model.ErrInvalidInput)
	}
	if !validPlayModes[in.PlayMode] {
		return fmt.Errorf("invalid play_mode %q: %w", in.PlayMode, model.ErrInvalidInput)
	}
	in.NetworkScope = classifyNetworkScope(in.RemoteIP)
	clampTelemetry(&in)
	if err := s.repo.Insert(ctx, in); err != nil {
		return fmt.Errorf("failed to record telemetry: %w", err)
	}
	return nil
}

// Summary returns per-play_mode aggregates for the (handler-clamped) window.
func (s *PlaybackTelemetryService) Summary(ctx context.Context, q model.TelemetryQuery) (*model.TelemetrySummary, error) {
	stats, err := s.repo.Aggregate(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate telemetry: %w", err)
	}
	return &model.TelemetrySummary{RangeDays: q.Days, Scope: q.Scope, ByPlayMode: stats}, nil
}

// clampTelemetry floors negative counters/durations to 0, defending against a
// buggy or hostile client. Nil optional fields pass through untouched.
func clampTelemetry(in *model.PlaybackTelemetryInput) {
	if in.WatchedMs < 0 {
		in.WatchedMs = 0
	}
	if in.RebufferCount < 0 {
		in.RebufferCount = 0
	}
	if in.RebufferMs < 0 {
		in.RebufferMs = 0
	}
	if in.TTFFMs != nil && *in.TTFFMs < 0 {
		*in.TTFFMs = 0
	}
	if in.AvgDownlinkMbps != nil && *in.AvgDownlinkMbps < 0 {
		*in.AvgDownlinkMbps = 0
	}
}

// classifyNetworkScope maps a request source IP to a coarse network location.
// Loopback, private (RFC1918), unique-local (fc00::/7) and link-local addresses
// are "lan"; any other routable address is "external"; an empty or unparseable
// value is "unknown".
func classifyNetworkScope(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "unknown"
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
		return "lan"
	}
	return "external"
}
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test ./internal/service/ -run 'TestClassifyNetworkScope|TestRecord|TestSummary' -v`
Expected: PASS (all subtests).

- [ ] **Step 8: Commit**

```bash
git add internal/model/playback_telemetry.go internal/repository/playback_telemetry_repo.go internal/mock/playback_telemetry_repo_mock.go internal/service/playback_telemetry_service.go internal/service/playback_telemetry_service_test.go
git commit -m "feat: add playback telemetry model, repo contract, and service"
```

---

## Task 3: Repository SQL implementation

Adds the real upsert + aggregate SQL. Per repo convention there is no live-DB Go test; a structured-but-skipped test documents intended assertions (Task 10 exercises the SQL via the HTTP integration suite).

**Files:**
- Modify: `internal/repository/playback_telemetry_repo.go` (add impl to the interface file)
- Create: `internal/repository/playback_telemetry_repo_test.go`

**Interfaces:**
- Consumes: `model.PlaybackTelemetryInput`, `model.TelemetryQuery`, `model.PlayModeStats`, `model.ErrNotFound`.
- Produces: `repository.NewPlaybackTelemetryRepository(pool *pgxpool.Pool) PlaybackTelemetryRepository`.

- [ ] **Step 1: Add the impl, queries, and constructor**

Append to `internal/repository/playback_telemetry_repo.go` (add imports `fmt` and `github.com/jackc/pgx/v5/pgxpool`):
```go
// queryInsertTelemetry upserts one session summary. It SELECTs video_id from
// videos so a missing video yields no row (RowsAffected == 0 → ErrNotFound),
// matching the watch_sessions upsert pattern. session_id is UNIQUE, so a
// re-send (beacon + unmount) overwrites the prior row.
const queryInsertTelemetry = `
    INSERT INTO playback_telemetry
        (user_id, video_id, session_id, play_mode, network_scope,
         ttff_ms, watched_ms, rebuffer_count, rebuffer_ms, avg_downlink_mbps, fatal_error_family)
    SELECT $1, v.id, $3, $4, $5, $6, $7, $8, $9, $10, $11
    FROM videos v
    WHERE v.id = $2
    ON CONFLICT (session_id) DO UPDATE SET
        play_mode          = EXCLUDED.play_mode,
        network_scope      = EXCLUDED.network_scope,
        ttff_ms            = EXCLUDED.ttff_ms,
        watched_ms         = EXCLUDED.watched_ms,
        rebuffer_count     = EXCLUDED.rebuffer_count,
        rebuffer_ms        = EXCLUDED.rebuffer_ms,
        avg_downlink_mbps  = EXCLUDED.avg_downlink_mbps,
        fatal_error_family = EXCLUDED.fatal_error_family
`

// queryAggregateTelemetry returns per-play_mode metrics over the trailing
// window. percentile_cont skips NULL ttff_ms rows; casts to float8 keep the
// scan targets *float64. $2 = '' disables the scope filter.
const queryAggregateTelemetry = `
    SELECT play_mode,
           COUNT(*) AS sessions,
           percentile_cont(0.5)  WITHIN GROUP (ORDER BY ttff_ms)::float8 AS ttff_p50_ms,
           percentile_cont(0.95) WITHIN GROUP (ORDER BY ttff_ms)::float8 AS ttff_p95_ms,
           (SUM(rebuffer_ms)::float8 / NULLIF(SUM(watched_ms + rebuffer_ms), 0)) AS rebuffer_ratio,
           AVG(avg_downlink_mbps)::float8 AS avg_mbps
    FROM playback_telemetry
    WHERE created_at >= NOW() - make_interval(days => $1)
      AND ($2 = '' OR network_scope = $2)
    GROUP BY play_mode
    ORDER BY play_mode
`

type playbackTelemetryRepository struct {
	pool *pgxpool.Pool
}

func NewPlaybackTelemetryRepository(pool *pgxpool.Pool) PlaybackTelemetryRepository {
	return &playbackTelemetryRepository{pool: pool}
}

func (r *playbackTelemetryRepository) Insert(ctx context.Context, in model.PlaybackTelemetryInput) error {
	result, err := r.pool.Exec(ctx, queryInsertTelemetry,
		in.UserID, in.VideoID, in.SessionID, in.PlayMode, in.NetworkScope,
		in.TTFFMs, in.WatchedMs, in.RebufferCount, in.RebufferMs, in.AvgDownlinkMbps, in.FatalErrorFamily)
	if err != nil {
		return fmt.Errorf("failed to insert playback telemetry %s: %w", in.SessionID, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *playbackTelemetryRepository) Aggregate(ctx context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error) {
	rows, err := r.pool.Query(ctx, queryAggregateTelemetry, q.Days, q.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to query telemetry aggregate: %w", err)
	}
	defer rows.Close()

	stats := make([]model.PlayModeStats, 0)
	for rows.Next() {
		var s model.PlayModeStats
		if err := rows.Scan(&s.PlayMode, &s.Sessions, &s.TTFFP50Ms, &s.TTFFP95Ms, &s.RebufferRatio, &s.AvgMbps); err != nil {
			return nil, fmt.Errorf("failed to scan telemetry aggregate row: %w", err)
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate telemetry aggregate rows: %w", err)
	}
	return stats, nil
}
```

Update the file's import block to:
```go
import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)
```

- [ ] **Step 2: Create the structured-but-skipped repo test**

`internal/repository/playback_telemetry_repo_test.go`:
```go
package repository

import "testing"

// No Go test in this repo connects to a live Postgres pool (see
// watch_session_repo_test.go for the rationale). The DB-backed assertions for
// playback_telemetry are exercised at the HTTP level by
// scripts/test_telemetry.sh via `task test-integration`.
//
// Intended assertions once wired to a real DB pool:
//  1. Insert with a seeded real video writes one row; a second Insert with the
//     same session_id (different metrics) overwrites it — COUNT stays 1
//     (ON CONFLICT (session_id) DO UPDATE).
//  2. Insert with a random/missing video_id returns model.ErrNotFound and
//     leaves no row (the SELECT ... FROM videos WHERE v.id = $2 yields nothing).
//  3. Aggregate groups by play_mode, computes ttff percentiles ignoring NULL
//     ttff_ms rows, and returns rebuffer_ratio = SUM(rebuffer_ms)/SUM(watched+rebuffer).
//  4. Aggregate with Scope="external" filters to external rows; Scope="" returns all.
func TestPlaybackTelemetryRepository_Deferred(t *testing.T) {
	t.Skip("DB-backed; covered by scripts/test_telemetry.sh (task test-integration)")
}
```

- [ ] **Step 3: Verify it builds and vets**

Run: `go vet ./internal/repository/ && go test ./internal/repository/ -run TestPlaybackTelemetryRepository_Deferred -v`
Expected: build OK; test reports SKIP.

- [ ] **Step 4: Commit**

```bash
git add internal/repository/playback_telemetry_repo.go internal/repository/playback_telemetry_repo_test.go
git commit -m "feat: implement playback telemetry repository (upsert + aggregate SQL)"
```

---

## Task 4: HTTP handler

**Files:**
- Create: `internal/handler/playback_telemetry_handler.go`
- Test: `internal/handler/playback_telemetry_handler_test.go`

**Interfaces:**
- Consumes: `service.NewPlaybackTelemetryService`, `service.PlaybackTelemetryService`, `mock.PlaybackTelemetryRepository`, `clampParam` (already in `handler` package from `analytics_handler.go`).
- Produces: `handler.NewPlaybackTelemetryHandler(svc *service.PlaybackTelemetryService) *PlaybackTelemetryHandler` with `Record(c)` and `Summary(c)`.

- [ ] **Step 1: Write the failing handler test**

`internal/handler/playback_telemetry_handler_test.go`:
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

func setupTelemetryRouter(repo *mock.PlaybackTelemetryRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewPlaybackTelemetryService(repo)
	h := NewPlaybackTelemetryHandler(svc)
	r := gin.New()
	// Inject a user_id like the JWT middleware would.
	r.Use(func(c *gin.Context) { c.Set("user_id", "u1"); c.Next() })
	r.POST("/api/playback/telemetry", h.Record)
	r.GET("/api/admin/playback/telemetry", h.Summary)
	return r
}

func TestTelemetryRecord_Success(t *testing.T) {
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, _ model.PlaybackTelemetryInput) error { return nil },
	}
	r := setupTelemetryRouter(repo)
	body := `{"session_id":"s1","video_id":"v1","play_mode":"direct","ttff_ms":900,"watched_ms":5000}`
	req := httptest.NewRequest(http.MethodPost, "/api/playback/telemetry", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d (%s)", w.Code, w.Body.String())
	}
}

func TestTelemetryRecord_MissingFields(t *testing.T) {
	r := setupTelemetryRouter(&mock.PlaybackTelemetryRepository{})
	req := httptest.NewRequest(http.MethodPost, "/api/playback/telemetry", bytes.NewBufferString(`{"session_id":"s1"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestTelemetryRecord_InvalidPlayMode(t *testing.T) {
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, _ model.PlaybackTelemetryInput) error {
			t.Fatal("Insert must not run for invalid play_mode")
			return nil
		},
	}
	r := setupTelemetryRouter(repo)
	body := `{"session_id":"s1","video_id":"v1","play_mode":"bogus"}`
	req := httptest.NewRequest(http.MethodPost, "/api/playback/telemetry", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestTelemetryRecord_VideoNotFound(t *testing.T) {
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, _ model.PlaybackTelemetryInput) error { return model.ErrNotFound },
	}
	r := setupTelemetryRouter(repo)
	body := `{"session_id":"s1","video_id":"ghost","play_mode":"direct"}`
	req := httptest.NewRequest(http.MethodPost, "/api/playback/telemetry", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestTelemetrySummary_Success(t *testing.T) {
	var gotDays int
	var gotScope string
	repo := &mock.PlaybackTelemetryRepository{
		AggregateFunc: func(_ context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error) {
			gotDays, gotScope = q.Days, q.Scope
			return []model.PlayModeStats{{PlayMode: "direct", Sessions: 2}}, nil
		},
	}
	r := setupTelemetryRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/playback/telemetry?days=7&scope=external", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if gotDays != 7 || gotScope != "external" {
		t.Fatalf("query not clamped/passed: days=%d scope=%q", gotDays, gotScope)
	}
	var resp struct {
		Data model.TelemetrySummary `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(resp.Data.ByPlayMode) != 1 || resp.Data.ByPlayMode[0].PlayMode != "direct" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestTelemetrySummary_BadScopeBecomesAll(t *testing.T) {
	var gotScope = "sentinel"
	repo := &mock.PlaybackTelemetryRepository{
		AggregateFunc: func(_ context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error) {
			gotScope = q.Scope
			return []model.PlayModeStats{}, nil
		},
	}
	r := setupTelemetryRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/playback/telemetry?scope=wat", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if gotScope != "" {
		t.Fatalf("want empty scope for unknown filter, got %q", gotScope)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/handler/ -run 'TestTelemetry' -v`
Expected: FAIL — `undefined: NewPlaybackTelemetryHandler`.

- [ ] **Step 3: Write the handler**

`internal/handler/playback_telemetry_handler.go`:
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

const (
	defaultTelemetryDays = 30
	minTelemetryDays     = 1
	maxTelemetryDays     = 365
)

type PlaybackTelemetryHandler struct {
	service *service.PlaybackTelemetryService
}

func NewPlaybackTelemetryHandler(svc *service.PlaybackTelemetryService) *PlaybackTelemetryHandler {
	return &PlaybackTelemetryHandler{service: svc}
}

type telemetryRequest struct {
	SessionID        string   `json:"session_id" binding:"required"`
	VideoID          string   `json:"video_id" binding:"required"`
	PlayMode         string   `json:"play_mode" binding:"required"`
	TTFFMs           *int     `json:"ttff_ms"`
	WatchedMs        int      `json:"watched_ms"`
	RebufferCount    int      `json:"rebuffer_count"`
	RebufferMs       int      `json:"rebuffer_ms"`
	AvgDownlinkMbps  *float64 `json:"avg_downlink_mbps"`
	FatalErrorFamily *string  `json:"fatal_error_family"`
}

// Record persists one playback session's terminal quality summary. Success is
// 204 (no body) — the write is an idempotent upsert, mirroring the heartbeat.
func (h *PlaybackTelemetryHandler) Record(c *gin.Context) {
	userID := c.GetString("user_id")

	var req telemetryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "session_id, video_id and play_mode are required",
		})
		return
	}

	err := h.service.Record(c.Request.Context(), model.PlaybackTelemetryInput{
		SessionID:        req.SessionID,
		UserID:           userID,
		VideoID:          req.VideoID,
		PlayMode:         req.PlayMode,
		RemoteIP:         c.ClientIP(),
		TTFFMs:           req.TTFFMs,
		WatchedMs:        req.WatchedMs,
		RebufferCount:    req.RebufferCount,
		RebufferMs:       req.RebufferMs,
		AvgDownlinkMbps:  req.AvgDownlinkMbps,
		FatalErrorFamily: req.FatalErrorFamily,
	})
	if err != nil {
		if errors.Is(err, model.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid telemetry"})
			return
		}
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "video not found"})
			return
		}
		slog.Error("failed to record playback telemetry", "error", err, "user_id", userID, "video_id", req.VideoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to record telemetry"})
		return
	}

	c.Status(http.StatusNoContent)
}

// Summary returns per-play_mode aggregates. days/scope are request-tunable.
func (h *PlaybackTelemetryHandler) Summary(c *gin.Context) {
	q := model.TelemetryQuery{
		Days:  clampParam(c.Query("days"), defaultTelemetryDays, minTelemetryDays, maxTelemetryDays),
		Scope: normalizeScope(c.Query("scope")),
	}

	summary, err := h.service.Summary(c.Request.Context(), q)
	if err != nil {
		slog.Error("failed to build telemetry summary", "error", err, "days", q.Days, "scope", q.Scope)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to build telemetry"})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{Data: summary})
}

// normalizeScope allows only known filters; anything else means "all scopes".
func normalizeScope(raw string) string {
	switch raw {
	case "lan", "external", "unknown":
		return raw
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/handler/ -run 'TestTelemetry' -v`
Expected: PASS (all subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/handler/playback_telemetry_handler.go internal/handler/playback_telemetry_handler_test.go
git commit -m "feat: add playback telemetry HTTP handler"
```

---

## Task 5: Wire into main.go + routes + RBAC policy

**Files:**
- Modify: `cmd/server/main.go` (repo/service/handler construction + 2 routes)
- Modify: `casbin/policy.csv` (viewer ingest permission)

**Interfaces:**
- Consumes: `repository.NewPlaybackTelemetryRepository`, `service.NewPlaybackTelemetryService`, `handler.NewPlaybackTelemetryHandler`.

- [ ] **Step 1: Construct the layers in main.go**

In `cmd/server/main.go`, after the `analyticsRepo := repository.NewAnalyticsRepository(pool)` line, add:
```go
	playbackTelemetryRepo := repository.NewPlaybackTelemetryRepository(pool)
```
After `analyticsService := service.NewAnalyticsService(analyticsRepo)`, add:
```go
	playbackTelemetryService := service.NewPlaybackTelemetryService(playbackTelemetryRepo)
```
After `analyticsHandler := handler.NewAnalyticsHandler(analyticsService)`, add:
```go
	playbackTelemetryHandler := handler.NewPlaybackTelemetryHandler(playbackTelemetryService)
```

- [ ] **Step 2: Register the routes**

In the protected `api` group, after `api.POST("/watch-sessions/heartbeat", watchSessionHandler.Heartbeat)`, add:
```go
		// Playback telemetry (viewer ingest; admin-only aggregate read)
		api.POST("/playback/telemetry", playbackTelemetryHandler.Record)
		api.GET("/admin/playback/telemetry", playbackTelemetryHandler.Summary)
```

- [ ] **Step 3: Add the viewer RBAC policy line**

Admin already matches `/api/*` (covers the GET summary). Viewers need explicit ingest permission. In `casbin/policy.csv`, after the line `p, viewer, /api/watch-sessions/heartbeat, POST`, add:
```csv
p, viewer, /api/playback/telemetry, POST
```

- [ ] **Step 4: Verify it builds**

Run: `go build ./... && go vet ./cmd/...`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/server/main.go casbin/policy.csv
git commit -m "feat: wire playback telemetry routes and viewer RBAC policy"
```

---

## Task 6: Frontend — pure helpers (`SessionSummary`, `fatalFamilyOf`)

**Files:**
- Modify: `web/src/utils/playbackStats.ts`
- Test: `web/src/utils/playbackStats.test.ts`

**Interfaces:**
- Produces:
  - `SessionSummary { ttffMs: number | null; watchedMs: number; rebufferCount: number; rebufferMs: number; avgDownlinkMbps: number | null; fatalErrorFamily: 'starved' | 'codec' | null }`
  - `fatalFamilyOf(phase: PlaybackPhase): 'starved' | 'codec' | null` — only the hard error phases are fatal; transient `buffering` is NOT (returns null), unlike `familyOf`.

- [ ] **Step 1: Add the failing test**

Append to `web/src/utils/playbackStats.test.ts`:
```ts
import { fatalFamilyOf } from './playbackStats'

describe('fatalFamilyOf', () => {
  const cases: { phase: PlaybackPhase; want: 'starved' | 'codec' | null }[] = [
    { phase: 'network-error', want: 'starved' },
    { phase: 'decode-error', want: 'codec' },
    { phase: 'unsupported', want: 'codec' },
    { phase: 'buffering', want: null }, // transient, not a fatal end state
    { phase: 'playing', want: null },
    { phase: 'paused', want: null },
    { phase: 'idle', want: null },
  ]
  cases.forEach(({ phase, want }) => {
    it(`maps ${phase} -> ${want}`, () => {
      expect(fatalFamilyOf(phase)).toBe(want)
    })
  })
})
```
(Add `fatalFamilyOf` to the existing top-of-file import from `./playbackStats`, or keep this separate import — both compile.)

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && npx vitest run src/utils/playbackStats.test.ts -t fatalFamilyOf`
Expected: FAIL — `fatalFamilyOf is not a function`.

- [ ] **Step 3: Implement in `playbackStats.ts`**

Append to `web/src/utils/playbackStats.ts`:
```ts
// SessionSummary is the terminal, session-cumulative playback quality report
// emitted once when a viewing ends. null means "not measured this session".
export interface SessionSummary {
  ttffMs: number | null
  watchedMs: number
  rebufferCount: number
  rebufferMs: number
  avgDownlinkMbps: number | null
  fatalErrorFamily: 'starved' | 'codec' | null
}

// fatalFamilyOf maps a phase to the fatal error family it represents, or null
// when the phase is not a fatal end state. Unlike familyOf, transient
// `buffering` is NOT fatal (it recovers) — only the hard MediaError phases are.
export function fatalFamilyOf(phase: PlaybackPhase): 'starved' | 'codec' | null {
  switch (phase) {
    case 'network-error':
      return 'starved'
    case 'decode-error':
    case 'unsupported':
      return 'codec'
    default:
      return null
  }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && npx vitest run src/utils/playbackStats.test.ts`
Expected: PASS (new + existing tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/utils/playbackStats.ts web/src/utils/playbackStats.test.ts
git commit -m "feat: add SessionSummary type and fatalFamilyOf helper"
```

---

## Task 7: Frontend — session accumulators in `usePlaybackStats`

Extends the hook with session-cumulative refs and a stable `getSessionSummary()`, reusing the existing `waiting`/`playing`/`canplay` listeners and buffer math (no second measurement system). Changes the return to `{ stats, getSessionSummary }`. No current consumer exists, so nothing breaks.

**Files:**
- Modify: `web/src/hooks/usePlaybackStats.ts`

**Interfaces:**
- Consumes: `SessionSummary`, `fatalFamilyOf` from `../utils/playbackStats`.
- Produces: `usePlaybackStats(videoRef, streamPath, avgBitrateBps): { stats: PlaybackStats; getSessionSummary: () => SessionSummary }`.

- [ ] **Step 1: Add imports and top-level accumulator refs**

In `web/src/hooks/usePlaybackStats.ts`, extend the import from `../utils/playbackStats` to also import `fatalFamilyOf` and `type SessionSummary`, and add `useCallback` to the `react` import.

Add these refs alongside the existing `rebufferRef` etc. (top-level of the hook, before the effect):
```ts
  const playStartRef = useRef<number | null>(null) // perf time of first play intent
  const ttffRef = useRef<number | null>(null)      // first-frame latency, set once
  const watchedMsRef = useRef(0)                    // wall-clock time spent in `playing`
  const stallStartRef = useRef<number | null>(null) // perf time the current stall began
  const rebufferMsRef = useRef(0)                   // cumulative stall duration
  const mbpsSumRef = useRef(0)
  const mbpsCountRef = useRef(0)
  const fatalFamilyRef = useRef<'starved' | 'codec' | null>(null)
  const lastTickRef = useRef<number | null>(null)   // perf time of previous publish
```

- [ ] **Step 2: Reset the new refs when the stream target changes**

At the top of the effect body (next to `rebufferRef.current = 0` etc.), add:
```ts
    playStartRef.current = null
    ttffRef.current = null
    watchedMsRef.current = 0
    stallStartRef.current = null
    rebufferMsRef.current = 0
    mbpsSumRef.current = 0
    mbpsCountRef.current = 0
    fatalFamilyRef.current = null
    lastTickRef.current = null
```

- [ ] **Step 3: Accumulate stall duration in the existing listeners**

Extend `onWaiting` to record the stall start (only for genuine rebuffers, matching the existing count condition):
```ts
    const onWaiting = () => {
      stalledRef.current = true
      // Count genuine rebuffers, not seek-induced waiting.
      if (boundEl && boundEl.currentTime > 0 && !boundEl.seeking) {
        rebufferRef.current += 1
        stallStartRef.current = performance.now()
      }
    }
```
Extend `onResume` to close the stall window:
```ts
    const onResume = () => {
      stalledRef.current = false
      if (stallStartRef.current != null) {
        rebufferMsRef.current += performance.now() - stallStartRef.current
        stallStartRef.current = null
      }
    }
```
Add a `play`-intent listener bound/unbound alongside the others. In `bind(el)` add `el.addEventListener('play', onPlay)` and in `unbind()` add `boundEl.removeEventListener('play', onPlay)`. Define `onPlay` next to `onWaiting`:
```ts
    const onPlay = () => {
      if (playStartRef.current == null) playStartRef.current = performance.now()
    }
```

- [ ] **Step 4: Accumulate TTFF, watched time, throughput avg, and fatal family in `publish`**

Inside `publish`, after `const now = performance.now()` and after the ranges/throughput/phase are computed (just before `setStats(...)`), add:
```ts
      // Watched wall-clock: accumulate time spent actually playing.
      if (lastTickRef.current != null && phase === 'playing') {
        watchedMsRef.current += now - lastTickRef.current
      }
      lastTickRef.current = now

      // TTFF: first advancing frame after a play intent, recorded once.
      if (ttffRef.current == null && playStartRef.current != null && el.currentTime > 0) {
        ttffRef.current = now - playStartRef.current
      }

      // Fatal end state (hard MediaError), sticky once seen.
      const fatal = fatalFamilyOf(phase)
      if (fatal) fatalFamilyRef.current = fatal
```
In the existing throughput block, where `inst` is computed and non-null, also accumulate the running average:
```ts
        if (inst != null) {
          mbpsRef.current =
            mbpsRef.current == null
              ? inst
              : MBPS_EWMA_ALPHA * inst + (1 - MBPS_EWMA_ALPHA) * mbpsRef.current
          mbpsSumRef.current += inst
          mbpsCountRef.current += 1
        }
```

- [ ] **Step 5: Add `getSessionSummary` and change the return**

Before the effect (or after it, but at hook top level), add a stable accessor:
```ts
  const getSessionSummary = useCallback(
    (): SessionSummary => ({
      ttffMs: ttffRef.current,
      watchedMs: Math.round(watchedMsRef.current),
      rebufferCount: rebufferRef.current,
      rebufferMs: Math.round(rebufferMsRef.current),
      avgDownlinkMbps: mbpsCountRef.current > 0 ? mbpsSumRef.current / mbpsCountRef.current : null,
      fatalErrorFamily: fatalFamilyRef.current,
    }),
    [],
  )
```
Change the final `return stats` to:
```ts
  return { stats, getSessionSummary }
```

- [ ] **Step 6: Verify typecheck and existing tests still pass**

Run: `cd web && npx tsc --noEmit && npx vitest run src/utils/playbackStats.test.ts`
Expected: no type errors; pure-helper tests still green. (The hook has no unit test; behavior is covered via `PlayerPage.test.tsx` in Task 9.)

- [ ] **Step 7: Commit**

```bash
git add web/src/hooks/usePlaybackStats.ts
git commit -m "feat: accumulate session-level playback metrics in usePlaybackStats"
```

---

## Task 8: Frontend — telemetry API client

**Files:**
- Create: `web/src/api/telemetry.ts`

**Interfaces:**
- Produces:
  - `PlaybackTelemetryPayload { session_id: string; video_id: string; play_mode: string; ttff_ms: number | null; watched_ms: number; rebuffer_count: number; rebuffer_ms: number; avg_downlink_mbps: number | null; fatal_error_family: string | null }`
  - `postPlaybackTelemetry(payload): Promise<void>`
  - `sendPlaybackTelemetryBeacon(payload): void` — keepalive fetch for page-leave/unmount.

- [ ] **Step 1: Write the module**

`web/src/api/telemetry.ts`:
```ts
import client from './client'

export interface PlaybackTelemetryPayload {
  session_id: string
  video_id: string
  play_mode: string
  ttff_ms: number | null
  watched_ms: number
  rebuffer_count: number
  rebuffer_ms: number
  avg_downlink_mbps: number | null
  fatal_error_family: string | null
}

export async function postPlaybackTelemetry(payload: PlaybackTelemetryPayload): Promise<void> {
  await client.post('/playback/telemetry', payload)
}

// sendPlaybackTelemetryBeacon posts on page-leave/unmount using keepalive so the
// request survives teardown (mirrors the heartbeat beacon in PlayerPage). The
// server upsert on session_id makes a duplicate with the normal path harmless.
export function sendPlaybackTelemetryBeacon(payload: PlaybackTelemetryPayload): void {
  const token = localStorage.getItem('token')
  fetch('/api/playback/telemetry', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(payload),
    keepalive: true,
  }).catch(() => {})
}
```

- [ ] **Step 2: Verify typecheck and lint**

Run: `cd web && npx tsc --noEmit && npx eslint src/api/telemetry.ts`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/api/telemetry.ts
git commit -m "feat: add playback telemetry api client"
```

---

## Task 9: Frontend — wire PlayerPage (mount hook, HUD toggle, send on end)

**Files:**
- Modify: `web/src/pages/PlayerPage.tsx`
- Test: `web/src/pages/PlayerPage.test.tsx`

**Interfaces:**
- Consumes: `usePlaybackStats` (new return shape), `NetworkHud`, `sendPlaybackTelemetryBeacon`, `useSearchParams`.

- [ ] **Step 1: Write the failing test (telemetry beacon on unmount)**

Add to `web/src/pages/PlayerPage.test.tsx`. First add the mock near the other `vi.mock` calls:
```ts
vi.mock('../api/telemetry')
```
and import it at the top with the others:
```ts
import * as telemetryApi from '../api/telemetry'
```
Then add a test (in a new or existing `describe`):
```ts
describe('PlayerPage telemetry', () => {
  beforeEach(() => {
    vi.mocked(videosApi.getVideo).mockResolvedValue({
      ...base,
      play_mode: 'direct',
    } as never)
  })

  it('sends a telemetry beacon on unmount after some playback', async () => {
    const { unmount } = renderPlayer()
    // Let the video load and the stats hook mount.
    await screen.findByText('T')

    const videoEl = document.querySelector('video') as HTMLVideoElement
    // Simulate first frame + playing so the session has measurable data.
    Object.defineProperty(videoEl, 'currentTime', { value: 5, configurable: true })
    fireEvent.play(videoEl)
    fireEvent.playing(videoEl)

    unmount()

    expect(telemetryApi.sendPlaybackTelemetryBeacon).toHaveBeenCalledTimes(1)
    const payload = vi.mocked(telemetryApi.sendPlaybackTelemetryBeacon).mock.calls[0][0]
    expect(payload.video_id).toBe('v1')
    expect(payload.play_mode).toBe('direct')
    expect(typeof payload.session_id).toBe('string')
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && npx vitest run src/pages/PlayerPage.test.tsx -t telemetry`
Expected: FAIL — beacon never called (wiring absent).

- [ ] **Step 3: Wire the hook, HUD toggle, and refs**

In `web/src/pages/PlayerPage.tsx`:

Add imports:
```ts
import { useSearchParams } from 'react-router-dom'
import { usePlaybackStats } from '../hooks/usePlaybackStats'
import NetworkHud from '../components/NetworkHud'
import { sendPlaybackTelemetryBeacon } from '../api/telemetry'
```
(Extend the existing `react-router-dom` import rather than duplicating it.)

Add a `playModeRef` next to `videoIDRef` and a `telemetrySentRef`:
```ts
  const playModeRef = useRef<string>('')
  const telemetrySentRef = useRef(false)
```

Compute stream inputs and mount the hook (place after `video`/`streamToken` state is declared, inside the component body):
```ts
  const avgBitrateBps =
    video && video.duration_seconds > 0
      ? (video.file_size_bytes * 8) / video.duration_seconds
      : null
  const streamPath = video
    ? video.play_mode === 'remux'
      ? `/api/videos/${video.id}/hls`
      : video.stream_url
    : null
  const { stats, getSessionSummary } = usePlaybackStats(videoRef, streamPath, avgBitrateBps)

  const [searchParams] = useSearchParams()
  const hudVisible =
    searchParams.get('hud') === '1' || localStorage.getItem('vaultflix-hud') === '1'
```

In `fetchVideo` (the `[id]` effect), where `videoIDRef.current = data.id` is set, also set:
```ts
        playModeRef.current = data.play_mode
        telemetrySentRef.current = false
```

- [ ] **Step 4: Add `sendTelemetry` and call it on session end**

Add a stable callback (near `flushHeartbeat`):
```ts
  // Emit the session's terminal quality summary once, on unmount / page leave.
  // Guarded so a session that never played sends nothing; the server upserts on
  // session_id, so a duplicate is harmless. Uses only refs (no setState here).
  const sendTelemetry = useCallback(() => {
    if (telemetrySentRef.current) return
    const vid = videoIDRef.current
    const sid = sessionIdRef.current
    const mode = playModeRef.current
    if (!vid || !sid || !mode) return
    const s = getSessionSummary()
    if (s.ttffMs == null && s.watchedMs <= 0) return
    telemetrySentRef.current = true
    sendPlaybackTelemetryBeacon({
      session_id: sid,
      video_id: vid,
      play_mode: mode,
      ttff_ms: s.ttffMs,
      watched_ms: s.watchedMs,
      rebuffer_count: s.rebufferCount,
      rebuffer_ms: s.rebufferMs,
      avg_downlink_mbps: s.avgDownlinkMbps,
      fatal_error_family: s.fatalErrorFamily,
    })
  }, [getSessionSummary])
```
In the `[id]` effect cleanup, next to `flushHeartbeat(true)`, add `sendTelemetry()`. Add `sendTelemetry` to that effect's dependency array (it is stable):
```ts
    return () => {
      cancelled = true
      sendProgressBeacon()
      flushHeartbeat(true)
      sendTelemetry()
    }
```
Update the dependency array from `}, [id, flushHeartbeat])` to `}, [id, flushHeartbeat, sendTelemetry])`.

- [ ] **Step 5: Render the HUD overlay behind the toggle**

Inside the direct/remux `<div className="relative overflow-hidden rounded-lg bg-black">` block, immediately after the `<video ... />` element, add:
```tsx
                {hudVisible && <NetworkHud stats={stats} />}
```

- [ ] **Step 6: Run to verify it passes**

Run: `cd web && npx vitest run src/pages/PlayerPage.test.tsx`
Expected: PASS (new telemetry test + existing PlayerPage tests).

- [ ] **Step 7: Typecheck and lint the changed files**

Run: `cd web && npx tsc --noEmit && npx eslint src/pages/PlayerPage.tsx src/hooks/usePlaybackStats.ts`
Expected: no errors/warnings.

- [ ] **Step 8: Commit**

```bash
git add web/src/pages/PlayerPage.tsx web/src/pages/PlayerPage.test.tsx
git commit -m "feat: mount playback stats, HUD toggle, and telemetry send in PlayerPage"
```

---

## Task 10: Integration test + full verification

**Files:**
- Create: `scripts/test_telemetry.sh`
- Modify: `scripts/test_all.sh` (add `telemetry` to `ALL_SUITES`)

**Interfaces:**
- Consumes: `scripts/test_helpers.sh` (`login_as`, `register_user`, `ensure_video`, `assert_eq`, `assert_not_empty`, `API_BASE`, `ADMIN_USER`, `ADMIN_PASS`).

- [ ] **Step 1: Write the integration script**

`scripts/test_telemetry.sh`:
```bash
#!/bin/bash
# =============================================================================
# Integration test: 播放遙測 ingest → 聚合讀取
# 測試項目: POST /api/playback/telemetry (204)、同 session_id upsert 不重複計數、
#           非法 play_mode (400)、缺 video (404)、GET /api/admin/playback/telemetry
#           聚合、viewer 讀 admin 端點 RBAC (403)
# 用 before/after DELTA 斷言 sessions，對「重跑/共用 DB」具隔離性。
# =============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/test_helpers.sh"

bold "=== Playback Telemetry 遙測 API 測試 ==="
check_prerequisites

ADMIN_TOKEN=$(login_as "$ADMIN_USER" "$ADMIN_PASS")
register_user "test_telemetry_viewer" "test1234" >/dev/null 2>&1 || true
VIEWER_TOKEN=$(login_as "test_telemetry_viewer" "test1234")

VIDEO_ID=$(ensure_video "$ADMIN_TOKEN")
assert_not_empty "取得影片 ID" "$VIDEO_ID"

SESSION_ID=$(cat /proc/sys/kernel/random/uuid)

# --- baseline: direct 分組的 sessions 數 ---
baseline_sessions() {
    curl -s "${API_BASE}/api/admin/playback/telemetry?days=7" \
        -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        | grep -o '"play_mode":"direct","sessions":[0-9]*' | grep -o '[0-9]*$' || echo 0
}
BEFORE=$(baseline_sessions)
[ -z "$BEFORE" ] && BEFORE=0

echo ""
bold "[1] POST /api/playback/telemetry — viewer ingest 回 204"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/playback/telemetry" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" -H "Content-Type: application/json" \
    -d "{\"session_id\":\"${SESSION_ID}\",\"video_id\":\"${VIDEO_ID}\",\"play_mode\":\"direct\",\"ttff_ms\":900,\"watched_ms\":60000,\"rebuffer_count\":1,\"rebuffer_ms\":500}")
assert_eq "ingest 回 204" "204" "$CODE"

echo ""
bold "[2] 同 session_id 再送一次 — upsert 不新增列"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/playback/telemetry" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" -H "Content-Type: application/json" \
    -d "{\"session_id\":\"${SESSION_ID}\",\"video_id\":\"${VIDEO_ID}\",\"play_mode\":\"direct\",\"ttff_ms\":950,\"watched_ms\":65000}")
assert_eq "重送回 204" "204" "$CODE"

echo ""
bold "[3] 非法 play_mode 回 400"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/playback/telemetry" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" -H "Content-Type: application/json" \
    -d "{\"session_id\":\"$(cat /proc/sys/kernel/random/uuid)\",\"video_id\":\"${VIDEO_ID}\",\"play_mode\":\"bogus\"}")
assert_eq "非法 play_mode 回 400" "400" "$CODE"

echo ""
bold "[4] 缺 video 回 404"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_BASE}/api/playback/telemetry" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}" -H "Content-Type: application/json" \
    -d "{\"session_id\":\"$(cat /proc/sys/kernel/random/uuid)\",\"video_id\":\"$(cat /proc/sys/kernel/random/uuid)\",\"play_mode\":\"direct\"}")
assert_eq "缺 video 回 404" "404" "$CODE"

echo ""
bold "[5] GET /api/admin/playback/telemetry — 聚合含 direct，DELTA == 1"
RESP=$(curl -s -w "\n%{http_code}" "${API_BASE}/api/admin/playback/telemetry?days=7" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}")
CODE=$(echo "$RESP" | tail -1)
assert_eq "聚合回 200" "200" "$CODE"
AFTER=$(baseline_sessions)
assert_eq "direct sessions DELTA == 1 (兩次同 session 只算一列)" "1" "$((AFTER - BEFORE))"

echo ""
bold "[6] viewer 讀 admin 端點回 403"
CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_BASE}/api/admin/playback/telemetry?days=7" \
    -H "Authorization: Bearer ${VIEWER_TOKEN}")
assert_eq "viewer 讀聚合回 403" "403" "$CODE"

bold "=== Playback Telemetry 測試通過 ==="
```

- [ ] **Step 2: Make it executable and register the suite**

Run:
```bash
chmod +x scripts/test_telemetry.sh
```
In `scripts/test_all.sh`, change the `ALL_SUITES` line to append `telemetry`:
```bash
ALL_SUITES="auth import videos tags enrich hls analytics telemetry"
```
Also update the header comment's suite list to include `telemetry`.

- [ ] **Step 3: Run the full fast gate**

Run: `task verify`
Expected: PASS — `go vet`, `gofmt`, `go test ./...`, web `tsc`, `eslint`, `vitest` all green.

- [ ] **Step 4: Run the integration suite**

Run: `task test-integration`
Expected: full stack comes up (migrate applies 017), `test_telemetry.sh` prints `=== Playback Telemetry 測試通過 ===`, overall suite green.

- [ ] **Step 5: Commit**

```bash
git add scripts/test_telemetry.sh scripts/test_all.sh
git commit -m "test: add playback telemetry integration suite"
```

---

## Self-Review

**1. Spec coverage:**
- Migration 017 (spec §Section 1) → Task 1. ✓
- Separate table, session_id UNIQUE upsert, raw components stored → Tasks 1, 3. ✓
- model / repository (interface in repo pkg) / service / handler layering (§Section 2) → Tasks 2, 3, 4. ✓
- `classifyNetworkScope` lan/external/unknown incl. loopback/RFC1918/ULA/link-local (§Section 2 + §Testing) → Task 2. ✓
- Aggregate `percentile_cont` P50/P95, rebuffer ratio from raw components, scope filter (§Section 2 query) → Task 3. ✓
- Routes + viewer RBAC + admin coverage (§Section 2) → Task 5. ✓ (admin `/api/*` already covers GET; viewer POST line added.)
- `usePlaybackStats` accumulators reusing existing listeners; TTFF + stall-duration gap filled (§現況核實 2, §Section 3) → Tasks 6, 7. ✓
- PlayerPage mounts hook, HUD behind `?hud=1`/localStorage toggle, send on unmount via beacon with guards + refs (§decision 4, §Section 3) → Task 9. ✓
- viewer JWT auth mirror heartbeat (§decision 5) → Task 5 route under `api` group + Task 4 `user_id` from context. ✓
- `fatal_error_family` retained (§decision a) → Tasks 1, 6, 7, 9. ✓
- play_mode grouping + scope filter, no cross (§decision b) → Task 3. ✓
- proxy IP degradation accepted + TODO (§decision c) → carried as spec TODO; `c.ClientIP()` used as-is in Task 4. ✓
- Tests: service table-driven, handler 200/400/404/500, vitest pure helpers, integration (§Section 4) → Tasks 2, 4, 6, 9, 10. ✓
- `task verify` + `task test-integration` done-condition → Task 10. ✓

**2. Placeholder scan:** No TBD/TODO-as-requirement, no "add validation"/"handle edge cases" hand-waves; every code step shows full code. The one carried TODO (proxy IP) is an explicit deferred decision, not a plan gap. ✓

**3. Type consistency:** `PlaybackTelemetryInput` fields identical across model/repo/service/handler; `SessionSummary` fields identical across `playbackStats.ts` (Task 6), hook return (Task 7), and `PlayerPage` consumption (Task 9); `PlaybackTelemetryPayload` snake_case matches `telemetryRequest` json tags (Task 4 ↔ Task 8); repo scan order matches `PlayModeStats` field order and SQL SELECT column order (Task 3). `getSessionSummary` named consistently in Tasks 7 and 9. ✓

**4. Resolved open question:** ingest success = **204** (mirror heartbeat; idempotent upsert, no body) — fixed in Task 4 handler + Task 10 assertions.
