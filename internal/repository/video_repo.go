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
// UpdatePreviewKey returns model.ErrNotFound when the video does not exist.
// UpdateMetadata applies enriched scalar fields to a video and sets enrichment_status='enriched'.
// SetEnrichmentStatus updates the enrichment_status column for the given video.
// ListByEnrichmentStatus returns videos matching the given enrichment_status, ordered by created_at.
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
	// ListMissingPreviews returns all videos whose preview_key is empty,
	// ordered by created_at ASC. Used by the preview-backfill job.
	ListMissingPreviews(ctx context.Context) ([]model.Video, error)
	// UpdatePreviewKey persists the given preview object key for the video.
	UpdatePreviewKey(ctx context.Context, id string, previewKey string) error
	// UpdateMetadata applies enriched scalar fields to a video and marks it as 'enriched'.
	UpdateMetadata(ctx context.Context, id string, m model.VideoMetadataUpdate) error
	// SetEnrichmentStatus updates the enrichment_status column for the given video.
	SetEnrichmentStatus(ctx context.Context, id, status string) error
	// ListByEnrichmentStatus returns all videos with the given enrichment_status, ordered by created_at.
	ListByEnrichmentStatus(ctx context.Context, status string) ([]model.Video, error)
	// SeedCode sets the code and enrichment_status columns for an existing video.
	// Returns model.ErrNotFound when no video with the given id exists.
	SeedCode(ctx context.Context, id, code, status string) error
	// UpdateCodecs persists the video and audio codec for the given video.
	// Returns model.ErrNotFound when no video with the given id exists.
	UpdateCodecs(ctx context.Context, id, videoCodec, audioCodec string) error
	// ListMissingCodecs returns videos whose video_codec is NULL or empty and
	// whose source_id and file_path are non-null, up to limit rows.
	ListMissingCodecs(ctx context.Context, limit int) ([]model.Video, error)
	// ListKeyframeCandidates returns videos with known codecs but no keyframe
	// index yet (remux filtering happens in the service layer).
	ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error)
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
    INSERT INTO videos (id, title, description, minio_object_key, thumbnail_key, preview_key,
                        duration_seconds, resolution, file_size_bytes, mime_type, video_codec, audio_codec,
                        original_filename, source_id, file_path, code, enrichment_status)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
    RETURNING created_at, updated_at
`

const queryGetVideoByID = `
    SELECT id, title, description, minio_object_key, thumbnail_key, preview_key,
           duration_seconds, resolution, file_size_bytes, mime_type,
           COALESCE(video_codec, '') AS video_codec, COALESCE(audio_codec, '') AS audio_codec,
           original_filename, created_at, updated_at, source_id, file_path
    FROM videos
    WHERE id = $1
`

const queryFindBySourceAndPath = `
    SELECT id, title, description, minio_object_key, thumbnail_key, preview_key,
           duration_seconds, resolution, file_size_bytes, mime_type,
           COALESCE(video_codec, '') AS video_codec, COALESCE(audio_codec, '') AS audio_codec,
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

const queryListMissingPreviews = `
    SELECT id, title, description, minio_object_key, thumbnail_key, preview_key,
           duration_seconds, resolution, file_size_bytes, mime_type,
           COALESCE(video_codec, '') AS video_codec, COALESCE(audio_codec, '') AS audio_codec,
           original_filename, created_at, updated_at, source_id, file_path
    FROM videos
    WHERE preview_key IS NULL OR preview_key = ''
    ORDER BY created_at ASC
`

