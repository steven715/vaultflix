package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/steven/vaultflix/internal/model"
)

// codecVideoRepo is the subset of VideoRepository needed by CodecBackfill.
type codecVideoRepo interface {
	ListMissingCodecs(ctx context.Context, limit int) ([]model.Video, error)
	UpdateCodecs(ctx context.Context, id, videoCodec, audioCodec string) error
}

// CodecBackfill is the codec kind of Backfill: it probes the video and
// audio codec of every Video whose Media Info lacks them, and builds the
// Keyframe Index of those that turn out to play by remux. BackfillRunner
// drives it.
type CodecBackfill struct {
	videoRepo codecVideoRepo
	files     mediaFileResolver
	media     mediaInfoProber
	keyframes keyframeIndexer
}

// mediaInfoProber reads Media Info (implemented by *MediaProcessor).
type mediaInfoProber interface {
	// ProbeMediaInfo returns a wrapped error when the file can't be probed.
	ProbeMediaInfo(ctx context.Context, path string) (model.MediaInfo, error)
}

// NewCodecBackfill creates a CodecBackfill.
func NewCodecBackfill(v codecVideoRepo, files mediaFileResolver, media mediaInfoProber, keyframes keyframeIndexer) *CodecBackfill {
	return &CodecBackfill{videoRepo: v, files: files, media: media, keyframes: keyframes}
}

// List returns the Videos whose codecs are unknown.
func (s *CodecBackfill) List(ctx context.Context) ([]model.Video, error) {
	videos, err := s.videoRepo.ListMissingCodecs(ctx, 10000)
	if err != nil {
		return nil, fmt.Errorf("failed to list videos missing codecs: %w", err)
	}
	return videos, nil
}

// ProcessOne probes and stores the codecs of v. Returns
// model.ErrMediaSourceDisabled (wrapped) when v's Media Source is disabled.
// A Keyframe Index that can't be built only logs: the codecs are stored, and
// the first play probes it again.
func (s *CodecBackfill) ProcessOne(ctx context.Context, v *model.Video) error {
	abs, err := resolveVideoFile(ctx, s.files, v)
	if err != nil {
		return err
	}
	info, err := s.media.ProbeMediaInfo(ctx, abs)
	if err != nil {
		return fmt.Errorf("failed to probe codecs of video %s: %w", v.ID, err)
	}
	if err := s.videoRepo.UpdateCodecs(ctx, v.ID, info.VideoCodec, info.AudioCodec); err != nil {
		return fmt.Errorf("failed to store codecs of video %s: %w", v.ID, err)
	}
	probed := *v
	probed.VideoCodec, probed.AudioCodec = info.VideoCodec, info.AudioCodec
	if err := s.keyframes.EnsureIndex(ctx, &probed, abs); err != nil {
		slog.Warn("keyframe index not built after codec backfill", "video_id", v.ID, "error", err)
	}
	return nil
}
