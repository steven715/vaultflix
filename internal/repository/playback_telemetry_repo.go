package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

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
// scan targets *float64. An empty $2 disables the scope filter.
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
