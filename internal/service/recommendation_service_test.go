package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestGetToday_ManualRecommendations(t *testing.T) {
	recs := []model.RecommendationWithVideo{
		{
			ID:              "rec-1",
			VideoID:         "vid-1",
			Title:           "Manual Pick 1",
			ThumbnailKey:    "thumbnails/vid-1.jpg",
			DurationSeconds: 3600,
			SortOrder:       1,
		},
		{
			ID:              "rec-2",
			VideoID:         "vid-2",
			Title:           "Manual Pick 2",
			ThumbnailKey:    "thumbnails/vid-2.jpg",
			DurationSeconds: 1800,
			SortOrder:       2,
		},
	}

	recRepo := &mock.RecommendationRepository{
		ListByDateFunc: func(ctx context.Context, date time.Time) ([]model.RecommendationWithVideo, error) {
			return recs, nil
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb-" + objectKey, nil
		},
	}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	items, err := svc.GetToday(context.Background(), "user-1", time.Now().Truncate(24*time.Hour), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].IsFallback {
		t.Error("expected is_fallback to be false for manual recommendations")
	}
	if items[0].Title != "Manual Pick 1" {
		t.Errorf("expected title 'Manual Pick 1', got %s", items[0].Title)
	}
	if items[0].ThumbnailURL != "https://minio/thumb-thumbnails/vid-1.jpg" {
		t.Errorf("expected thumbnail url, got %s", items[0].ThumbnailURL)
	}
}

func TestGetToday_FallbackToRandom(t *testing.T) {
	videos := []model.Video{
		{ID: "vid-10", Title: "Random 1", ThumbnailKey: "thumbnails/vid-10.jpg", DurationSeconds: 600},
		{ID: "vid-11", Title: "Random 2", ThumbnailKey: "thumbnails/vid-11.jpg", DurationSeconds: 900},
	}

	var capturedLimit int
	recRepo := &mock.RecommendationRepository{
		ListByDateFunc: func(ctx context.Context, date time.Time) ([]model.RecommendationWithVideo, error) {
			return []model.RecommendationWithVideo{}, nil
		},
		GetRandomUnwatchedFunc: func(ctx context.Context, userID string, limit int) ([]model.Video, error) {
			capturedLimit = limit
			return videos, nil
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb-" + objectKey, nil
		},
	}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	items, err := svc.GetToday(context.Background(), "user-1", time.Now().Truncate(24*time.Hour), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedLimit != 5 {
		t.Errorf("expected fallback limit 5, got %d", capturedLimit)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if !items[0].IsFallback {
		t.Error("expected is_fallback to be true for random recommendations")
	}
	if items[0].VideoID != "vid-10" {
		t.Errorf("expected video_id vid-10, got %s", items[0].VideoID)
	}
}

func TestGetToday_NoRecommendationsAtAll(t *testing.T) {
	recRepo := &mock.RecommendationRepository{
		ListByDateFunc: func(ctx context.Context, date time.Time) ([]model.RecommendationWithVideo, error) {
			return []model.RecommendationWithVideo{}, nil
		},
		GetRandomUnwatchedFunc: func(ctx context.Context, userID string, limit int) ([]model.Video, error) {
			return []model.Video{}, nil
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	items, err := svc.GetToday(context.Background(), "user-1", time.Now().Truncate(24*time.Hour), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if items == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestCreate_VideoNotFound(t *testing.T) {
	recRepo := &mock.RecommendationRepository{}
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return nil, model.ErrNotFound
		},
	}
	minioSvc := &mock.MinIOClient{}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	_, err := svc.Create(context.Background(), "nonexistent", time.Now(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCreate_DuplicateConflict(t *testing.T) {
	recRepo := &mock.RecommendationRepository{
		CreateFunc: func(ctx context.Context, rec *model.DailyRecommendation) error {
			return model.ErrConflict
		},
	}
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: "vid-1"}, nil
		},
	}
	minioSvc := &mock.MinIOClient{}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	_, err := svc.Create(context.Background(), "vid-1", time.Now(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, model.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestListByDate_Success(t *testing.T) {
	recs := []model.RecommendationWithVideo{
		{
			ID:              "rec-1",
			VideoID:         "vid-1",
			Title:           "Video One",
			ThumbnailKey:    "thumbnails/vid-1.jpg",
			DurationSeconds: 3600,
			SortOrder:       1,
		},
		{
			ID:              "rec-2",
			VideoID:         "vid-2",
			Title:           "Video Two",
			ThumbnailKey:    "thumbnails/vid-2.jpg",
			DurationSeconds: 1800,
			SortOrder:       2,
		},
	}

	recRepo := &mock.RecommendationRepository{
		ListByDateFunc: func(ctx context.Context, date time.Time) ([]model.RecommendationWithVideo, error) {
			return recs, nil
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb-" + objectKey, nil
		},
	}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	items, err := svc.ListByDate(context.Background(), time.Now().Truncate(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Title != "Video One" {
		t.Errorf("expected title 'Video One', got %s", items[0].Title)
	}
	if items[0].ThumbnailURL != "https://minio/thumb-thumbnails/vid-1.jpg" {
		t.Errorf("expected thumbnail url, got %s", items[0].ThumbnailURL)
	}
	if items[1].Title != "Video Two" {
		t.Errorf("expected title 'Video Two', got %s", items[1].Title)
	}
	if items[0].IsFallback {
		t.Error("expected is_fallback to be false")
	}
}

func TestListByDate_Empty(t *testing.T) {
	recRepo := &mock.RecommendationRepository{
		ListByDateFunc: func(ctx context.Context, date time.Time) ([]model.RecommendationWithVideo, error) {
			return []model.RecommendationWithVideo{}, nil
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	items, err := svc.ListByDate(context.Background(), time.Now().Truncate(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestUpdateSortOrder_Success(t *testing.T) {
	recRepo := &mock.RecommendationRepository{
		UpdateSortOrderFunc: func(ctx context.Context, id string, sortOrder int) error {
			return nil
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	err := svc.UpdateSortOrder(context.Background(), "rec-1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateSortOrder_NotFound(t *testing.T) {
	recRepo := &mock.RecommendationRepository{
		UpdateSortOrderFunc: func(ctx context.Context, id string, sortOrder int) error {
			return model.ErrNotFound
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	err := svc.UpdateSortOrder(context.Background(), "nonexistent", 5)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDelete_Success(t *testing.T) {
	recRepo := &mock.RecommendationRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	err := svc.Delete(context.Background(), "rec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	recRepo := &mock.RecommendationRepository{
		DeleteFunc: func(ctx context.Context, id string) error {
			return model.ErrNotFound
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewRecommendationService(recRepo, videoRepo, minioSvc)
	err := svc.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
