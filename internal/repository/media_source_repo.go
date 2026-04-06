package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// MediaSourceRepository manages media source persistence.
//
// List returns all media sources (no pagination, expected count is small).
// FindByID returns model.ErrNotFound when the source does not exist.
// Create returns model.ErrAlreadyExists when mount_path is duplicated.
// Update returns model.ErrNotFound when the source does not exist.
// Delete returns model.ErrNotFound when the source does not exist.
type MediaSourceRepository interface {
	List(ctx context.Context) ([]model.MediaSource, error)
	FindByID(ctx context.Context, id string) (*model.MediaSource, error)
	Create(ctx context.Context, source *model.MediaSource) error
	Update(ctx context.Context, source *model.MediaSource) error
	Delete(ctx context.Context, id string) error
}

const queryListMediaSources = `
    SELECT ms.id, ms.label, ms.mount_path, ms.enabled, ms.created_at, ms.updated_at,
           COUNT(v.id) AS video_count
    FROM media_sources ms
    LEFT JOIN videos v ON v.source_id = ms.id
    GROUP BY ms.id
    ORDER BY ms.created_at ASC
`

const queryFindMediaSourceByID = `
    SELECT id, label, mount_path, enabled, created_at, updated_at
    FROM media_sources
    WHERE id = $1
`

const queryCreateMediaSource = `
    INSERT INTO media_sources (label, mount_path)
    VALUES ($1, $2)
    RETURNING id, enabled, created_at, updated_at
`

const queryUpdateMediaSource = `
    UPDATE media_sources
    SET label = $1, enabled = $2, updated_at = NOW()
    WHERE id = $3
`

const queryDeleteMediaSource = `
    DELETE FROM media_sources WHERE id = $1
`

type mediaSourceRepository struct {
	pool *pgxpool.Pool
}

func NewMediaSourceRepository(pool *pgxpool.Pool) MediaSourceRepository {
	return &mediaSourceRepository{pool: pool}
}

func (r *mediaSourceRepository) List(ctx context.Context) ([]model.MediaSource, error) {
	rows, err := r.pool.Query(ctx, queryListMediaSources)
	if err != nil {
		return nil, fmt.Errorf("failed to list media sources: %w", err)
	}
	defer rows.Close()

	var sources []model.MediaSource
	for rows.Next() {
		var s model.MediaSource
		if err := rows.Scan(&s.ID, &s.Label, &s.MountPath, &s.Enabled, &s.CreatedAt, &s.UpdatedAt, &s.VideoCount); err != nil {
			return nil, fmt.Errorf("failed to scan media source: %w", err)
		}
		sources = append(sources, s)
	}

	if sources == nil {
		sources = []model.MediaSource{}
	}

	return sources, nil
}

func (r *mediaSourceRepository) FindByID(ctx context.Context, id string) (*model.MediaSource, error) {
	var s model.MediaSource
	err := r.pool.QueryRow(ctx, queryFindMediaSourceByID, id).Scan(
		&s.ID, &s.Label, &s.MountPath, &s.Enabled, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("failed to find media source %s: %w", id, err)
	}
	return &s, nil
}

func (r *mediaSourceRepository) Create(ctx context.Context, source *model.MediaSource) error {
	err := r.pool.QueryRow(ctx, queryCreateMediaSource, source.Label, source.MountPath).Scan(
		&source.ID, &source.Enabled, &source.CreatedAt, &source.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create media source: %w", err)
	}
	return nil
}

func (r *mediaSourceRepository) Update(ctx context.Context, source *model.MediaSource) error {
	result, err := r.pool.Exec(ctx, queryUpdateMediaSource, source.Label, source.Enabled, source.ID)
	if err != nil {
		return fmt.Errorf("failed to update media source %s: %w", source.ID, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *mediaSourceRepository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDeleteMediaSource, id)
	if err != nil {
		return fmt.Errorf("failed to delete media source %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}
