package service

import (
	"context"
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
// model.ErrNotFound (wrapped) if the Video is missing or has neither a Media
// Source nor a MinIO object; otherwise follows MediaSourceService.ResolveFile's
// error contract (ErrMediaSourceDisabled, ErrPathNotAllowed, ErrPathNotExist).
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
	if src.Path, err = resolveVideoFile(ctx, s.files, video); err != nil {
		return nil, err
	}
	return src, nil
}

// resolveVideoFile turns v into its on-disk path: the one rule from a Video to
// its file, for playback and Backfills alike. Returns model.ErrNotFound
// (wrapped) for a Video without a Media Source; otherwise keeps ResolveFile's
// errors (e.g. model.ErrMediaSourceDisabled) visible to errors.Is.
func resolveVideoFile(ctx context.Context, files mediaFileResolver, v *model.Video) (string, error) {
	if v.SourceID == nil || v.FilePath == nil {
		return "", fmt.Errorf("video %s has no media source: %w", v.ID, model.ErrNotFound)
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
