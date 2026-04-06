package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// VideoRepository defines the contract for video data access.
// GetByID returns model.ErrNotFound when the video does not exist.
// Update returns model.ErrNotFound when the video does not exist.
// Delete returns model.ErrNotFound when the video does not exist.
type VideoRepository interface {
	ExistsByFilenameAndSize(ctx context.Context, filename string, sizeBytes int64) (bool, error)
	Create(ctx context.Context, video *model.Video) error
	List(ctx context.Context, filter model.VideoFilter) ([]model.Video, int64, error)
	GetByID(ctx context.Context, id string) (*model.Video, error)
	Update(ctx context.Context, id string, input model.UpdateVideoInput) error
	Delete(ctx context.Context, id string) error
	// FindBySourceAndPath looks up a video by source_id + file_path.
	// Returns model.ErrNotFound when no matching video exists.
	FindBySourceAndPath(ctx context.Context, sourceID string, filePath string) (*model.Video, error)
}

var allowedSortColumns = map[string]string{
	"created_at":       "v.created_at",
	"title":            "v.title",
	"duration_seconds": "v.duration_seconds",
	"file_size_bytes":  "v.file_size_bytes",
}

const queryExistsVideoByFilenameAndSize = `
    SELECT EXISTS(
        SELECT 1 FROM videos
        WHERE original_filename = $1 AND file_size_bytes = $2
    )
`

const queryCreateVideo = `
    INSERT INTO videos (id, title, description, minio_object_key, thumbnail_key,
                        duration_seconds, resolution, file_size_bytes, mime_type,
                        original_filename, source_id, file_path)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
    RETURNING created_at, updated_at
`

const queryGetVideoByID = `
    SELECT id, title, description, minio_object_key, thumbnail_key,
           duration_seconds, resolution, file_size_bytes, mime_type,
           original_filename, created_at, updated_at, source_id, file_path
    FROM videos
    WHERE id = $1
`

const queryFindBySourceAndPath = `
    SELECT id, title, description, minio_object_key, thumbnail_key,
           duration_seconds, resolution, file_size_bytes, mime_type,
           original_filename, created_at, updated_at, source_id, file_path
    FROM videos
    WHERE source_id = $1 AND file_path = $2
`

const queryUpdateVideo = `
    UPDATE videos
    SET title = $2, description = $3, updated_at = NOW()
    WHERE id = $1
`

const queryDeleteVideo = `
    DELETE FROM videos WHERE id = $1
`

type videoRepository struct {
	pool *pgxpool.Pool
}

func NewVideoRepository(pool *pgxpool.Pool) VideoRepository {
	return &videoRepository{pool: pool}
}

func (r *videoRepository) ExistsByFilenameAndSize(ctx context.Context, filename string, sizeBytes int64) (bool, error) {
	var exists bool

	err := r.pool.QueryRow(ctx, queryExistsVideoByFilenameAndSize, filename, sizeBytes).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check video existence for %s: %w", filename, err)
	}

	return exists, nil
}

func (r *videoRepository) Create(ctx context.Context, video *model.Video) error {
	err := r.pool.QueryRow(ctx, queryCreateVideo,
		video.ID, video.Title, video.Description, video.MinIOObjectKey, video.ThumbnailKey,
		video.DurationSeconds, video.Resolution, video.FileSizeBytes, video.MimeType,
		video.OriginalFilename, video.SourceID, video.FilePath,
	).Scan(&video.CreatedAt, &video.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create video %s: %w", video.OriginalFilename, err)
	}

	return nil
}

func (r *videoRepository) GetByID(ctx context.Context, id string) (*model.Video, error) {
	var video model.Video
	err := r.pool.QueryRow(ctx, queryGetVideoByID, id).Scan(
		&video.ID, &video.Title, &video.Description, &video.MinIOObjectKey, &video.ThumbnailKey,
		&video.DurationSeconds, &video.Resolution, &video.FileSizeBytes, &video.MimeType,
		&video.OriginalFilename, &video.CreatedAt, &video.UpdatedAt, &video.SourceID, &video.FilePath,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get video %s: %w", id, err)
	}
	return &video, nil
}

