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
