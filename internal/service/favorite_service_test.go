package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestAddFavorite_Success(t *testing.T) {
	addCalled := false
	favoriteRepo := &mock.FavoriteRepository{
		AddFunc: func(ctx context.Context, userID, videoID string) error {
			addCalled = true
			return nil
		},
	}
	minioSvc := &mock.MinIOClient{}

	svc := NewFavoriteService(favoriteRepo, minioSvc)
	err := svc.Add(context.Background(), "user-1", "vid-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !addCalled {
		t.Error("expected Add to be called")
	}
}

func TestAddFavorite_AlreadyExists(t *testing.T) {
	favoriteRepo := &mock.FavoriteRepository{
		AddFunc: func(ctx context.Context, userID, videoID string) error {
			return model.ErrAlreadyExists
		},
	}
	minioSvc := &mock.MinIOClient{}

	svc := NewFavoriteService(favoriteRepo, minioSvc)
	err := svc.Add(context.Background(), "user-1", "vid-1")
	if err != nil {
		t.Fatalf("expected nil error for idempotent add, got %v", err)
	}
}

func TestRemoveFavorite_NotFound(t *testing.T) {
	favoriteRepo := &mock.FavoriteRepository{
		RemoveFunc: func(ctx context.Context, userID, videoID string) error {
			return model.ErrNotFound
		},
	}
	minioSvc := &mock.MinIOClient{}

	svc := NewFavoriteService(favoriteRepo, minioSvc)
	err := svc.Remove(context.Background(), "user-1", "vid-1")
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListFavorites_WithThumbnails(t *testing.T) {
	favoriteRepo := &mock.FavoriteRepository{
		ListByUserFunc: func(ctx context.Context, userID string, page, pageSize int) ([]model.VideoSummary, int64, error) {
			return []model.VideoSummary{
				{
					ID:              "vid-1",
					Title:           "Fav Video",
					ThumbnailKey:    "thumbnails/vid-1.jpg",
					DurationSeconds: 300,
					Resolution:      "1920x1080",
					FileSizeBytes:   1048576,
					CreatedAt:       time.Now(),
				},
			}, 1, nil
		},
	}
	minioSvc := &mock.MinIOClient{
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb/" + objectKey, nil
		},
	}

	svc := NewFavoriteService(favoriteRepo, minioSvc)
	items, total, err := svc.List(context.Background(), "user-1", 1, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ThumbnailURL == "" {
		t.Error("expected thumbnail URL to be generated")
	}
	if items[0].ID != "vid-1" {
		t.Errorf("expected id vid-1, got %s", items[0].ID)
	}
}
