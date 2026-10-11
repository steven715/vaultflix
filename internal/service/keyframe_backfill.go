package service

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

// keyframeVideoRepo 是 KeyframeBackfill 所需的 video 查詢子集。
type keyframeVideoRepo interface {
	// ListKeyframeCandidates returns Videos with known codecs and no Keyframe
	// Index, or a wrapped repository error.
	ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error)
}

// keyframeIndexer builds a Video's Keyframe Index when its Play Mode needs
// one (implemented by *KeyframeService).
type keyframeIndexer interface {
	// EnsureIndex probes absPath and stores v's Keyframe Index if v plays by
	// remux, and does nothing otherwise. Returns a wrapped probe/store error.
	EnsureIndex(ctx context.Context, v *model.Video, absPath string) error
}

// KeyframeBackfill is the Keyframe Index kind of Backfill: it probes every
// remux Video that has no Keyframe Index yet. BackfillRunner drives it.
type KeyframeBackfill struct {
	keyframes keyframeIndexer
	videoRepo keyframeVideoRepo
	files     mediaFileResolver
}

// NewKeyframeBackfill creates a KeyframeBackfill that stores through keyframes.
func NewKeyframeBackfill(keyframes keyframeIndexer, videoRepo keyframeVideoRepo, files mediaFileResolver) *KeyframeBackfill {
	return &KeyframeBackfill{keyframes: keyframes, videoRepo: videoRepo, files: files}
}

// List returns the remux Videos without a Keyframe Index; other Play Modes
// never use one, so they are not part of the job at all.
func (b *KeyframeBackfill) List(ctx context.Context) ([]model.Video, error) {
	candidates, err := b.videoRepo.ListKeyframeCandidates(ctx, 10000)
	if err != nil {
		return nil, fmt.Errorf("failed to list keyframe candidates: %w", err)
	}
	remux := make([]model.Video, 0, len(candidates))
	for _, v := range candidates {
		if usesKeyframeIndex(&v) {
			remux = append(remux, v)
		}
	}
	return remux, nil
}

// ProcessOne probes v and stores its Keyframe Index. Returns
// model.ErrMediaSourceDisabled (wrapped) when v's Media Source is disabled.
func (b *KeyframeBackfill) ProcessOne(ctx context.Context, v *model.Video) error {
	abs, err := resolveVideoFile(ctx, b.files, v)
	if err != nil {
		return err
	}
	if err := b.keyframes.EnsureIndex(ctx, v, abs); err != nil {
		return fmt.Errorf("keyframe backfill: %w", err)
	}
	return nil
}
