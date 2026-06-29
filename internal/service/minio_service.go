package service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/minio/minio-go/v7"
)

const (
	defaultPresignedExpiry = 2 * time.Hour
	// presignCacheLeeway is subtracted from the presigned URL's lifetime when
	// caching, so a cached URL is never returned in the last few minutes
	// before its real expiry — that protects callers (browsers) from racing a
	// 403 right after consuming the URL.
	presignCacheLeeway = 5 * time.Minute
)

// MinIOClient defines the contract for object storage operations.
// GeneratePresignedURL, GenerateThumbnailPresignedURL and GeneratePreviewPresignedURL
// use defaultPresignedExpiry when expiry is 0.
// Delete methods return nil if the object does not exist in MinIO (idempotent).
type MinIOClient interface {
	UploadVideo(ctx context.Context, objectKey, filePath string) error
	UploadThumbnail(ctx context.Context, objectKey, filePath string) error
	UploadPreview(ctx context.Context, objectKey, filePath string) error
	UploadCover(ctx context.Context, objectKey, filePath string) error
	UploadActressAvatar(ctx context.Context, objectKey, filePath string) error
	GeneratePresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	GenerateThumbnailPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	GeneratePreviewPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	GenerateCoverPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	GenerateActressAvatarPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	DeleteVideo(ctx context.Context, objectKey string) error
	DeleteThumbnail(ctx context.Context, objectKey string) error
	DeletePreview(ctx context.Context, objectKey string) error
}

type minIOService struct {
	client          *minio.Client
	presignClient   *minio.Client
	videoBucket     string
	thumbnailBucket string
	previewBucket   string
	urlCache        URLCache
}

// NewMinIOService creates a MinIO service. If presignClient is non-nil, it is used
// for generating presigned URLs (e.g. when the public endpoint differs from the
// internal endpoint). Otherwise, client is used for everything. urlCache must
// be non-nil; pass NewInMemoryURLCache() for the default.
func NewMinIOService(client, presignClient *minio.Client, videoBucket, thumbnailBucket, previewBucket string, urlCache URLCache) MinIOClient {
	if presignClient == nil {
		presignClient = client
	}
	return &minIOService{
		client:          client,
		presignClient:   presignClient,
		videoBucket:     videoBucket,
		thumbnailBucket: thumbnailBucket,
		previewBucket:   previewBucket,
		urlCache:        urlCache,
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
	return s.presignCached(ctx, s.thumbnailBucket, "thumb", "thumbnail", objectKey, expiry)
}

func (s *minIOService) UploadPreview(ctx context.Context, objectKey, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open preview file %s: %w", filePath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat preview file %s: %w", filePath, err)
	}

	_, err = s.client.PutObject(ctx, s.previewBucket, objectKey, file, stat.Size(), minio.PutObjectOptions{
		ContentType: "video/mp4",
	})
	if err != nil {
		return fmt.Errorf("failed to upload preview to minio %s: %w", objectKey, err)
	}

	return nil
}

func (s *minIOService) GeneratePreviewPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	return s.presignCached(ctx, s.previewBucket, "preview", "preview", objectKey, expiry)
}

// uploadImage opens filePath, stats it, and PutObjects it into thumbnailBucket
// as image/jpeg. label is used only in error messages.
func (s *minIOService) uploadImage(ctx context.Context, objectKey, filePath, label string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open %s file %s: %w", label, filePath, err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat %s file %s: %w", label, filePath, err)
	}

	_, err = s.client.PutObject(ctx, s.thumbnailBucket, objectKey, file, stat.Size(), minio.PutObjectOptions{
		ContentType: "image/jpeg",
	})
	if err != nil {
		return fmt.Errorf("failed to upload %s to minio %s: %w", label, objectKey, err)
	}

	return nil
}

func (s *minIOService) UploadCover(ctx context.Context, objectKey, filePath string) error {
	return s.uploadImage(ctx, objectKey, filePath, "cover")
}

func (s *minIOService) UploadActressAvatar(ctx context.Context, objectKey, filePath string) error {
	return s.uploadImage(ctx, objectKey, filePath, "actress avatar")
}

func (s *minIOService) GenerateCoverPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	return s.presignCached(ctx, s.thumbnailBucket, "cover", "cover", objectKey, expiry)
}

func (s *minIOService) GenerateActressAvatarPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	return s.presignCached(ctx, s.thumbnailBucket, "avatar", "avatar", objectKey, expiry)
}

// presignCached produces a presigned GET URL and memoises it in urlCache. The
// cache key is "<cachePrefix>:<objectKey>"; cache TTL is the presigned expiry
// minus presignCacheLeeway so a cached URL never returns within the leeway
// window before its real expiry. errLabel appears in the wrapped error for
// observability.
func (s *minIOService) presignCached(ctx context.Context, bucket, cachePrefix, errLabel, objectKey string, expiry time.Duration) (string, error) {
	if expiry == 0 {
		expiry = defaultPresignedExpiry
	}

	cacheKey := cachePrefix + ":" + objectKey
	if cached, ok := s.urlCache.Get(cacheKey); ok {
		return cached, nil
	}

	presignedURL, err := s.presignClient.PresignedGetObject(ctx, bucket, objectKey, expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned url for %s %s: %w", errLabel, objectKey, err)
	}
	relative := toRelativeMinIOURL(presignedURL)

	if ttl := expiry - presignCacheLeeway; ttl > 0 {
		s.urlCache.Set(cacheKey, relative, ttl)
	}
	return relative, nil
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

func (s *minIOService) DeletePreview(ctx context.Context, objectKey string) error {
	return s.deleteObject(ctx, s.previewBucket, objectKey)
}
