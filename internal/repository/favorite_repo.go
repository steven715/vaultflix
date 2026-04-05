package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// FavoriteRepository defines the contract for favorite data access.
// Add returns model.ErrAlreadyExists when the favorite already exists.
// Remove returns model.ErrNotFound when the favorite does not exist.
// Exists returns (false, nil) when the favorite does not exist — not an error.
// ListByUser returns results ordered by created_at DESC with pagination.
type FavoriteRepository interface {
	Add(ctx context.Context, userID, videoID string) error
	Remove(ctx context.Context, userID, videoID string) error
	Exists(ctx context.Context, userID, videoID string) (bool, error)
	ListByUser(ctx context.Context, userID string, page, pageSize int) ([]model.VideoSummary, int64, error)
}

const queryAddFavorite = `
    INSERT INTO favorites (user_id, video_id)
    VALUES ($1, $2)
    ON CONFLICT (user_id, video_id) DO NOTHING
`

const queryRemoveFavorite = `
    DELETE FROM favorites WHERE user_id = $1 AND video_id = $2
`

const queryExistsFavorite = `
    SELECT EXISTS(SELECT 1 FROM favorites WHERE user_id = $1 AND video_id = $2)
`

const queryCountFavoritesByUser = `
    SELECT COUNT(*) FROM favorites WHERE user_id = $1
`

const queryListFavoritesByUser = `
    SELECT v.id, v.title, v.thumbnail_key, v.duration_seconds,
           v.resolution, v.file_size_bytes, v.created_at
    FROM favorites f
    JOIN videos v ON v.id = f.video_id
    WHERE f.user_id = $1
    ORDER BY f.created_at DESC
    LIMIT $2 OFFSET $3
`

type favoriteRepository struct {
	pool *pgxpool.Pool
}

func NewFavoriteRepository(pool *pgxpool.Pool) FavoriteRepository {
	return &favoriteRepository{pool: pool}
}

func (r *favoriteRepository) Add(ctx context.Context, userID, videoID string) error {
	result, err := r.pool.Exec(ctx, queryAddFavorite, userID, videoID)
	if err != nil {
		return fmt.Errorf("failed to add favorite for user %s video %s: %w", userID, videoID, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrAlreadyExists
	}
	return nil
}

func (r *favoriteRepository) Remove(ctx context.Context, userID, videoID string) error {
	result, err := r.pool.Exec(ctx, queryRemoveFavorite, userID, videoID)
	if err != nil {
		return fmt.Errorf("failed to remove favorite for user %s video %s: %w", userID, videoID, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *favoriteRepository) Exists(ctx context.Context, userID, videoID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsFavorite, userID, videoID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check favorite for user %s video %s: %w", userID, videoID, err)
	}
	return exists, nil
}

func (r *favoriteRepository) ListByUser(ctx context.Context, userID string, page, pageSize int) ([]model.VideoSummary, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, queryCountFavoritesByUser, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count favorites for user %s: %w", userID, err)
	}

	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx, queryListFavoritesByUser, userID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list favorites for user %s: %w", userID, err)
	}
	defer rows.Close()

	var items []model.VideoSummary
	for rows.Next() {
		var item model.VideoSummary
		if err := rows.Scan(
			&item.ID, &item.Title, &item.ThumbnailKey, &item.DurationSeconds,
			&item.Resolution, &item.FileSizeBytes, &item.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan favorite row: %w", err)
		}
		items = append(items, item)
	}

	if items == nil {
		items = []model.VideoSummary{}
	}

	return items, total, nil
}
