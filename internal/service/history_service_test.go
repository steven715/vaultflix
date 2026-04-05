package service

import (
	"context"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestSaveProgress_NewVideo(t *testing.T) {
	var capturedRecord *model.WatchHistory
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: "vid-1", DurationSeconds: 600}, nil
		},
	}
	historyRepo := &mock.WatchHistoryRepository{
		UpsertFunc: func(ctx context.Context, record *model.WatchHistory) error {
			capturedRecord = record
			return nil
		},
	}
	minioSvc := &mock.MinIOClient{}

	svc := NewWatchHistoryService(historyRepo, videoRepo, minioSvc)
	err := svc.SaveProgress(context.Background(), "user-1", "vid-1", 120)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedRecord == nil {
		t.Fatal("expected upsert to be called")
	}
	if capturedRecord.UserID != "user-1" {
		t.Errorf("expected user_id user-1, got %s", capturedRecord.UserID)
	}
	if capturedRecord.VideoID != "vid-1" {
		t.Errorf("expected video_id vid-1, got %s", capturedRecord.VideoID)
	}
	if capturedRecord.ProgressSeconds != 120 {
		t.Errorf("expected progress 120, got %d", capturedRecord.ProgressSeconds)
	}
	if capturedRecord.Completed {
		t.Error("expected completed to be false for 120/600")
	}
}

func TestSaveProgress_AutoComplete(t *testing.T) {
	var capturedRecord *model.WatchHistory
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: "vid-1", DurationSeconds: 100}, nil
		},
	}
	historyRepo := &mock.WatchHistoryRepository{
		UpsertFunc: func(ctx context.Context, record *model.WatchHistory) error {
			capturedRecord = record
			return nil
		},
	}
	minioSvc := &mock.MinIOClient{}

	svc := NewWatchHistoryService(historyRepo, videoRepo, minioSvc)
	// 90/100 = 90%, should mark as completed
	err := svc.SaveProgress(context.Background(), "user-1", "vid-1", 90)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !capturedRecord.Completed {
		t.Error("expected completed to be true for progress >= 90% duration")
	}
}

func TestSaveProgress_UpdateExisting(t *testing.T) {
	upsertCalled := false
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: "vid-1", DurationSeconds: 600}, nil
		},
	}
	historyRepo := &mock.WatchHistoryRepository{
		UpsertFunc: func(ctx context.Context, record *model.WatchHistory) error {
			upsertCalled = true
			return nil
		},
	}
	minioSvc := &mock.MinIOClient{}

	svc := NewWatchHistoryService(historyRepo, videoRepo, minioSvc)
	err := svc.SaveProgress(context.Background(), "user-1", "vid-1", 300)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !upsertCalled {
		t.Error("expected upsert to be called for update")
	}
}

func TestGetProgress_NeverWatched(t *testing.T) {
	historyRepo := &mock.WatchHistoryRepository{
		GetByUserAndVideoFunc: func(ctx context.Context, userID, videoID string) (*model.WatchHistory, error) {
			return nil, model.ErrNotFound
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewWatchHistoryService(historyRepo, videoRepo, minioSvc)
	progress, err := svc.GetProgress(context.Background(), "user-1", "vid-1")
	if err != nil {
		t.Fatalf("expected nil error for never-watched, got %v", err)
	}
	if progress != 0 {
		t.Errorf("expected progress 0, got %d", progress)
	}
}

func TestList_WithThumbnails(t *testing.T) {
	historyRepo := &mock.WatchHistoryRepository{
		ListByUserFunc: func(ctx context.Context, userID string, page, pageSize int) ([]model.WatchHistoryWithVideo, int64, error) {
			return []model.WatchHistoryWithVideo{
				{
					ID:              "wh-1",
					VideoID:         "vid-1",
					Title:           "Test Video",
					ThumbnailKey:    "thumbnails/vid-1.jpg",
					DurationSeconds: 600,
					ProgressSeconds: 300,
					Completed:       false,
					WatchedAt:       time.Now(),
				},
			}, 1, nil
		},
	}
	videoRepo := &mock.VideoRepository{}
	minioSvc := &mock.MinIOClient{
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb/" + objectKey, nil
		},
	}

	svc := NewWatchHistoryService(historyRepo, videoRepo, minioSvc)
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
	if items[0].VideoID != "vid-1" {
		t.Errorf("expected video_id vid-1, got %s", items[0].VideoID)
	}
}
