package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/steven/vaultflix/internal/model"
)

// mediaDeriver is the MediaProcessor surface Import needs.
type mediaDeriver interface {
	// ProbeMediaInfo returns a wrapped error when the file can't be probed.
	ProbeMediaInfo(ctx context.Context, path string) (model.MediaInfo, error)
	// MakeThumbnail returns the uploaded Thumbnail key, or a wrapped error.
	MakeThumbnail(ctx context.Context, f model.MediaFile) (string, error)
	// MakePreview returns the uploaded Preview key, or a wrapped error.
	MakePreview(ctx context.Context, f model.MediaFile) (string, error)
}

// processOneFile registers one scanned file as a Video and builds its Keyframe
// Index if it plays by remux. Returns model.ErrJobItemSkipped when the file is
// already registered, or a wrapped error naming the file when it can't be
// registered. A Keyframe Index that can't be built only logs: the first play
// probes it again.
func (s *ImportService) processOneFile(ctx context.Context, source *model.MediaSource, filePath string) error {
	filename := filepath.Base(filePath)

	relPath, err := filepath.Rel(source.MountPath, filePath)
	if err != nil {
		return fmt.Errorf("failed to calculate relative path for %s: %w", filename, err)
	}
	relPath = filepath.ToSlash(relPath)

	_, err = s.videoRepo.FindBySourceAndPath(ctx, source.ID, relPath)
	if err == nil {
		slog.Info("video skipped, already imported", "file", filename, "source_id", source.ID, "file_path", relPath)
		return model.ErrJobItemSkipped
	}
	if !errors.Is(err, model.ErrNotFound) {
		return fmt.Errorf("failed to check duplicate for %s: %w", filename, err)
	}

	video, err := s.deriveVideo(ctx, filePath)
	if err != nil {
		return fmt.Errorf("failed to import %s: %w", filename, err)
	}
	video.SourceID, video.FilePath = &source.ID, &relPath
	if err := s.videoRepo.Create(ctx, video); err != nil {
		return fmt.Errorf("failed to save video record for %s: %w", filename, err)
	}

	if s.keyframes != nil {
		if err := s.keyframes.EnsureIndex(ctx, video, filePath); err != nil {
			slog.Warn("keyframe index not built at import", "video_id", video.ID, "file", filename, "error", err)
		}
	}
	slog.Info("video imported",
		"video_id", video.ID, "file", filename, "source_id", source.ID, "file_path", relPath,
		"duration", video.DurationSeconds, "resolution", video.Resolution, "size_bytes", video.FileSizeBytes,
	)
	return nil
}

// deriveVideo builds a new Video from the file: Media Info, Thumbnail, Preview
// and Code. A missing Preview only degrades the Video; every other step fails it.
func (s *ImportService) deriveVideo(ctx context.Context, filePath string) (*model.Video, error) {
	filename := filepath.Base(filePath)
	stat, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	info, err := s.media.ProbeMediaInfo(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("media info: %w", err)
	}

	file := model.MediaFile{VideoID: uuid.NewString(), Path: filePath, DurationSeconds: info.DurationSeconds}
	thumbnailKey, err := s.media.MakeThumbnail(ctx, file)
	if err != nil {
		return nil, fmt.Errorf("thumbnail: %w", err)
	}
	previewKey, err := s.media.MakePreview(ctx, file)
	if err != nil {
		slog.Warn("preview generation failed, importing without preview", "video_id", file.VideoID, "file", filename, "error", err)
		previewKey = ""
	}

	code, status := codeFromFilename(filename)
	return &model.Video{
		ID:               file.VideoID,
		Title:            strings.TrimSuffix(filename, filepath.Ext(filename)),
		ThumbnailKey:     thumbnailKey,
		PreviewKey:       previewKey,
		DurationSeconds:  info.DurationSeconds,
		Resolution:       info.Resolution,
		FileSizeBytes:    stat.Size(),
		MimeType:         info.MimeType,
		VideoCodec:       info.VideoCodec,
		AudioCodec:       info.AudioCodec,
		OriginalFilename: filename,
		Code:             code,
		EnrichmentStatus: status,
	}, nil
}
