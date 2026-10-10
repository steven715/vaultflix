package repository

import (
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/steven/vaultflix/internal/model"
)

// videoColumns is the one column list every read of a Video selects (videos
// aliased as v), so a model.Video is always complete regardless of which
// method produced it. Adding a column means editing this list and scanVideo
// together; TestVideoRepository_GetByID_ReadsEveryColumn fails otherwise.
// Every expression is aliased so the list also works inside a subquery.
const videoColumns = `v.id, v.title, v.description, v.minio_object_key, v.thumbnail_key, v.preview_key,
    v.duration_seconds, v.resolution, v.file_size_bytes, v.mime_type,
    COALESCE(v.video_codec, '') AS video_codec, COALESCE(v.audio_codec, '') AS audio_codec,
    v.original_filename, v.created_at, v.updated_at, v.source_id, v.file_path,
    COALESCE(v.code, '') AS code, v.release_date, COALESCE(v.runtime_minutes, 0) AS runtime_minutes,
    COALESCE(v.maker, '') AS maker, COALESCE(v.label, '') AS label, COALESCE(v.series, '') AS series,
    COALESCE(v.cover_key, '') AS cover_key, v.enrichment_status, v.enriched_at`

// scanVideo scans one row selected with videoColumns. pgx.Rows satisfies
// pgx.Row, so it serves both QueryRow and Query loops.
func scanVideo(row pgx.Row) (model.Video, error) {
	var v model.Video
	err := row.Scan(
		&v.ID, &v.Title, &v.Description, &v.MinIOObjectKey, &v.ThumbnailKey, &v.PreviewKey,
		&v.DurationSeconds, &v.Resolution, &v.FileSizeBytes, &v.MimeType,
		&v.VideoCodec, &v.AudioCodec,
		&v.OriginalFilename, &v.CreatedAt, &v.UpdatedAt, &v.SourceID, &v.FilePath,
		&v.Code, &v.ReleaseDate, &v.RuntimeMinutes,
		&v.Maker, &v.Label, &v.Series,
		&v.CoverKey, &v.EnrichmentStatus, &v.EnrichedAt,
	)
	if err != nil {
		return model.Video{}, fmt.Errorf("failed to scan video row: %w", err)
	}
	return v, nil
}

// collectVideos drains rows selected with videoColumns. It closes rows and
// never returns a nil slice.
func collectVideos(rows pgx.Rows) ([]model.Video, error) {
	defer rows.Close()
	videos := []model.Video{}
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		videos = append(videos, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate videos: %w", err)
	}
	return videos, nil
}
