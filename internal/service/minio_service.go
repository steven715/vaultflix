package service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
)

const defaultPresignedExpiry = 2 * time.Hour

// MinIOClient defines the contract for object storage operations.
// GeneratePresignedURL and GenerateThumbnailPresignedURL use defaultPresignedExpiry when expiry is 0.
// Delete methods return nil if the object does not exist in MinIO (idempotent).
type MinIOClient interface {
	UploadVideo(ctx context.Context, objectKey, filePath string) error
	UploadThumbnail(ctx context.Context, objectKey, filePath string) error
	GeneratePresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	GenerateThumbnailPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	DeleteVideo(ctx context.Context, objectKey string) error
	DeleteThumbnail(ctx context.Context, objectKey string) error
}

type minIOService struct {
	client          *minio.Client
	presignClient   *minio.Client
	videoBucket     string
	thumbnailBucket string
}

// NewMinIOService creates a MinIO service. If presignClient is non-nil, it is used
// for generating presigned URLs (e.g. when the public endpoint differs from the
// internal endpoint). Otherwise, client is used for everything.
func NewMinIOService(client, presignClient *minio.Client, videoBucket, thumbnailBucket string) MinIOClient {
	if presignClient == nil {
		presignClient = client
	}
	return &minIOService{
		client:          client,
		presignClient:   presignClient,
		videoBucket:     videoBucket,
		thumbnailBucket: thumbnailBucket,
	}
}

func (s *minIOService) UploadVideo(ctx context.Context, objectKey, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open video file %s: %w", filePath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat video file %s: %w", filePath, err)
	}

	_, err = s.client.PutObject(ctx, s.videoBucket, objectKey, file, stat.Size(), minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to upload video to minio %s: %w", objectKey, err)
	}

	return nil
}

func (s *minIOService) UploadThumbnail(ctx context.Context, objectKey, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open thumbnail file %s: %w", filePath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat thumbnail file %s: %w", filePath, err)
	}

	_, err = s.client.PutObject(ctx, s.thumbnailBucket, objectKey, file, stat.Size(), minio.PutObjectOptions{
		ContentType: "image/jpeg",
	})
	if err != nil {
		return fmt.Errorf("failed to upload thumbnail to minio %s: %w", objectKey, err)
	}

	return nil
}

func (s *minIOService) GeneratePresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	if expiry == 0 {
		expiry = defaultPresignedExpiry
	}

	presignedURL, err := s.presignClient.PresignedGetObject(ctx, s.videoBucket, objectKey, expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned url for %s: %w", objectKey, err)
	}

	return toRelativeMinIOURL(presignedURL), nil
}

func (s *minIOService) GenerateThumbnailPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	if expiry == 0 {
		expiry = defaultPresignedExpiry
	}

	presignedURL, err := s.presignClient.PresignedGetObject(ctx, s.thumbnailBucket, objectKey, expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned url for thumbnail %s: %w", objectKey, err)
	}

	return toRelativeMinIOURL(presignedURL), nil
}

// toRelativeMinIOURL converts an absolute MinIO presigned URL to a relative
// path through the nginx /minio/ proxy. This ensures URLs work regardless of
// the browser's origin (localhost, ngrok, etc.) while preserving the signature.
func toRelativeMinIOURL(u *url.URL) string {
	return "/minio" + u.RequestURI()
}

func (s *minIOService) deleteObject(ctx context.Context, bucket, objectKey string) error {
	err := s.client.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object %s from bucket %s: %w", objectKey, bucket, err)
	}

	return nil
}

func (s *minIOService) DeleteVideo(ctx context.Context, objectKey string) error {
	return s.deleteObject(ctx, s.videoBucket, objectKey)
}

func (s *minIOService) DeleteThumbnail(ctx context.Context, objectKey string) error {
	return s.deleteObject(ctx, s.thumbnailBucket, objectKey)
}
