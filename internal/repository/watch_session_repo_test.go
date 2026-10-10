package repository

import "testing"

// A real-Postgres harness now exists (openTestPool, ADR-0012; see
// video_repo_test.go). This test predates it and is still a stub: the
// assertions below are the spec for wiring it to openTestPool.
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
	t.Skip("not yet wired to openTestPool; see comment above — covered by HTTP-level task test-integration meanwhile")
}
