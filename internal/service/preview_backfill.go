package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// PreviewBackfill is the Preview kind of Backfill: it cuts and uploads a
// Preview for every Video that has none (imported before Previews existed, or
// whose earlier attempt failed). BackfillRunner drives it.
type PreviewBackfill struct {
	videoRepo repository.VideoRepository
	files     mediaFileResolver
	minioSvc  MinIOClient

	// generatePreview is the preview-clip producer; overridable so tests can
	// avoid shelling out to ffmpeg.
	generatePreview func(ctx context.Context, srcPath string, durationSeconds int) (string, error)
}

// NewPreviewBackfill creates a PreviewBackfill using the real ffmpeg clipper.
func NewPreviewBackfill(videoRepo repository.VideoRepository, files mediaFileResolver, minioSvc MinIOClient) *PreviewBackfill {
	return &PreviewBackfill{videoRepo: videoRepo, files: files, minioSvc: minioSvc, generatePreview: generatePreviewClip}
}

// List returns every Video without a Preview.
func (p *PreviewBackfill) List(ctx context.Context) ([]model.Video, error) {
	videos, err := p.videoRepo.ListMissingPreviews(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list videos missing previews: %w", err)
	}
	return videos, nil
}

// ProcessOne cuts, uploads and records the Preview of v. Returns
// model.ErrMediaSourceDisabled (wrapped) when v's Media Source is disabled.
func (p *PreviewBackfill) ProcessOne(ctx context.Context, v *model.Video) error {
	if v.SourceID == nil || v.FilePath == nil {
		return errors.New("video has no source/file_path; legacy MinIO-stored videos cannot be backfilled")
	}

	absPath, err := p.files.ResolveFile(ctx, *v.SourceID, *v.FilePath)
	if err != nil {
		return fmt.Errorf("resolve preview source file: %w", err)
	}

	previewPath, err := p.generatePreview(ctx, absPath, v.DurationSeconds)
	if err != nil {
		return fmt.Errorf("generate preview clip: %w", err)
	}
	defer os.Remove(previewPath)

	objectKey := fmt.Sprintf("previews/%s.mp4", v.ID)
	if err := p.minioSvc.UploadPreview(ctx, objectKey, previewPath); err != nil {
		return fmt.Errorf("upload preview: %w", err)
	}

	if err := p.videoRepo.UpdatePreviewKey(ctx, v.ID, objectKey); err != nil {
		return fmt.Errorf("update preview key: %w", err)
	}
	return nil
}
