package mock

import (
	"context"
	"time"
)

type MinIOClient struct {
	UploadVideoFunc                func(ctx context.Context, objectKey, filePath string) error
	UploadThumbnailFunc            func(ctx context.Context, objectKey, filePath string) error
	GeneratePresignedURLFunc       func(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	GenerateThumbnailPresignedURLFunc func(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	DeleteVideoFunc                func(ctx context.Context, objectKey string) error
	DeleteThumbnailFunc            func(ctx context.Context, objectKey string) error
}

func (m *MinIOClient) UploadVideo(ctx context.Context, objectKey, filePath string) error {
	return m.UploadVideoFunc(ctx, objectKey, filePath)
}

func (m *MinIOClient) UploadThumbnail(ctx context.Context, objectKey, filePath string) error {
	return m.UploadThumbnailFunc(ctx, objectKey, filePath)
}

func (m *MinIOClient) GeneratePresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	return m.GeneratePresignedURLFunc(ctx, objectKey, expiry)
}

func (m *MinIOClient) GenerateThumbnailPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	return m.GenerateThumbnailPresignedURLFunc(ctx, objectKey, expiry)
}

func (m *MinIOClient) DeleteVideo(ctx context.Context, objectKey string) error {
	return m.DeleteVideoFunc(ctx, objectKey)
}

func (m *MinIOClient) DeleteThumbnail(ctx context.Context, objectKey string) error {
	return m.DeleteThumbnailFunc(ctx, objectKey)
}
