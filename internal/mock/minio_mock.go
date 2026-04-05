package mock

import (
	"context"
	"fmt"
	"time"
)

type MinIOClient struct {
	UploadVideoFunc                   func(ctx context.Context, objectKey, filePath string) error
	UploadThumbnailFunc               func(ctx context.Context, objectKey, filePath string) error
	GeneratePresignedURLFunc          func(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	GenerateThumbnailPresignedURLFunc func(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	DeleteVideoFunc                   func(ctx context.Context, objectKey string) error
	DeleteThumbnailFunc               func(ctx context.Context, objectKey string) error
}

func (m *MinIOClient) UploadVideo(ctx context.Context, objectKey, filePath string) error {
	if m.UploadVideoFunc == nil {
		return fmt.Errorf("mock: UploadVideoFunc not set")
	}
	return m.UploadVideoFunc(ctx, objectKey, filePath)
}

func (m *MinIOClient) UploadThumbnail(ctx context.Context, objectKey, filePath string) error {
	if m.UploadThumbnailFunc == nil {
		return fmt.Errorf("mock: UploadThumbnailFunc not set")
	}
	return m.UploadThumbnailFunc(ctx, objectKey, filePath)
}

func (m *MinIOClient) GeneratePresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	if m.GeneratePresignedURLFunc == nil {
		return "", fmt.Errorf("mock: GeneratePresignedURLFunc not set")
	}
	return m.GeneratePresignedURLFunc(ctx, objectKey, expiry)
}

func (m *MinIOClient) GenerateThumbnailPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	if m.GenerateThumbnailPresignedURLFunc == nil {
		return "", fmt.Errorf("mock: GenerateThumbnailPresignedURLFunc not set")
	}
	return m.GenerateThumbnailPresignedURLFunc(ctx, objectKey, expiry)
}

func (m *MinIOClient) DeleteVideo(ctx context.Context, objectKey string) error {
	if m.DeleteVideoFunc == nil {
		return fmt.Errorf("mock: DeleteVideoFunc not set")
	}
	return m.DeleteVideoFunc(ctx, objectKey)
}

func (m *MinIOClient) DeleteThumbnail(ctx context.Context, objectKey string) error {
	if m.DeleteThumbnailFunc == nil {
		return fmt.Errorf("mock: DeleteThumbnailFunc not set")
	}
	return m.DeleteThumbnailFunc(ctx, objectKey)
}
