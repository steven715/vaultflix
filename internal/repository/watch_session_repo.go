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
