package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// RecommendationService defines the contract for daily recommendation business logic.
// GetToday returns manual recommendations if available, otherwise falls back to random unwatched videos.
// Create returns a wrapped error containing model.ErrNotFound if the video does not exist,
// or model.ErrConflict if the same video+date already exists.
// Delete returns a wrapped error containing model.ErrNotFound if the recommendation does not exist.
// UpdateSortOrder returns a wrapped error containing model.ErrNotFound if the recommendation does not exist.
type RecommendationService interface {
	GetToday(ctx context.Context, userID string, date time.Time, fallbackCount int) ([]model.RecommendationItem, error)
	ListByDate(ctx context.Context, date time.Time) ([]model.RecommendationItem, error)
	Create(ctx context.Context, videoID string, date time.Time, sortOrder int) (*model.DailyRecommendation, error)
	UpdateSortOrder(ctx context.Context, id string, sortOrder int) error
	Delete(ctx context.Context, id string) error
}

type recommendationService struct {
	recRepo   repository.RecommendationRepository
	videoRepo repository.VideoRepository
	minioSvc  MinIOClient
}

func NewRecommendationService(
	recRepo repository.RecommendationRepository,
	videoRepo repository.VideoRepository,
	minioSvc MinIOClient,
) RecommendationService {
	return &recommendationService{
		recRepo:   recRepo,
		videoRepo: videoRepo,
		minioSvc:  minioSvc,
	}
}

func (s *recommendationService) GetToday(ctx context.Context, userID string, date time.Time, fallbackCount int) ([]model.RecommendationItem, error) {
	recs, err := s.recRepo.ListByDate(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("failed to list today's recommendations: %w", err)
	}

	if len(recs) > 0 {
		items := make([]model.RecommendationItem, len(recs))
		for i, rec := range recs {
			thumbnailURL := s.generateThumbnailURL(ctx, rec.ThumbnailKey)
			items[i] = model.RecommendationItem{
				ID:              rec.ID,
				VideoID:         rec.VideoID,
				Title:           rec.Title,
				ThumbnailURL:    thumbnailURL,
				DurationSeconds: rec.DurationSeconds,
				Resolution:      rec.Resolution,
				FileSizeBytes:   rec.FileSizeBytes,
				SortOrder:       rec.SortOrder,
				IsFallback:      false,
			}
		}
		return items, nil
	}

	// Fallback to random unwatched videos
	videos, err := s.recRepo.GetRandomUnwatched(ctx, userID, fallbackCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get random unwatched videos: %w", err)
	}

	items := make([]model.RecommendationItem, len(videos))
	for i, v := range videos {
		thumbnailURL := s.generateThumbnailURL(ctx, v.ThumbnailKey)
		items[i] = model.RecommendationItem{
			ID:              v.ID,
			VideoID:         v.ID,
			Title:           v.Title,
			ThumbnailURL:    thumbnailURL,
			DurationSeconds: v.DurationSeconds,
			Resolution:      v.Resolution,
			FileSizeBytes:   v.FileSizeBytes,
			SortOrder:       i + 1,
			IsFallback:      true,
		}
	}

	return items, nil
}

func (s *recommendationService) ListByDate(ctx context.Context, date time.Time) ([]model.RecommendationItem, error) {
	recs, err := s.recRepo.ListByDate(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("failed to list recommendations for %s: %w", date.Format("2006-01-02"), err)
	}

	items := make([]model.RecommendationItem, len(recs))
	for i, rec := range recs {
		thumbnailURL := s.generateThumbnailURL(ctx, rec.ThumbnailKey)
		items[i] = model.RecommendationItem{
			ID:              rec.ID,
			VideoID:         rec.VideoID,
			Title:           rec.Title,
			ThumbnailURL:    thumbnailURL,
			DurationSeconds: rec.DurationSeconds,
			Resolution:      rec.Resolution,
			FileSizeBytes:   rec.FileSizeBytes,
			SortOrder:       rec.SortOrder,
			IsFallback:      false,
		}
	}
	return items, nil
}

func (s *recommendationService) Create(ctx context.Context, videoID string, date time.Time, sortOrder int) (*model.DailyRecommendation, error) {
	// Verify video exists
	if _, err := s.videoRepo.GetByID(ctx, videoID); err != nil {
		return nil, fmt.Errorf("failed to verify video %s for recommendation: %w", videoID, err)
	}

	rec := &model.DailyRecommendation{
		VideoID:       videoID,
		RecommendDate: date,
		SortOrder:     sortOrder,
	}

	if err := s.recRepo.Create(ctx, rec); err != nil {
		return nil, fmt.Errorf("failed to create recommendation: %w", err)
	}

	return rec, nil
}

func (s *recommendationService) UpdateSortOrder(ctx context.Context, id string, sortOrder int) error {
	if err := s.recRepo.UpdateSortOrder(ctx, id, sortOrder); err != nil {
		return fmt.Errorf("failed to update sort order for recommendation %s: %w", id, err)
	}
	return nil
}

func (s *recommendationService) Delete(ctx context.Context, id string) error {
	if err := s.recRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete recommendation %s: %w", id, err)
	}
	return nil
}

func (s *recommendationService) generateThumbnailURL(ctx context.Context, thumbnailKey string) string {
	if thumbnailKey == "" {
		return ""
	}
	url, err := s.minioSvc.GenerateThumbnailPresignedURL(ctx, thumbnailKey, 0)
	if err != nil {
		slog.Warn("failed to generate thumbnail url for recommendation",
			"thumbnail_key", thumbnailKey,
			"error", err,
		)
		return ""
	}
	return url
}