func (r *videoRepository) Update(ctx context.Context, id string, input model.UpdateVideoInput) error {
	result, err := r.pool.Exec(ctx, queryUpdateVideo, id, input.Title, input.Description)
	if err != nil {
		return fmt.Errorf("failed to update video %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *videoRepository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDeleteVideo, id)
	if err != nil {
		return fmt.Errorf("failed to delete video %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *videoRepository) List(ctx context.Context, filter model.VideoFilter) ([]model.Video, int64, error) {
	whereClause, args := buildWhereClause(filter)

	countQuery := "SELECT COUNT(DISTINCT v.id) FROM videos v" + whereClause
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count videos: %w", err)
	}

	sortCol := allowedSortColumns[filter.SortBy]
	if sortCol == "" {
		sortCol = "v.created_at"
	}
	sortOrder := "DESC"
	if strings.EqualFold(filter.SortOrder, "asc") {
		sortOrder = "ASC"
	}

	offset := (filter.Page - 1) * filter.PageSize
	nextArg := len(args) + 1

	dataQuery := "SELECT DISTINCT v.id, v.title, v.description, v.minio_object_key, v.thumbnail_key, " +
		"v.duration_seconds, v.resolution, v.file_size_bytes, v.mime_type, " +
		"v.original_filename, v.created_at, v.updated_at, v.source_id, v.file_path " +
		"FROM videos v" + whereClause +
		" ORDER BY " + sortCol + " " + sortOrder +
		" LIMIT $" + strconv.Itoa(nextArg) + " OFFSET $" + strconv.Itoa(nextArg+1)
	args = append(args, filter.PageSize, offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list videos: %w", err)
	}
	defer rows.Close()

	var videos []model.Video
	for rows.Next() {
		var v model.Video
		if err := rows.Scan(
			&v.ID, &v.Title, &v.Description, &v.MinIOObjectKey, &v.ThumbnailKey,
			&v.DurationSeconds, &v.Resolution, &v.FileSizeBytes, &v.MimeType,
			&v.OriginalFilename, &v.CreatedAt, &v.UpdatedAt, &v.SourceID, &v.FilePath,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan video: %w", err)
		}
		videos = append(videos, v)
	}

	if videos == nil {
		videos = []model.Video{}
	}

	return videos, total, nil
}

func (r *videoRepository) FindBySourceAndPath(ctx context.Context, sourceID string, filePath string) (*model.Video, error) {
	var video model.Video
	err := r.pool.QueryRow(ctx, queryFindBySourceAndPath, sourceID, filePath).Scan(
		&video.ID, &video.Title, &video.Description, &video.MinIOObjectKey, &video.ThumbnailKey,
		&video.DurationSeconds, &video.Resolution, &video.FileSizeBytes, &video.MimeType,
		&video.OriginalFilename, &video.CreatedAt, &video.UpdatedAt, &video.SourceID, &video.FilePath,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, fmt.Errorf("failed to find video by source %s and path %s: %w", sourceID, filePath, err)
	}
	return &video, nil
}

func buildWhereClause(filter model.VideoFilter) (string, []interface{}) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if filter.Query != "" {
		conditions = append(conditions, "to_tsvector('simple', v.title) @@ to_tsquery('simple', $"+strconv.Itoa(argIdx)+")")
		args = append(args, filter.Query)
		argIdx++
	}

	if len(filter.TagIDs) > 0 {
		conditions = append(conditions,
			"v.id IN (SELECT vt.video_id FROM video_tags vt WHERE vt.tag_id = ANY($"+strconv.Itoa(argIdx)+") "+
				"GROUP BY vt.video_id HAVING COUNT(DISTINCT vt.tag_id) = $"+strconv.Itoa(argIdx+1)+")")
		args = append(args, filter.TagIDs, len(filter.TagIDs))
		argIdx += 2
	}

	clause := ""
	if len(conditions) > 0 {
		clause = " WHERE " + strings.Join(conditions, " AND ")
	}

	return clause, args
}
