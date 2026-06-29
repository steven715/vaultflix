package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// SuggestionRepository defines the contract for metadata suggestion data access.
// Create inserts a new suggestion and backfills ID and FetchedAt.
// GetByVideoID returns all suggestions for a video, ordered by fetched_at DESC.
// GetByID returns model.ErrNotFound when the suggestion does not exist.
// Delete removes a suggestion by ID.
type SuggestionRepository interface {
	Create(ctx context.Context, s *model.MetadataSuggestion) error
	GetByVideoID(ctx context.Context, videoID string) ([]model.MetadataSuggestion, error)
	GetByID(ctx context.Context, id string) (*model.MetadataSuggestion, error)
	Delete(ctx context.Context, id string) error
}

const queryCreateSuggestion = `
    INSERT INTO metadata_suggestions (video_id, source, code, payload, status)
    VALUES ($1, $2, $3, $4, $5)
    RETURNING id, fetched_at
`

const querySuggestionsByVideo = `
    SELECT id, video_id, source, code, payload, fetched_at, status
    FROM metadata_suggestions
    WHERE video_id = $1
    ORDER BY fetched_at DESC
`

const querySuggestionByID = `
    SELECT id, video_id, source, code, payload, fetched_at, status
    FROM metadata_suggestions
    WHERE id = $1
`

const queryDeleteSuggestion = `DELETE FROM metadata_suggestions WHERE id = $1`

type suggestionRepo struct {
	pool *pgxpool.Pool
}

// NewSuggestionRepository returns a new SuggestionRepository backed by the given pool.
func NewSuggestionRepository(pool *pgxpool.Pool) SuggestionRepository {
	return &suggestionRepo{pool: pool}
}

func (r *suggestionRepo) Create(ctx context.Context, s *model.MetadataSuggestion) error {
	payloadJSON, err := json.Marshal(s.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal suggestion payload: %w", err)
	}

	err = r.pool.QueryRow(ctx, queryCreateSuggestion,
		s.VideoID, s.Source, s.Code, payloadJSON, s.Status,
	).Scan(&s.ID, &s.FetchedAt)
	if err != nil {
		return fmt.Errorf("failed to create suggestion for video %s: %w", s.VideoID, err)
	}
	return nil
}

func (r *suggestionRepo) GetByVideoID(ctx context.Context, videoID string) ([]model.MetadataSuggestion, error) {
	rows, err := r.pool.Query(ctx, querySuggestionsByVideo, videoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get suggestions for video %s: %w", videoID, err)
	}
	defer rows.Close()

	var suggestions []model.MetadataSuggestion
	for rows.Next() {
		s, err := scanSuggestion(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan suggestion for video %s: %w", videoID, err)
		}
		suggestions = append(suggestions, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate suggestions for video %s: %w", videoID, err)
	}

	if suggestions == nil {
		suggestions = []model.MetadataSuggestion{}
	}

	return suggestions, nil
}

func (r *suggestionRepo) GetByID(ctx context.Context, id string) (*model.MetadataSuggestion, error) {
	rows, err := r.pool.Query(ctx, querySuggestionByID, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get suggestion %s: %w", id, err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to get suggestion %s: %w", id, err)
		}
		return nil, model.ErrNotFound
	}

	s, err := scanSuggestion(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to scan suggestion %s: %w", id, err)
	}
	return &s, nil
}

func (r *suggestionRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, queryDeleteSuggestion, id)
	if err != nil {
		return fmt.Errorf("failed to delete suggestion %s: %w", id, err)
	}
	return nil
}

// scanSuggestion reads one row into a MetadataSuggestion, unmarshalling the JSONB payload.
func scanSuggestion(rows pgx.Rows) (model.MetadataSuggestion, error) {
	var s model.MetadataSuggestion
	var payloadJSON []byte
	if err := rows.Scan(&s.ID, &s.VideoID, &s.Source, &s.Code, &payloadJSON, &s.FetchedAt, &s.Status); err != nil {
		return s, fmt.Errorf("scan suggestion row: %w", err)
	}
	if err := json.Unmarshal(payloadJSON, &s.Payload); err != nil {
		return s, fmt.Errorf("unmarshal payload: %w", err)
	}
	return s, nil
}

// Ensure suggestionRepo satisfies SuggestionRepository at compile time.
var _ SuggestionRepository = (*suggestionRepo)(nil)
