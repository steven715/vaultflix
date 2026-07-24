package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

const queryGetKeyframeIndex = `
	SELECT segments, probed_at
	FROM video_keyframe_index
	WHERE video_id = $1
`

const queryUpsertKeyframeIndex = `
	INSERT INTO video_keyframe_index (video_id, segments, probed_at)
	VALUES ($1, $2, $3)
	ON CONFLICT (video_id) DO UPDATE SET segments = EXCLUDED.segments, probed_at = EXCLUDED.probed_at
`

// keyframeIndexRepository persists keyframe segment boundary tables.
type keyframeIndexRepository struct {
	pool *pgxpool.Pool
}

// NewKeyframeIndexRepository creates a keyframe index repository.
func NewKeyframeIndexRepository(pool *pgxpool.Pool) *keyframeIndexRepository {
	return &keyframeIndexRepository{pool: pool}
}

// Get returns a video's boundary table; returns model.ErrNotFound if none exists.
func (r *keyframeIndexRepository) Get(ctx context.Context, videoID string) (*model.KeyframeIndex, error) {
	idx := &model.KeyframeIndex{VideoID: videoID}
	var raw []byte
	err := r.pool.QueryRow(ctx, queryGetKeyframeIndex, videoID).Scan(&raw, &idx.ProbedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get keyframe index for video %s: %w", videoID, err)
	}
	if err := json.Unmarshal(raw, &idx.Segments); err != nil {
		return nil, fmt.Errorf("failed to decode keyframe segments for video %s: %w", videoID, err)
	}
	return idx, nil
}

// Upsert inserts or replaces a boundary table.
func (r *keyframeIndexRepository) Upsert(ctx context.Context, idx *model.KeyframeIndex) error {
	raw, err := json.Marshal(idx.Segments)
	if err != nil {
		return fmt.Errorf("failed to encode keyframe segments for video %s: %w", idx.VideoID, err)
	}
	if _, err := r.pool.Exec(ctx, queryUpsertKeyframeIndex, idx.VideoID, raw, idx.ProbedAt); err != nil {
		return fmt.Errorf("failed to upsert keyframe index for video %s: %w", idx.VideoID, err)
	}
	return nil
}
