package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// keyframeVideoRepo 是 KeyframeBackfill 所需的 video 查詢子集。
type keyframeVideoRepo interface {
	// ListKeyframeCandidates returns Videos with known codecs and no Keyframe
	// Index, or a wrapped repository error.
	ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error)
}

// KeyframeBackfill is the Keyframe Index kind of Backfill: it probes every
// remux Video that has no Keyframe Index yet. BackfillRunner drives it.
type KeyframeBackfill struct {
	keyframes *KeyframeService
	videoRepo keyframeVideoRepo
	files     mediaFileResolver
}

// NewKeyframeBackfill creates a KeyframeBackfill that stores through keyframes.
func NewKeyframeBackfill(keyframes *KeyframeService, videoRepo keyframeVideoRepo, files mediaFileResolver) *KeyframeBackfill {
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
		container := strings.TrimPrefix(filepath.Ext(v.OriginalFilename), ".")
		if ClassifyPlayMode(container, v.VideoCodec, v.AudioCodec) == model.PlayModeRemux {
			remux = append(remux, v)
		}
	}
	return remux, nil
}

// ProcessOne probes v and stores its Keyframe Index, bounded by probeTimeout.
// Returns model.ErrMediaSourceDisabled (wrapped) when v's Media Source is
// disabled.
func (b *KeyframeBackfill) ProcessOne(ctx context.Context, v *model.Video) error {
	// ListKeyframeCandidates guarantees source_id/file_path are non-NULL.
	abs, err := b.files.ResolveFile(ctx, *v.SourceID, *v.FilePath)
	if err != nil {
		return fmt.Errorf("failed to resolve file of video %s: %w", v.ID, err)
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	if err := b.keyframes.probeAndStore(ctx, v.ID, abs); err != nil {
		return fmt.Errorf("failed to build keyframe index of video %s: %w", v.ID, err)
	}
	return nil
}
