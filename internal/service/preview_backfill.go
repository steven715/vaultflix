package service

import (
	"context"
	"fmt"
	"log/slog"
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
	absPath, err := resolveVideoFile(ctx, p.files, v)
	if err != nil {
		return err
	}

	previewPath, err := p.generatePreview(ctx, absPath, v.DurationSeconds)
	if err != nil {
		return fmt.Errorf("failed to cut preview of video %s: %w", v.ID, err)
	}
	defer func() {
		if err := os.Remove(previewPath); err != nil {
			slog.Warn("failed to remove temp preview clip", "video_id", v.ID, "path", previewPath, "error", err)
		}
	}()

	objectKey := fmt.Sprintf("previews/%s.mp4", v.ID)
	if err := p.minioSvc.UploadPreview(ctx, objectKey, previewPath); err != nil {
		return fmt.Errorf("failed to upload preview of video %s: %w", v.ID, err)
	}
	if err := p.videoRepo.UpdatePreviewKey(ctx, v.ID, objectKey); err != nil {
		return fmt.Errorf("failed to record preview key of video %s: %w", v.ID, err)
	}
	return nil
}
