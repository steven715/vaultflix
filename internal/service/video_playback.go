package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// ResolvePlayback returns where a Video's bytes are and its Play Mode: what
// every streaming path (direct Stream, HLS) needs, without the detail page's
// Tags or image URLs.
//
// A legacy MinIO-stored Video gives its LegacyObjectKey. Returns
// model.ErrNotFound (wrapped) only when the Video itself is missing; otherwise
// follows MediaSourceService.ResolveFile's error contract
// (ErrMediaSourceDisabled, ErrPathNotAllowed, ErrPathNotExist). Broken data —
// no Media Source and no MinIO object, or a Media Source row that is gone — is
// a plain error, so it is logged rather than read as "Video not found".
func (s *VideoService) ResolvePlayback(ctx context.Context, videoID string) (*model.PlaybackSource, error) {
	video, err := s.videoRepo.GetByID(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get video %s: %w", videoID, err)
	}
	src := &model.PlaybackSource{PlayMode: videoPlayMode(video), MimeType: video.MimeType}
	if video.SourceID == nil && video.MinIOObjectKey != "" {
		src.LegacyObjectKey = video.MinIOObjectKey
		return src, nil
	}
	src.Path, err = resolveVideoFile(ctx, s.files, video)
	if errors.Is(err, model.ErrNotFound) {
		// %v, not %w: the Video exists; its Media Source does not.
		return nil, fmt.Errorf("media source of video %s is missing: %v", videoID, err)
	}
	if err != nil {
		return nil, err
	}
	return src, nil
}

// resolveVideoFile turns v into its on-disk path: the one rule from a Video to
// its file, for playback and Backfills alike. Fails (never panics) for a
// Video without a Media Source; otherwise keeps ResolveFile's errors (e.g.
// model.ErrMediaSourceDisabled) visible to errors.Is.
func resolveVideoFile(ctx context.Context, files mediaFileResolver, v *model.Video) (string, error) {
	if v.SourceID == nil || v.FilePath == nil {
		return "", fmt.Errorf("video %s has no media source (legacy MinIO-stored video)", v.ID)
	}
	abs, err := files.ResolveFile(ctx, *v.SourceID, *v.FilePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file of video %s: %w", v.ID, err)
	}
	return abs, nil
}

// videoPlayMode classifies v by its container (file extension) and codecs.
func videoPlayMode(v *model.Video) model.PlayMode {
	container := strings.TrimPrefix(filepath.Ext(v.OriginalFilename), ".")
	return ClassifyPlayMode(container, v.VideoCodec, v.AudioCodec)
}
