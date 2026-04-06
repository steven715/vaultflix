package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestVideoService_GetByID_Success(t *testing.T) {
	video := &model.Video{
		ID:             "vid-1",
		Title:          "Test Video",
		MinIOObjectKey: "videos/vid-1/test.mp4",
		ThumbnailKey:   "thumbnails/vid-1.jpg",
	}
	tags := []model.Tag{
		{ID: 1, Name: "action", Category: "genre"},
	}

	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			if id == "vid-1" {
				return video, nil
			}
			return nil, model.ErrNotFound
		},
	}
	tagRepo := &mock.TagRepository{
		GetByVideoIDFunc: func(ctx context.Context, videoID string) ([]model.Tag, error) {
			return tags, nil
		},
	}
	minioSvc := &mock.MinIOClient{
		GeneratePresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/stream-url", nil
		},
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb-url", nil
		},
	}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	detail, err := svc.GetByID(context.Background(), "vid-1", 2*time.Hour, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detail.ID != "vid-1" {
		t.Errorf("expected video id vid-1, got %s", detail.ID)
	}
	if detail.StreamURL != "https://minio/stream-url" {
		t.Errorf("expected stream url, got %s", detail.StreamURL)
	}
	if detail.ThumbnailURL != "https://minio/thumb-url" {
		t.Errorf("expected thumbnail url, got %s", detail.ThumbnailURL)
	}
	if len(detail.Tags) != 1 || detail.Tags[0].Name != "action" {
		t.Errorf("expected 1 tag 'action', got %v", detail.Tags)
	}
}

