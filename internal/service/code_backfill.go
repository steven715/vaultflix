package service

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/scraper/avid"
)

// codeSeedRepo is the subset of VideoRepository CodeBackfill needs.
type codeSeedRepo interface {
	// ListByEnrichmentStatus returns the Videos with status, or a wrapped error.
	ListByEnrichmentStatus(ctx context.Context, status string) ([]model.Video, error)
	// SeedCode returns model.ErrNotFound when the Video no longer exists.
	SeedCode(ctx context.Context, id, code, status string) error
}

// CodeBackfill is the Code kind of Backfill: for every Video Enrichment has
// never looked at, it parses a Code from the file name and marks the Video
// pending (Code found) or no_code. BackfillRunner drives it.
type CodeBackfill struct {
	videoRepo codeSeedRepo
}

// NewCodeBackfill creates a CodeBackfill.
func NewCodeBackfill(videoRepo codeSeedRepo) *CodeBackfill {
	return &CodeBackfill{videoRepo: videoRepo}
}

// List returns the Videos whose enrichment_status is still none.
func (b *CodeBackfill) List(ctx context.Context) ([]model.Video, error) {
	videos, err := b.videoRepo.ListByEnrichmentStatus(ctx, model.EnrichmentNone)
	if err != nil {
		return nil, fmt.Errorf("failed to list videos with status none: %w", err)
	}
	return videos, nil
}

// ProcessOne seeds v's Code and enrichment status. A file name without a Code
// is a successful outcome (status no_code), not a failure.
func (b *CodeBackfill) ProcessOne(ctx context.Context, v *model.Video) error {
	code, status := codeFromFilename(v.OriginalFilename)
	if err := b.videoRepo.SeedCode(ctx, v.ID, code, status); err != nil {
		return fmt.Errorf("failed to seed code of video %s: %w", v.ID, err)
	}
	return nil
}

// codeFromFilename parses a Code from a file name. A Code makes the Video
// pending for Enrichment; without one it is no_code. Import and CodeBackfill
// share this rule.
func codeFromFilename(filename string) (code, status string) {
	if code, ok := avid.ExtractCode(filename); ok {
		return code, model.EnrichmentPending
	}
	return "", model.EnrichmentNoCode
}
