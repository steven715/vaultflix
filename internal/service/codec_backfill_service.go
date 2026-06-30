package service

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// codecProbeFunc returns (videoCodec, audioCodec, error).
type codecProbeFunc func(ctx context.Context, absPath string) (string, string, error)

// codecVideoRepo is the subset of VideoRepository needed by CodecBackfillService.
type codecVideoRepo interface {
	ListMissingCodecs(ctx context.Context, limit int) ([]model.Video, error)
	UpdateCodecs(ctx context.Context, id, videoCodec, audioCodec string) error
}

// codecSourceRepo is the subset of MediaSourceRepository needed by CodecBackfillService.
type codecSourceRepo interface {
	FindByID(ctx context.Context, id string) (*model.MediaSource, error)
}

// CodecBackfillService backfills missing codecs for existing videos.
type CodecBackfillService struct {
	videoRepo  codecVideoRepo
	sourceRepo codecSourceRepo
	probe      codecProbeFunc
}

// NewCodecBackfillService creates a CodecBackfillService with the real ffprobe probe.
func NewCodecBackfillService(v codecVideoRepo, s codecSourceRepo) *CodecBackfillService {
	return &CodecBackfillService{videoRepo: v, sourceRepo: s, probe: probeCodecs}
}

// Run backfills all videos missing codecs, returning processed and failed counts.
func (s *CodecBackfillService) Run(ctx context.Context) (int, int, error) {
	videos, err := s.videoRepo.ListMissingCodecs(ctx, 10000)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list videos missing codecs: %w", err)
	}

	processed, failed := 0, 0
	for _, v := range videos {
		source, err := s.sourceRepo.FindByID(ctx, *v.SourceID)
		if err != nil {
			slog.Warn("backfill: source lookup failed", "video_id", v.ID, "error", err)
			failed++
			continue
		}
		abs := filepath.Clean(filepath.Join(source.MountPath, *v.FilePath))
		vc, ac, err := s.probe(ctx, abs)
		if err != nil {
			slog.Warn("backfill: probe failed", "video_id", v.ID, "error", err)
			failed++
			continue
		}
		if err := s.videoRepo.UpdateCodecs(ctx, v.ID, vc, ac); err != nil {
			slog.Warn("backfill: update failed", "video_id", v.ID, "error", err)
			failed++
			continue
		}
		processed++
	}
	slog.Info("codec backfill complete", "processed", processed, "failed", failed)
	return processed, failed, nil
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