const queryUpdateVideoPreviewKey = `
    UPDATE videos
    SET preview_key = $2, updated_at = NOW()
    WHERE id = $1
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
		video.ID, video.Title, video.Description, video.MinIOObjectKey, video.ThumbnailKey, video.PreviewKey,
		video.DurationSeconds, video.Resolution, video.FileSizeBytes, video.MimeType, video.VideoCodec, video.AudioCodec,
		video.OriginalFilename, video.SourceID, video.FilePath, video.Code, video.EnrichmentStatus,
	).Scan(&video.CreatedAt, &video.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create video %s: %w", video.OriginalFilename, err)
	}

	return nil
}

func (r *videoRepository) GetByID(ctx context.Context, id string) (*model.Video, error) {
	var video model.Video
	err := r.pool.QueryRow(ctx, queryGetVideoByID, id).Scan(
		&video.ID, &video.Title, &video.Description, &video.MinIOObjectKey, &video.ThumbnailKey, &video.PreviewKey,
		&video.DurationSeconds, &video.Resolution, &video.FileSizeBytes, &video.MimeType,
		&video.VideoCodec, &video.AudioCodec,
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

	offset := (filter.Page - 1) * filter.PageSize
	nextArg := len(args) + 1

	selectClause := "SELECT DISTINCT v.id, v.title, v.description, v.minio_object_key, v.thumbnail_key, v.preview_key, " +
		"v.duration_seconds, v.resolution, v.file_size_bytes, v.mime_type, " +
		"COALESCE(v.video_codec, '') AS video_codec, COALESCE(v.audio_codec, '') AS audio_codec, " +
		"v.original_filename, v.created_at, v.updated_at, v.source_id, v.file_path " +
		"FROM videos v" + whereClause

	limitOffset := " LIMIT $" + strconv.Itoa(nextArg) + " OFFSET $" + strconv.Itoa(nextArg+1)

	var dataQuery string
	if filter.SortBy == "random" {
		// SELECT DISTINCT forbids ORDER BY RANDOM() (the ORDER BY expression must
		// appear in the select list), so wrap the DISTINCT rows in a subquery and
		// shuffle the outer result. sort_order is irrelevant for a random sample.
		dataQuery = "SELECT * FROM (" + selectClause + ") sub ORDER BY RANDOM()" + limitOffset
	} else {
		sortCol := allowedSortColumns[filter.SortBy]
		if sortCol == "" {
			sortCol = "v.created_at"
		}
		sortOrder := "DESC"
		if strings.EqualFold(filter.SortOrder, "asc") {
			sortOrder = "ASC"
		}
		dataQuery = selectClause + " ORDER BY " + sortCol + " " + sortOrder + limitOffset
	}
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
			&v.ID, &v.Title, &v.Description, &v.MinIOObjectKey, &v.ThumbnailKey, &v.PreviewKey,
			&v.DurationSeconds, &v.Resolution, &v.FileSizeBytes, &v.MimeType,
			&v.VideoCodec, &v.AudioCodec,
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

func (r *videoRepository) ListMissingPreviews(ctx context.Context) ([]model.Video, error) {
	rows, err := r.pool.Query(ctx, queryListMissingPreviews)
	if err != nil {
		return nil, fmt.Errorf("failed to list videos missing previews: %w", err)
	}
	defer rows.Close()

	var videos []model.Video
	for rows.Next() {
		var v model.Video
		if err := rows.Scan(
			&v.ID, &v.Title, &v.Description, &v.MinIOObjectKey, &v.ThumbnailKey, &v.PreviewKey,
			&v.DurationSeconds, &v.Resolution, &v.FileSizeBytes, &v.MimeType,
			&v.VideoCodec, &v.AudioCodec,
			&v.OriginalFilename, &v.CreatedAt, &v.UpdatedAt, &v.SourceID, &v.FilePath,
		); err != nil {
			return nil, fmt.Errorf("failed to scan video missing preview: %w", err)
		}
		videos = append(videos, v)
	}

	if videos == nil {
		videos = []model.Video{}
	}

	return videos, nil
}

func (r *videoRepository) UpdatePreviewKey(ctx context.Context, id string, previewKey string) error {
	result, err := r.pool.Exec(ctx, queryUpdateVideoPreviewKey, id, previewKey)
	if err != nil {
		return fmt.Errorf("failed to update preview key for video %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *videoRepository) FindBySourceAndPath(ctx context.Context, sourceID string, filePath string) (*model.Video, error) {
	var video model.Video
	err := r.pool.QueryRow(ctx, queryFindBySourceAndPath, sourceID, filePath).Scan(
		&video.ID, &video.Title, &video.Description, &video.MinIOObjectKey, &video.ThumbnailKey, &video.PreviewKey,
		&video.DurationSeconds, &video.Resolution, &video.FileSizeBytes, &video.MimeType,
		&video.VideoCodec, &video.AudioCodec,
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
