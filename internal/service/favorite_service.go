package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// FavoriteService defines the contract for favorite business logic.
// Add is idempotent: adding an already-favorited video silently succeeds.
// Remove returns model.ErrNotFound when the favorite does not exist.
// IsFavorited returns (false, nil) when not favorited — not an error.
// List returns items with presigned thumbnail URLs.
type FavoriteService interface {
	Add(ctx context.Context, userID, videoID string) error
	Remove(ctx context.Context, userID, videoID string) error
	IsFavorited(ctx context.Context, userID, videoID string) (bool, error)
	List(ctx context.Context, userID string, page, pageSize int) ([]model.VideoSummaryWithURL, int64, error)
}

type favoriteService struct {
	favoriteRepo repository.FavoriteRepository
	minioSvc     MinIOClient
}

func NewFavoriteService(
	favoriteRepo repository.FavoriteRepository,
	minioSvc MinIOClient,
) FavoriteService {
	return &favoriteService{
		favoriteRepo: favoriteRepo,
		minioSvc:     minioSvc,
	}
}

func (s *favoriteService) Add(ctx context.Context, userID, videoID string) error {
	err := s.favoriteRepo.Add(ctx, userID, videoID)
	if err != nil {
		if errors.Is(err, model.ErrAlreadyExists) {
			return nil
		}
		return fmt.Errorf("failed to add favorite for user %s video %s: %w", userID, videoID, err)
	}
	return nil
}

func (s *favoriteService) Remove(ctx context.Context, userID, videoID string) error {
	if err := s.favoriteRepo.Remove(ctx, userID, videoID); err != nil {
		return fmt.Errorf("failed to remove favorite for user %s video %s: %w", userID, videoID, err)
	}
	return nil
}

func (s *favoriteService) IsFavorited(ctx context.Context, userID, videoID string) (bool, error) {
	exists, err := s.favoriteRepo.Exists(ctx, userID, videoID)
	if err != nil {
		return false, fmt.Errorf("failed to check favorite for user %s video %s: %w", userID, videoID, err)
	}
	return exists, nil
}

func (s *favoriteService) List(ctx context.Context, userID string, page, pageSize int) ([]model.VideoSummaryWithURL, int64, error) {
	items, total, err := s.favoriteRepo.ListByUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list favorites for user %s: %w", userID, err)
	}

	result := make([]model.VideoSummaryWithURL, len(items))
	for i, item := range items {
		var thumbnailURL string
		if item.ThumbnailKey != "" {
			url, err := s.minioSvc.GenerateThumbnailPresignedURL(ctx, item.ThumbnailKey, 0)
			if err != nil {
				slog.Warn("failed to generate thumbnail url for favorite",
					"video_id", item.ID,
					"error", err,
				)
			} else {
				thumbnailURL = url
			}
		}

		result[i] = model.VideoSummaryWithURL{
			ID:              item.ID,
			Title:           item.Title,
			ThumbnailURL:    thumbnailURL,
			DurationSeconds: item.DurationSeconds,
			Resolution:      item.Resolution,
			FileSizeBytes:   item.FileSizeBytes,
			CreatedAt:       item.CreatedAt,
		}
	}

	return result, total, nil
}
