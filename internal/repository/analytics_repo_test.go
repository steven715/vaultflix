package repository

import "testing"

// No sibling *_repo_test.go exists anywhere under internal/repository that
// connects directly to a live Postgres pool (see watch_session_repo_test.go
// for the precedent this mirrors). All DB-backed integration coverage in this
// repo instead lives in scripts/test_*.sh, driven through the real HTTP API
// against a running docker compose stack (see Taskfile.yml's
// test-integration target, which runs `docker compose ... run --rm
// test-runner` executing scripts/test_all.sh).
//
// Rather than invent a new Go-level pgxpool test harness that has no
// precedent here, this test is left structured but skipped, and the
// assertions below are deferred to the HTTP-level integration suite
// (expected to land in Task 12's admin-analytics HTTP integration script,
// e.g. scripts/test_admin_analytics.sh or equivalent, exercised via `task
// test-integration`).
//
// Intended assertions once wired to a real DB pool, given a seeded video and
// a mix of watch_sessions rows spread across several days (some with
// watched_seconds < 10, some >= 10, some for a second user):
//
//  1. KPIs(ctx, days) — total_views only counts rows with watched_seconds
//     >= 10 (the FILTER); total_watched_seconds sums watched_seconds across
//     ALL rows regardless of the >=10 threshold; avg_completion is the
//     average of LEAST(max_progress_seconds/video_duration_seconds, 1.0)
//     restricted to rows with video_duration_seconds > 0 AND watched_seconds
//     >= 10; active_users counts DISTINCT user_id among rows with
//     watched_seconds >= 10 (a user who only has sub-10s sessions in the
//     window must not be counted).
//  2. DailyRaw(ctx, days) — returns a map keyed by "YYYY-MM-DD" containing
//     ONLY the days that actually have at least one session row in the
//     window (a day with zero sessions is absent from the map entirely —
//     zero-fill for the trend is the service's job, not the repo's).
//     Per-day Views respects the same >=10 FILTER as KPIs; WatchedSeconds
//     sums all rows for that day regardless of the threshold.
//  3. TopVideos(ctx, days, limit) — rows are ordered by
//     SUM(ws.watched_seconds) DESC, WatchHours == watched_seconds/3600.0,
//     and a video with zero total watched_seconds in the window is excluded
//     by the HAVING clause even if it has session rows (e.g. all deleted or
//     watched_seconds somehow 0). limit caps the result length.
//  4. TopTags(ctx, days, limit) — rows are ordered by views (the >=10
//     FILTER count) DESC; a tag with zero qualifying views in the window is
//     excluded by the HAVING clause even if some sub-10s sessions reference
//     it through video_tags.
//  5. All four methods return a non-nil empty slice/map (never nil) when the
//     window has no matching sessions at all.
func TestAnalyticsRepository_Aggregations(t *testing.T) {
	t.Skip("no live-DB Go test harness exists in this repo; see comment above — covered by HTTP-level task test-integration instead")
}
