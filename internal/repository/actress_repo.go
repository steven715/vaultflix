package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// ActressRepository defines the contract for actress data access.
// Upsert inserts or updates an actress by name_ja and backfills ID and CreatedAt.
// AddVideoActress links an actress to a video; silently ignores duplicate links.
// GetByVideoID returns all actresses for a video, ordered by name_ja.
type ActressRepository interface {
	Upsert(ctx context.Context, a *model.Actress) error
	AddVideoActress(ctx context.Context, videoID, actressID string) error
	GetByVideoID(ctx context.Context, videoID string) ([]model.Actress, error)
}

const queryUpsertActress = `
    INSERT INTO actresses (name_ja, name_romaji, avatar_key)
    VALUES ($1, $2, $3)
    ON CONFLICT (name_ja) DO UPDATE
        SET name_romaji = EXCLUDED.name_romaji,
            avatar_key  = EXCLUDED.avatar_key
    RETURNING id, created_at
`

const queryAddVideoActress = `
    INSERT INTO video_actresses (video_id, actress_id)
    VALUES ($1, $2)
    ON CONFLICT DO NOTHING
`

const queryActressByVideo = `
    SELECT a.id, a.name_ja, a.name_romaji, a.avatar_key, a.created_at
    FROM actresses a
    JOIN video_actresses va ON va.actress_id = a.id
    WHERE va.video_id = $1
    ORDER BY a.name_ja
`

type actressRepo struct {
	pool *pgxpool.Pool
}

// NewActressRepository returns a new ActressRepository backed by the given pool.
func NewActressRepository(pool *pgxpool.Pool) ActressRepository {
	return &actressRepo{pool: pool}
}

func (r *actressRepo) Upsert(ctx context.Context, a *model.Actress) error {
	err := r.pool.QueryRow(ctx, queryUpsertActress, a.NameJa, a.NameRomaji, a.AvatarKey).
		Scan(&a.ID, &a.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to upsert actress %s: %w", a.NameJa, err)
	}
	return nil
}

func (r *actressRepo) AddVideoActress(ctx context.Context, videoID, actressID string) error {
	_, err := r.pool.Exec(ctx, queryAddVideoActress, videoID, actressID)
	if err != nil {
		return fmt.Errorf("failed to add actress %s to video %s: %w", actressID, videoID, err)
	}
	return nil
}

func (r *actressRepo) GetByVideoID(ctx context.Context, videoID string) ([]model.Actress, error) {
	rows, err := r.pool.Query(ctx, queryActressByVideo, videoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get actresses for video %s: %w", videoID, err)
	}
	defer rows.Close()

	var actresses []model.Actress
	for rows.Next() {
		var a model.Actress
		if err := rows.Scan(&a.ID, &a.NameJa, &a.NameRomaji, &a.AvatarKey, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan actress for video %s: %w", videoID, err)
		}
		actresses = append(actresses, a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate actresses for video %s: %w", videoID, err)
	}

	if actresses == nil {
		actresses = []model.Actress{}
	}

	return actresses, nil
}
