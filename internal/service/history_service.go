package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// WatchHistoryService defines the contract for watch history business logic.
// SaveProgress auto-marks completed when progress >= 90% of video duration.
// GetProgress returns (0, nil) when the user has never watched the video — not an error.
// List returns items with presigned thumbnail URLs.
type WatchHistoryService interface {
	SaveProgress(ctx context.Context, userID, videoID string, progressSeconds int) error
	List(ctx context.Context, userID string, page, pageSize int) ([]model.WatchHistoryItem, int64, error)
	GetProgress(ctx context.Context, userID, videoID string) (int, error)
}

type watchHistoryService struct {
	historyRepo repository.WatchHistoryRepository
	videoRepo   repository.VideoRepository
	minioSvc    MinIOClient
}

func NewWatchHistoryService(
	historyRepo repository.WatchHistoryRepository,
	videoRepo repository.VideoRepository,
	minioSvc MinIOClient,
) WatchHistoryService {
	return &watchHistoryService{
		historyRepo: historyRepo,
		videoRepo:   videoRepo,
		minioSvc:    minioSvc,
	}
}

func (s *watchHistoryService) SaveProgress(ctx context.Context, userID, videoID string, progressSeconds int) error {
	video, err := s.videoRepo.GetByID(ctx, videoID)
	if err != nil {
		return fmt.Errorf("failed to get video %s for progress save: %w", videoID, err)
	}

	completed := false
	if video.DurationSeconds > 0 && float64(progressSeconds) >= float64(video.DurationSeconds)*0.9 {
		completed = true
	}

	record := &model.WatchHistory{
		UserID:          userID,
		VideoID:         videoID,
		ProgressSeconds: progressSeconds,
		Completed:       completed,
	}

	if err := s.historyRepo.Upsert(ctx, record); err != nil {
		return fmt.Errorf("failed to save progress for user %s video %s: %w", userID, videoID, err)
	}

	return nil
}

func (s *watchHistoryService) List(ctx context.Context, userID string, page, pageSize int) ([]model.WatchHistoryItem, int64, error) {
	items, total, err := s.historyRepo.ListByUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list watch history for user %s: %w", userID, err)
	}

	result := make([]model.WatchHistoryItem, len(items))
	for i, item := range items {
		var thumbnailURL string
		if item.ThumbnailKey != "" {
			url, err := s.minioSvc.GenerateThumbnailPresignedURL(ctx, item.ThumbnailKey, 0)
			if err != nil {
				slog.Warn("failed to generate thumbnail url for watch history",
					"video_id", item.VideoID,
					"error", err,
				)
			} else {
				thumbnailURL = url
			}
		}

		result[i] = model.WatchHistoryItem{
			ID:              item.ID,
			VideoID:         item.VideoID,
			Title:           item.Title,
			ThumbnailURL:    thumbnailURL,
			DurationSeconds: item.DurationSeconds,
			ProgressSeconds: item.ProgressSeconds,
			Completed:       item.Completed,
			WatchedAt:       item.WatchedAt,
		}
	}

	return result, total, nil
}

func (s *watchHistoryService) GetProgress(ctx context.Context, userID, videoID string) (int, error) {
	record, err := s.historyRepo.GetByUserAndVideo(ctx, userID, videoID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get progress for user %s video %s: %w", userID, videoID, err)
	}
	return record.ProgressSeconds, nil
}
