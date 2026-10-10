package service

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// codecProbeFunc returns (videoCodec, audioCodec, error).
type codecProbeFunc func(ctx context.Context, absPath string) (string, string, error)

// codecVideoRepo is the subset of VideoRepository needed by CodecBackfill.
type codecVideoRepo interface {
	ListMissingCodecs(ctx context.Context, limit int) ([]model.Video, error)
	UpdateCodecs(ctx context.Context, id, videoCodec, audioCodec string) error
}

// CodecBackfill is the codec kind of Backfill: it probes the video and
// audio codec of every Video whose Media Info lacks them. BackfillRunner drives it.
type CodecBackfill struct {
	videoRepo codecVideoRepo
	files     mediaFileResolver
	probe     codecProbeFunc
}

// NewCodecBackfill creates a CodecBackfill with the real ffprobe probe.
func NewCodecBackfill(v codecVideoRepo, files mediaFileResolver) *CodecBackfill {
	return &CodecBackfill{videoRepo: v, files: files, probe: probeCodecs}
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
func (s *CodecBackfill) ProcessOne(ctx context.Context, v *model.Video) error {
	abs, err := resolveVideoFile(ctx, s.files, v)
	if err != nil {
		return err
	}
	vc, ac, err := s.probe(ctx, abs)
	if err != nil {
		return fmt.Errorf("failed to probe codecs of video %s: %w", v.ID, err)
	}
	if err := s.videoRepo.UpdateCodecs(ctx, v.ID, vc, ac); err != nil {
		return fmt.Errorf("failed to store codecs of video %s: %w", v.ID, err)
	}
	return nil
}

func probeCodecs(ctx context.Context, absPath string) (string, string, error) {
	run := func(stream string) (string, error) {
		cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error",
			"-select_streams", stream, "-show_entries", "stream=codec_name",
			"-of", "default=nw=1:nk=1", absPath)
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("ffprobe %s failed: %w", stream, err)
		}
		return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), nil
	}
	vc, err := run("v:0")
	if err != nil {
		return "", "", err
	}
	ac, _ := run("a:0") // no audio track is acceptable
	return vc, ac, nil
}
