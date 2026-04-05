package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// WatchHistoryRepository defines the contract for watch history data access.
// Upsert uses INSERT ON CONFLICT(user_id, video_id) DO UPDATE to create or update a record.
// GetByUserAndVideo returns model.ErrNotFound when no record exists.
// ListByUser returns results ordered by watched_at DESC with pagination.
type WatchHistoryRepository interface {
	Upsert(ctx context.Context, record *model.WatchHistory) error
	ListByUser(ctx context.Context, userID string, page, pageSize int) ([]model.WatchHistoryWithVideo, int64, error)
	GetByUserAndVideo(ctx context.Context, userID, videoID string) (*model.WatchHistory, error)
}

const queryUpsertWatchHistory = `
    INSERT INTO watch_history (user_id, video_id, progress_seconds, completed, watched_at, updated_at)
    VALUES ($1, $2, $3, $4, NOW(), NOW())
    ON CONFLICT (user_id, video_id) DO UPDATE
    SET progress_seconds = $3,
        completed = $4,
        watched_at = NOW(),
        updated_at = NOW()
`

const queryCountWatchHistoryByUser = `
    SELECT COUNT(*) FROM watch_history WHERE user_id = $1
`

const queryListWatchHistoryByUser = `
    SELECT wh.id, wh.video_id, v.title, v.thumbnail_key, v.duration_seconds,
           wh.progress_seconds, wh.completed, wh.watched_at
    FROM watch_history wh
    JOIN videos v ON v.id = wh.video_id
    WHERE wh.user_id = $1
    ORDER BY wh.watched_at DESC
    LIMIT $2 OFFSET $3
`

const queryGetWatchHistoryByUserAndVideo = `
    SELECT id, user_id, video_id, progress_seconds, completed, watched_at, updated_at
    FROM watch_history
    WHERE user_id = $1 AND video_id = $2
`

type watchHistoryRepository struct {
	pool *pgxpool.Pool
}

func NewWatchHistoryRepository(pool *pgxpool.Pool) WatchHistoryRepository {
	return &watchHistoryRepository{pool: pool}
}

func (r *watchHistoryRepository) Upsert(ctx context.Context, record *model.WatchHistory) error {
	_, err := r.pool.Exec(ctx, queryUpsertWatchHistory,
		record.UserID, record.VideoID, record.ProgressSeconds, record.Completed,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert watch history for user %s video %s: %w",
			record.UserID, record.VideoID, err)
	}
	return nil
}

func (r *watchHistoryRepository) ListByUser(ctx context.Context, userID string, page, pageSize int) ([]model.WatchHistoryWithVideo, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, queryCountWatchHistoryByUser, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count watch history for user %s: %w", userID, err)
	}

	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx, queryListWatchHistoryByUser, userID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list watch history for user %s: %w", userID, err)
	}
	defer rows.Close()

	var items []model.WatchHistoryWithVideo
	for rows.Next() {
		var item model.WatchHistoryWithVideo
		if err := rows.Scan(
			&item.ID, &item.VideoID, &item.Title, &item.ThumbnailKey,
			&item.DurationSeconds, &item.ProgressSeconds, &item.Completed, &item.WatchedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan watch history row: %w", err)
		}
		items = append(items, item)
	}

	if items == nil {
		items = []model.WatchHistoryWithVideo{}
	}

	return items, total, nil
}

func (r *watchHistoryRepository) GetByUserAndVideo(ctx context.Context, userID, videoID string) (*model.WatchHistory, error) {
	var h model.WatchHistory
	err := r.pool.QueryRow(ctx, queryGetWatchHistoryByUserAndVideo, userID, videoID).Scan(
		&h.ID, &h.UserID, &h.VideoID, &h.ProgressSeconds, &h.Completed, &h.WatchedAt, &h.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get watch history for user %s video %s: %w", userID, videoID, err)
	}
	return &h, nil
}
