package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// ResolvePlayback returns the validated on-disk path of a Video and its Play
// Mode, which is everything a streaming module needs to serve it.
//
// Returns model.ErrNotFound (wrapped) if the Video is missing or has no Media
// Source (legacy MinIO-stored Videos); otherwise follows
// MediaSourceService.ResolveFile's error contract (ErrMediaSourceDisabled,
// ErrPathNotAllowed, ErrPathNotExist).
func (s *VideoService) ResolvePlayback(ctx context.Context, videoID string) (string, model.PlayMode, error) {
	video, err := s.videoRepo.GetByID(ctx, videoID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get video %s: %w", videoID, err)
	}
	if video.SourceID == nil || video.FilePath == nil {
		return "", "", fmt.Errorf("video %s has no media source: %w", videoID, model.ErrNotFound)
	}
	path, err := s.files.ResolveFile(ctx, *video.SourceID, *video.FilePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve file of video %s: %w", videoID, err)
	}
	container := strings.TrimPrefix(filepath.Ext(video.OriginalFilename), ".")
	return path, ClassifyPlayMode(container, video.VideoCodec, video.AudioCodec), nil
}
