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

// TagRepository defines the contract for tag data access.
// GetByID returns model.ErrNotFound when the tag does not exist.
// Create returns model.ErrAlreadyExists when the tag name is duplicated.
// AddVideoTag returns model.ErrConflict when the relation already exists.
// RemoveVideoTag returns model.ErrNotFound when the relation does not exist.
type TagRepository interface {
	List(ctx context.Context, category string) ([]model.TagWithCount, error)
	Create(ctx context.Context, tag *model.Tag) error
	GetByID(ctx context.Context, id int) (*model.Tag, error)
	GetByVideoID(ctx context.Context, videoID string) ([]model.Tag, error)
	GetByVideoIDs(ctx context.Context, videoIDs []string) (map[string][]model.Tag, error)
	AddVideoTag(ctx context.Context, videoID string, tagID int) error
	RemoveVideoTag(ctx context.Context, videoID string, tagID int) error
}

const queryListTags = `
    SELECT t.id, t.name, t.category, COUNT(vt.video_id) AS video_count
    FROM tags t
    LEFT JOIN video_tags vt ON t.id = vt.tag_id
    GROUP BY t.id, t.name, t.category
    ORDER BY t.name
`

const queryListTagsByCategory = `
    SELECT t.id, t.name, t.category, COUNT(vt.video_id) AS video_count
    FROM tags t
    LEFT JOIN video_tags vt ON t.id = vt.tag_id
    WHERE t.category = $1
    GROUP BY t.id, t.name, t.category
    ORDER BY t.name
`

const queryCreateTag = `
    INSERT INTO tags (name, category)
    VALUES ($1, $2)
    RETURNING id
`

const queryGetTagByID = `
    SELECT id, name, category FROM tags WHERE id = $1
`

const queryAddVideoTag = `
    INSERT INTO video_tags (video_id, tag_id)
    VALUES ($1, $2)
`

const queryRemoveVideoTag = `
    DELETE FROM video_tags
    WHERE video_id = $1 AND tag_id = $2
`

const queryGetTagsByVideoID = `
    SELECT t.id, t.name, t.category
    FROM tags t
    INNER JOIN video_tags vt ON t.id = vt.tag_id
    WHERE vt.video_id = $1
    ORDER BY t.name
`

const queryGetTagsByVideoIDs = `
    SELECT vt.video_id, t.id, t.name, t.category
    FROM tags t
    INNER JOIN video_tags vt ON t.id = vt.tag_id
    WHERE vt.video_id = ANY($1)
    ORDER BY t.name
`

type tagRepository struct {
	pool *pgxpool.Pool
}

func NewTagRepository(pool *pgxpool.Pool) TagRepository {
	return &tagRepository{pool: pool}
}

func (r *tagRepository) List(ctx context.Context, category string) ([]model.TagWithCount, error) {
	var rows pgx.Rows
	var err error

	if category == "" {
		rows, err = r.pool.Query(ctx, queryListTags)
	} else {
		rows, err = r.pool.Query(ctx, queryListTagsByCategory, category)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	defer rows.Close()

	var tags []model.TagWithCount
	for rows.Next() {
		var t model.TagWithCount
		if err := rows.Scan(&t.ID, &t.Name, &t.Category, &t.VideoCount); err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}
		tags = append(tags, t)
	}

	if tags == nil {
		tags = []model.TagWithCount{}
	}

	return tags, nil
}

func (r *tagRepository) GetByID(ctx context.Context, id int) (*model.Tag, error) {
	var tag model.Tag
	err := r.pool.QueryRow(ctx, queryGetTagByID, id).Scan(&tag.ID, &tag.Name, &tag.Category)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get tag %d: %w", id, err)
	}
	return &tag, nil
}

func (r *tagRepository) Create(ctx context.Context, tag *model.Tag) error {
	err := r.pool.QueryRow(ctx, queryCreateTag, tag.Name, tag.Category).Scan(&tag.ID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create tag %s: %w", tag.Name, err)
	}
	return nil
}

func (r *tagRepository) AddVideoTag(ctx context.Context, videoID string, tagID int) error {
	_, err := r.pool.Exec(ctx, queryAddVideoTag, videoID, tagID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				return model.ErrConflict
			}
			if pgErr.Code == "23503" {
				return model.ErrNotFound
			}
		}
		return fmt.Errorf("failed to add tag %d to video %s: %w", tagID, videoID, err)
	}
	return nil
}

func (r *tagRepository) RemoveVideoTag(ctx context.Context, videoID string, tagID int) error {
	result, err := r.pool.Exec(ctx, queryRemoveVideoTag, videoID, tagID)
	if err != nil {
		return fmt.Errorf("failed to remove tag %d from video %s: %w", tagID, videoID, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *tagRepository) GetByVideoID(ctx context.Context, videoID string) ([]model.Tag, error) {
	rows, err := r.pool.Query(ctx, queryGetTagsByVideoID, videoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tags for video %s: %w", videoID, err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Category); err != nil {
			return nil, fmt.Errorf("failed to scan tag for video %s: %w", videoID, err)
		}
		tags = append(tags, t)
	}

	if tags == nil {
		tags = []model.Tag{}
	}

	return tags, nil
}

func (r *tagRepository) GetByVideoIDs(ctx context.Context, videoIDs []string) (map[string][]model.Tag, error) {
	rows, err := r.pool.Query(ctx, queryGetTagsByVideoIDs, videoIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to batch get tags for videos: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]model.Tag)
	for rows.Next() {
		var videoID string
		var t model.Tag
		if err := rows.Scan(&videoID, &t.ID, &t.Name, &t.Category); err != nil {
			return nil, fmt.Errorf("failed to scan batch tag: %w", err)
		}
		result[videoID] = append(result[videoID], t)
	}

	return result, nil
}
