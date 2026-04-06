package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// RecommendationRepository defines the contract for daily recommendation data access.
// ListByDate returns recommendations for the given date, sorted by sort_order ASC.
// Create returns model.ErrConflict when the same video+date combination already exists.
// Delete returns model.ErrNotFound when the recommendation does not exist.
// GetRandomUnwatched returns up to limit videos the user has not completed watching.
// UpdateSortOrder returns model.ErrNotFound when the recommendation does not exist.
type RecommendationRepository interface {
	ListByDate(ctx context.Context, date time.Time) ([]model.RecommendationWithVideo, error)
	Create(ctx context.Context, rec *model.DailyRecommendation) error
	UpdateSortOrder(ctx context.Context, id string, sortOrder int) error
	Delete(ctx context.Context, id string) error
	GetRandomUnwatched(ctx context.Context, userID string, limit int) ([]model.Video, error)
}

const queryListRecommendationsByDate = `
    SELECT dr.id, dr.video_id, dr.recommend_date, dr.sort_order,
           v.title, v.thumbnail_key, v.duration_seconds, v.resolution, v.file_size_bytes
    FROM daily_recommendations dr
    JOIN videos v ON v.id = dr.video_id
    WHERE dr.recommend_date = $1
    ORDER BY dr.sort_order ASC
`

const queryCreateRecommendation = `
    INSERT INTO daily_recommendations (video_id, recommend_date, sort_order)
    VALUES ($1, $2, $3)
    RETURNING id, created_at
`

const queryUpdateRecommendationSortOrder = `
    UPDATE daily_recommendations SET sort_order = $2 WHERE id = $1
`

const queryDeleteRecommendation = `
    DELETE FROM daily_recommendations WHERE id = $1
`

const queryGetRandomUnwatched = `
    SELECT v.id, v.title, v.description, v.minio_object_key, v.thumbnail_key,
           v.duration_seconds, v.resolution, v.file_size_bytes, v.mime_type,
           v.original_filename, v.created_at, v.updated_at
    FROM videos v
    LEFT JOIN watch_history wh ON wh.video_id = v.id AND wh.user_id = $1
    WHERE wh.id IS NULL OR wh.completed = FALSE
    ORDER BY RANDOM()
    LIMIT $2
`

type recommendationRepository struct {
	pool *pgxpool.Pool
}

func NewRecommendationRepository(pool *pgxpool.Pool) RecommendationRepository {
	return &recommendationRepository{pool: pool}
}

func (r *recommendationRepository) ListByDate(ctx context.Context, date time.Time) ([]model.RecommendationWithVideo, error) {
	rows, err := r.pool.Query(ctx, queryListRecommendationsByDate, date)
	if err != nil {
		return nil, fmt.Errorf("failed to list recommendations for %s: %w", date.Format("2006-01-02"), err)
	}
	defer rows.Close()

	var recs []model.RecommendationWithVideo
	for rows.Next() {
		var rec model.RecommendationWithVideo
		if err := rows.Scan(
			&rec.ID, &rec.VideoID, &rec.RecommendDate, &rec.SortOrder,
			&rec.Title, &rec.ThumbnailKey, &rec.DurationSeconds, &rec.Resolution, &rec.FileSizeBytes,
		); err != nil {
			return nil, fmt.Errorf("failed to scan recommendation: %w", err)
		}
		recs = append(recs, rec)
	}

	if recs == nil {
		recs = []model.RecommendationWithVideo{}
	}

	return recs, nil
}

func (r *recommendationRepository) Create(ctx context.Context, rec *model.DailyRecommendation) error {
	err := r.pool.QueryRow(ctx, queryCreateRecommendation,
		rec.VideoID, rec.RecommendDate, rec.SortOrder,
	).Scan(&rec.ID, &rec.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.ErrConflict
		}
		return fmt.Errorf("failed to create recommendation: %w", err)
	}
	return nil
}

func (r *recommendationRepository) UpdateSortOrder(ctx context.Context, id string, sortOrder int) error {
	result, err := r.pool.Exec(ctx, queryUpdateRecommendationSortOrder, id, sortOrder)
	if err != nil {
		return fmt.Errorf("failed to update sort order for recommendation %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *recommendationRepository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDeleteRecommendation, id)
	if err != nil {
		return fmt.Errorf("failed to delete recommendation %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *recommendationRepository) GetRandomUnwatched(ctx context.Context, userID string, limit int) ([]model.Video, error) {
	rows, err := r.pool.Query(ctx, queryGetRandomUnwatched, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get random unwatched videos: %w", err)
	}
	defer rows.Close()

	var videos []model.Video
	for rows.Next() {
		var v model.Video
		if err := rows.Scan(
			&v.ID, &v.Title, &v.Description, &v.MinIOObjectKey, &v.ThumbnailKey,
			&v.DurationSeconds, &v.Resolution, &v.FileSizeBytes, &v.MimeType,
			&v.OriginalFilename, &v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan random unwatched video: %w", err)
		}
		videos = append(videos, v)
	}

	if videos == nil {
		videos = []model.Video{}
	}

	return videos, nil
}
