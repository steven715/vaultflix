package repository

import "testing"

// No sibling *_repo_test.go exists anywhere under internal/repository (and no
// build-tag-gated Go test connects directly to a live Postgres pool anywhere
// in this codebase — the sole precedent, internal/service/preview_clip_smoke_test.go,
// is gated behind `-tags=smoke` but only shells out to ffmpeg, not a DB).
// All DB-backed integration coverage in this repo instead lives in
// scripts/test_*.sh, driven through the real HTTP API against a running
// docker compose stack (see Taskfile.yml's test-integration target, which
// runs `docker compose ... run --rm test-runner` executing scripts/test_all.sh).
//
// Rather than invent a new Go-level pgxpool test harness that has no
// precedent here, this test is left structured but skipped, and the
// assertions below are deferred to the HTTP-level integration suite
// (expected to land as scripts/test_watch_sessions.sh or equivalent in a
// later task per the SDD plan, exercised via `task test-integration`).
//
// Intended assertions once wired to a real DB pool:
//  1. First Upsert(ctx, HeartbeatInput{SessionID: sid, VideoID: realVideoID, PlayedDelta: 30, PositionSeconds: 30})
//     against a seeded real video inserts a row with watched_seconds == 30
//     and video_duration_seconds snapshotted from videos.duration_seconds.
//  2. A second Upsert with the same SessionID and PlayedDelta: 20,
//     PositionSeconds: 10 (a rewind) sums watched_seconds to 50 and takes
//     GREATEST(max_progress_seconds) == 30 (does not regress on rewind).
//  3. Upsert with a random/missing VideoID returns model.ErrNotFound, and
//     leaves no row behind (RowsAffected == 0 because the SELECT ... FROM
//     videos WHERE v.id = $3 yields nothing to insert).
func TestWatchSessionRepository_Upsert(t *testing.T) {
	t.Skip("no live-DB Go test harness exists in this repo; see comment above — covered by HTTP-level task test-integration instead")
}
