package service

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// PreviewBackfill is the Preview kind of Backfill: it cuts and uploads a
// Preview for every Video that has none (imported before Previews existed, or
// whose earlier attempt failed). BackfillRunner drives it.
type PreviewBackfill struct {
	videoRepo repository.VideoRepository
	files     mediaFileResolver
	previews  previewMaker
}

// previewMaker cuts and uploads a Preview (implemented by *MediaProcessor).
type previewMaker interface {
	// MakePreview returns the uploaded Preview key, or a wrapped error.
	MakePreview(ctx context.Context, f model.MediaFile) (string, error)
}

// NewPreviewBackfill creates a PreviewBackfill.
func NewPreviewBackfill(videoRepo repository.VideoRepository, files mediaFileResolver, previews previewMaker) *PreviewBackfill {
	return &PreviewBackfill{videoRepo: videoRepo, files: files, previews: previews}
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

	objectKey, err := p.previews.MakePreview(ctx, model.MediaFile{VideoID: v.ID, Path: absPath, DurationSeconds: v.DurationSeconds})
	if err != nil {
		return err
	}
	if err := p.videoRepo.UpdatePreviewKey(ctx, v.ID, objectKey); err != nil {
		return fmt.Errorf("failed to record preview key of video %s: %w", v.ID, err)
	}
	return nil
}
