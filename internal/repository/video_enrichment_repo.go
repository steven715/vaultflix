package repository

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

const queryUpdateVideoMetadata = `
    UPDATE videos
    SET code = $2, title = $3, release_date = $4, runtime_minutes = $5,
        maker = $6, label = $7, series = $8, cover_key = $9,
        enrichment_status = 'enriched', enriched_at = NOW(), updated_at = NOW()
    WHERE id = $1
`

const querySetEnrichmentStatus = `
    UPDATE videos SET enrichment_status = $2, updated_at = NOW() WHERE id = $1
`

const queryVideosByEnrichmentStatus = `
    SELECT id, title, enrichment_status, code, original_filename, source_id, file_path
    FROM videos WHERE enrichment_status = $1 ORDER BY created_at
`

const querySeedVideoCode = `
    UPDATE videos SET code = $2, enrichment_status = $3, updated_at = NOW() WHERE id = $1
`

func (r *videoRepository) UpdateMetadata(ctx context.Context, id string, m model.VideoMetadataUpdate) error {
	result, err := r.pool.Exec(ctx, queryUpdateVideoMetadata,
		id, m.Code, m.Title, m.ReleaseDate, m.RuntimeMinutes,
		m.Maker, m.Label, m.Series, m.CoverKey,
	)
	if err != nil {
		return fmt.Errorf("failed to update metadata for video %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *videoRepository) SetEnrichmentStatus(ctx context.Context, id, status string) error {
	result, err := r.pool.Exec(ctx, querySetEnrichmentStatus, id, status)
	if err != nil {
		return fmt.Errorf("failed to set enrichment status for video %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *videoRepository) ListByEnrichmentStatus(ctx context.Context, status string) ([]model.Video, error) {
	rows, err := r.pool.Query(ctx, queryVideosByEnrichmentStatus, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list videos by enrichment status %s: %w", status, err)
	}
	defer rows.Close()

	var videos []model.Video
	for rows.Next() {
		var v model.Video
		if err := rows.Scan(
			&v.ID, &v.Title, &v.EnrichmentStatus, &v.Code, &v.OriginalFilename, &v.SourceID, &v.FilePath,
		); err != nil {
			return nil, fmt.Errorf("failed to scan video by enrichment status: %w", err)
		}
		videos = append(videos, v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate videos by enrichment status: %w", err)
	}

	if videos == nil {
		videos = []model.Video{}
	}

	return videos, nil
}

func (r *videoRepository) SeedCode(ctx context.Context, id, code, status string) error {
	result, err := r.pool.Exec(ctx, querySeedVideoCode, id, code, status)
	if err != nil {
		return fmt.Errorf("failed to seed code for video %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}
