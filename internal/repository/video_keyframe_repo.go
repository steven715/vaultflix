package repository

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

const queryListKeyframeCandidates = `
	SELECT ` + videoColumns + `
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
	videos, err := collectVideos(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to list keyframe candidates: %w", err)
	}
	return videos, nil
}