func TestVideoService_GetByID_NotFound(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return nil, model.ErrNotFound
		},
	}
	tagRepo := &mock.TagRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	_, err := svc.GetByID(context.Background(), "nonexistent", 2*time.Hour, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestVideoService_List_WithTags(t *testing.T) {
	videos := []model.Video{
		{ID: "v1", Title: "Video 1"},
		{ID: "v2", Title: "Video 2"},
	}
	tagMap := map[string][]model.Tag{
		"v1": {{ID: 1, Name: "action", Category: "genre"}},
	}

	var capturedVideoIDs []string
	videoRepo := &mock.VideoRepository{
		ListFunc: func(ctx context.Context, filter model.VideoFilter) ([]model.Video, int64, error) {
			return videos, 2, nil
		},
	}
	tagRepo := &mock.TagRepository{
		GetByVideoIDsFunc: func(ctx context.Context, videoIDs []string) (map[string][]model.Tag, error) {
			capturedVideoIDs = videoIDs
			return tagMap, nil
		},
	}
	minioSvc := &mock.MinIOClient{}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	result, total, err := svc.List(context.Background(), model.VideoFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// Verify GetByVideoIDs was called with correct IDs (batch, not N+1)
	if len(capturedVideoIDs) != 2 || capturedVideoIDs[0] != "v1" || capturedVideoIDs[1] != "v2" {
		t.Errorf("expected GetByVideoIDs called with [v1, v2], got %v", capturedVideoIDs)
	}

	// v1 has tags, v2 has empty tags
	if len(result[0].Tags) != 1 {
		t.Errorf("expected 1 tag for v1, got %d", len(result[0].Tags))
	}
	if len(result[1].Tags) != 0 {
		t.Errorf("expected 0 tags for v2, got %d", len(result[1].Tags))
	}
}

func TestVideoService_List_WithThumbnailURLs(t *testing.T) {
	videos := []model.Video{
		{ID: "v1", Title: "Video 1", ThumbnailKey: "thumbnails/v1.jpg"},
		{ID: "v2", Title: "Video 2"},
	}
	tagMap := map[string][]model.Tag{}

	videoRepo := &mock.VideoRepository{
		ListFunc: func(ctx context.Context, filter model.VideoFilter) ([]model.Video, int64, error) {
			return videos, 2, nil
		},
	}
	tagRepo := &mock.TagRepository{
		GetByVideoIDsFunc: func(ctx context.Context, videoIDs []string) (map[string][]model.Tag, error) {
			return tagMap, nil
		},
	}
	minioSvc := &mock.MinIOClient{
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb-" + objectKey, nil
		},
	}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	result, _, err := svc.List(context.Background(), model.VideoFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result[0].ThumbnailURL != "https://minio/thumb-thumbnails/v1.jpg" {
		t.Errorf("expected thumbnail url for v1, got %s", result[0].ThumbnailURL)
	}
	if result[1].ThumbnailURL != "" {
		t.Errorf("expected empty thumbnail url for v2, got %s", result[1].ThumbnailURL)
	}
}

func TestVideoService_List_EmptyResult(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		ListFunc: func(ctx context.Context, filter model.VideoFilter) ([]model.Video, int64, error) {
			return []model.Video{}, 0, nil
		},
	}
	tagRepo := &mock.TagRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	result, total, err := svc.List(context.Background(), model.VideoFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if result == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestVideoService_Delete_MinIOFailure(t *testing.T) {
	video := &model.Video{
		ID:             "vid-1",
		MinIOObjectKey: "videos/vid-1/test.mp4",
		ThumbnailKey:   "thumbnails/vid-1.jpg",
	}

	dbDeleted := false
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return video, nil
		},
		DeleteFunc: func(ctx context.Context, id string) error {
			dbDeleted = true
			return nil
		},
	}
	tagRepo := &mock.TagRepository{}
	minioSvc := &mock.MinIOClient{
		DeleteVideoFunc: func(ctx context.Context, objectKey string) error {
			return errors.New("minio connection refused")
		},
		DeleteThumbnailFunc: func(ctx context.Context, objectKey string) error {
			return errors.New("minio connection refused")
		},
	}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	err := svc.Delete(context.Background(), "vid-1")

	// Delete should succeed even if MinIO fails
	if err != nil {
		t.Fatalf("expected nil error (MinIO failure is best-effort), got %v", err)
	}
	if !dbDeleted {
		t.Error("expected DB record to be deleted")
	}
}

func TestVideoService_Update_Success(t *testing.T) {
	updatedVideo := &model.Video{
		ID:    "vid-1",
		Title: "New Title",
	}

	var capturedInput model.UpdateVideoInput
	videoRepo := &mock.VideoRepository{
		UpdateFunc: func(ctx context.Context, id string, input model.UpdateVideoInput) error {
			capturedInput = input
			return nil
		},
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return updatedVideo, nil
		},
	}
	tagRepo := &mock.TagRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	input := model.UpdateVideoInput{Title: "New Title", Description: "New desc"}
	result, err := svc.Update(context.Background(), "vid-1", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedInput.Title != "New Title" || capturedInput.Description != "New desc" {
		t.Errorf("unexpected captured input: %+v", capturedInput)
	}
	if result.ID != "vid-1" {
		t.Errorf("expected video id vid-1, got %s", result.ID)
	}
}

func TestVideoService_Update_NotFound(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		UpdateFunc: func(ctx context.Context, id string, input model.UpdateVideoInput) error {
			return model.ErrNotFound
		},
	}
	tagRepo := &mock.TagRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	_, err := svc.Update(context.Background(), "nonexistent", model.UpdateVideoInput{Title: "x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestVideoService_Delete_NotFound(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return nil, model.ErrNotFound
		},
	}
	tagRepo := &mock.TagRepository{}
	minioSvc := &mock.MinIOClient{}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	err := svc.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestVideoService_GetByID_LocalPathMode(t *testing.T) {
	sourceID := "src-1"
	filePath := "movies/test.mp4"
	video := &model.Video{
		ID:             "vid-1",
		Title:          "Test Video",
		MinIOObjectKey: "",
		ThumbnailKey:   "thumbnails/vid-1.jpg",
		SourceID:       &sourceID,
		FilePath:       &filePath,
	}

	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return video, nil
		},
	}
	tagRepo := &mock.TagRepository{
		GetByVideoIDFunc: func(ctx context.Context, videoID string) ([]model.Tag, error) {
			return []model.Tag{}, nil
		},
	}

	presignedURLCalled := false
	minioSvc := &mock.MinIOClient{
		GeneratePresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			presignedURLCalled = true
			return "https://minio/stream", nil
		},
		GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
			return "https://minio/thumb", nil
		},
	}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	detail, err := svc.GetByID(context.Background(), "vid-1", 2*time.Hour, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if presignedURLCalled {
		t.Error("expected GeneratePresignedURL NOT to be called for local path video")
	}
	if detail.StreamURL != "" {
		t.Errorf("expected empty stream url for local path video, got %s", detail.StreamURL)
	}
	if detail.ThumbnailURL != "https://minio/thumb" {
		t.Errorf("expected thumbnail url, got %s", detail.ThumbnailURL)
	}
	if detail.SourceID == nil || *detail.SourceID != "src-1" {
		t.Errorf("expected source_id src-1, got %v", detail.SourceID)
	}
}

func TestVideoService_Delete_LocalPathMode(t *testing.T) {
	sourceID := "src-1"
	filePath := "movies/test.mp4"
	video := &model.Video{
		ID:             "vid-1",
		MinIOObjectKey: "",
		ThumbnailKey:   "thumbnails/vid-1.jpg",
		SourceID:       &sourceID,
		FilePath:       &filePath,
	}

	dbDeleted := false
	deleteVideoCalled := false
	deleteThumbnailCalled := false

	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			return video, nil
		},
		DeleteFunc: func(ctx context.Context, id string) error {
			dbDeleted = true
			return nil
		},
	}
	tagRepo := &mock.TagRepository{}
	minioSvc := &mock.MinIOClient{
		DeleteVideoFunc: func(ctx context.Context, objectKey string) error {
			deleteVideoCalled = true
			return nil
		},
		DeleteThumbnailFunc: func(ctx context.Context, objectKey string) error {
			deleteThumbnailCalled = true
			return nil
		},
	}

	svc := NewVideoService(videoRepo, tagRepo, minioSvc)
	err := svc.Delete(context.Background(), "vid-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !dbDeleted {
		t.Error("expected DB record to be deleted")
	}
	if deleteVideoCalled {
		t.Error("expected MinIO DeleteVideo NOT to be called for local path video")
	}
	if !deleteThumbnailCalled {
		t.Error("expected MinIO DeleteThumbnail to be called")
	}
}
