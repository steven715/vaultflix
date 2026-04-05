package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

type VideoService struct {
	videoRepo repository.VideoRepository
	tagRepo   repository.TagRepository
	minioSvc  MinIOClient
}

func NewVideoService(videoRepo repository.VideoRepository, tagRepo repository.TagRepository, minioSvc MinIOClient) *VideoService {
	return &VideoService{
		videoRepo: videoRepo,
		tagRepo:   tagRepo,
		minioSvc:  minioSvc,
	}
}

func (s *VideoService) List(ctx context.Context, filter model.VideoFilter) ([]model.VideoWithTags, int64, error) {
	videos, total, err := s.videoRepo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list videos: %w", err)
	}

	if len(videos) == 0 {
		return []model.VideoWithTags{}, total, nil
	}

	videoIDs := make([]string, len(videos))
	for i, v := range videos {
		videoIDs[i] = v.ID
	}

	tagMap, err := s.tagRepo.GetByVideoIDs(ctx, videoIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to batch load tags: %w", err)
	}

	result := make([]model.VideoWithTags, len(videos))
	for i, v := range videos {
		tags := tagMap[v.ID]
		if tags == nil {
			tags = []model.Tag{}
		}

		var thumbnailURL string
		if v.ThumbnailKey != "" {
			url, err := s.minioSvc.GenerateThumbnailPresignedURL(ctx, v.ThumbnailKey, 0)
			if err != nil {
				slog.Warn("failed to generate thumbnail url for list",
					"video_id", v.ID,
					"error", err,
				)
			} else {
				thumbnailURL = url
			}
		}

		result[i] = model.VideoWithTags{
			Video:        v,
			Tags:         tags,
			ThumbnailURL: thumbnailURL,
		}
	}

	return result, total, nil
}

func (s *VideoService) GetByID(ctx context.Context, id string, urlExpiry time.Duration) (*model.VideoDetail, error) {
	video, err := s.videoRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get video %s: %w", id, err)
	}

	tags, err := s.tagRepo.GetByVideoID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get tags for video %s: %w", id, err)
	}

	streamURL, err := s.minioSvc.GeneratePresignedURL(ctx, video.MinIOObjectKey, urlExpiry)
	if err != nil {
		return nil, fmt.Errorf("failed to generate stream url for video %s: %w", id, err)
	}

	var thumbnailURL string
	if video.ThumbnailKey != "" {
		thumbnailURL, err = s.minioSvc.GenerateThumbnailPresignedURL(ctx, video.ThumbnailKey, urlExpiry)
		if err != nil {
			slog.Warn("failed to generate thumbnail url",
				"video_id", id,
				"error", err,
			)
		}
	}

	return &model.VideoDetail{
		VideoWithTags: model.VideoWithTags{
			Video:        *video,
			Tags:         tags,
			ThumbnailURL: thumbnailURL,
		},
		StreamURL: streamURL,
	}, nil
}

func (s *VideoService) Update(ctx context.Context, id string, input model.UpdateVideoInput) (*model.Video, error) {
	if err := s.videoRepo.Update(ctx, id, input); err != nil {
		return nil, fmt.Errorf("failed to update video %s: %w", id, err)
	}

	video, err := s.videoRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated video %s: %w", id, err)
	}

	return video, nil
}

func (s *VideoService) Delete(ctx context.Context, id string) error {
	video, err := s.videoRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get video for deletion %s: %w", id, err)
	}

	// Delete DB record first to avoid orphan DB data if MinIO fails
	if err := s.videoRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete video record %s: %w", id, err)
	}

	// Best-effort MinIO cleanup: log errors but don't fail the operation
	if video.MinIOObjectKey != "" {
		if err := s.minioSvc.DeleteVideo(ctx, video.MinIOObjectKey); err != nil {
			slog.Error("failed to delete video object from minio, orphan object may remain",
				"video_id", id,
				"object_key", video.MinIOObjectKey,
				"error", err,
			)
		}
	}

	if video.ThumbnailKey != "" {
		if err := s.minioSvc.DeleteThumbnail(ctx, video.ThumbnailKey); err != nil {
			slog.Error("failed to delete thumbnail from minio, orphan object may remain",
				"video_id", id,
				"thumbnail_key", video.ThumbnailKey,
				"error", err,
			)
		}
	}

	return nil
}
