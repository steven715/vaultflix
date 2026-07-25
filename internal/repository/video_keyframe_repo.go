package repository

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

const queryListKeyframeCandidates = `
	SELECT v.id, v.original_filename, v.source_id, v.file_path, COALESCE(v.video_codec, ''), COALESCE(v.audio_codec, '')
	FROM videos v
	LEFT JOIN video_keyframe_index k ON k.video_id = v.id
	WHERE k.video_id IS NULL
	  AND v.video_codec IS NOT NULL AND v.video_codec <> ''
	  AND v.source_id IS NOT NULL AND v.file_path IS NOT NULL
	ORDER BY v.created_at ASC
	LIMIT $1
`

// ListKeyframeCandidates returns videos with no keyframe boundary table and known codecs
// (remux filtering happens in the service layer).
func (r *videoRepository) ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error) {
	rows, err := r.pool.Query(ctx, queryListKeyframeCandidates, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list keyframe candidates: %w", err)
	}
	defer rows.Close()

	var videos []model.Video
	for rows.Next() {
		var v model.Video
		if err := rows.Scan(&v.ID, &v.OriginalFilename, &v.SourceID, &v.FilePath, &v.VideoCodec, &v.AudioCodec); err != nil {
			return nil, fmt.Errorf("failed to scan keyframe candidate: %w", err)
		}
		videos = append(videos, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate keyframe candidates: %w", err)
	}
	if videos == nil {
		videos = []model.Video{}
	}
	return videos, nil
}
