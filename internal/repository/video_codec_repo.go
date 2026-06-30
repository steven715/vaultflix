package repository

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

const queryUpdateCodecs = `
	UPDATE videos
	SET video_codec = $2, audio_codec = $3, updated_at = NOW()
	WHERE id = $1
`

const queryListMissingCodecs = `
	SELECT v.id, v.original_filename, v.source_id, v.file_path
	FROM videos v
	WHERE (v.video_codec IS NULL OR v.video_codec = '')
	  AND v.source_id IS NOT NULL AND v.file_path IS NOT NULL
	LIMIT $1
`

// UpdateCodecs persists the video and audio codec for the given video.
func (r *videoRepository) UpdateCodecs(ctx context.Context, id, videoCodec, audioCodec string) error {
	result, err := r.pool.Exec(ctx, queryUpdateCodecs, id, videoCodec, audioCodec)
	if err != nil {
		return fmt.Errorf("failed to update codecs for video %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

// ListMissingCodecs returns videos whose video_codec is NULL or empty and
// whose source_id and file_path are non-null, up to limit rows.
func (r *videoRepository) ListMissingCodecs(ctx context.Context, limit int) ([]model.Video, error) {
	rows, err := r.pool.Query(ctx, queryListMissingCodecs, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list videos missing codecs: %w", err)
	}
	defer rows.Close()

	var videos []model.Video
	for rows.Next() {
		var v model.Video
		if err := rows.Scan(&v.ID, &v.OriginalFilename, &v.SourceID, &v.FilePath); err != nil {
			return nil, fmt.Errorf("failed to scan video missing codec: %w", err)
		}
		videos = append(videos, v)
	}

	if videos == nil {
		videos = []model.Video{}
	}

	return videos, nil
}
