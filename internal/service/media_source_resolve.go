package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// mediaFileResolver turns a Video's (source_id, file_path) into a validated
// on-disk path. MediaSourceService is the production implementation; every
// service that hands a Video file to ffmpeg/ffprobe or a byte server depends
// on this instead of joining mount_path itself.
type mediaFileResolver interface {
	// ResolveFile follows MediaSourceService.ResolveFile's error contract.
	ResolveFile(ctx context.Context, sourceID, filePath string) (string, error)
}

// ResolveFile returns the absolute on-disk path of filePath inside the Media
// Source sourceID. It is the only place that turns (source_id, file_path) into
// a path on disk: callers never join mount_path themselves.
//
// Returns model.ErrNotFound (wrapped) if the Media Source does not exist;
// model.ErrMediaSourceDisabled if it is disabled;
// model.ErrPathNotAllowed if the result escapes the Media Source mount or the
// allowed mount prefix;
// model.ErrPathNotExist (wrapped) if the file is absent from disk;
// a wrapped error otherwise.
func (s *MediaSourceService) ResolveFile(ctx context.Context, sourceID, filePath string) (string, error) {
	source, err := s.repo.FindByID(ctx, sourceID)
	if err != nil {
		return "", fmt.Errorf("failed to get media source %s: %w", sourceID, err)
	}
	if !source.Enabled {
		return "", model.ErrMediaSourceDisabled
	}
	abs := filepath.Join(source.MountPath, filePath)
	// 兩層檢查：必須落在該 Media Source 的 mount 內，且 mount 本身必須在
	// 注入的 mountPrefix 內（防 DB 中的 mount_path 本身有問題）。
	if !isWithin(abs, source.MountPath) || !isWithin(abs, s.mountPrefix) {
		return "", model.ErrPathNotAllowed
	}
	if _, err := os.Stat(abs); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%s: %w", abs, model.ErrPathNotExist)
		}
		return "", fmt.Errorf("failed to stat %s: %w", abs, err)
	}
	return abs, nil
}

// MountPrefix returns the injected root every Media Source mount lives under.
// In production it equals AllowedMountPrefix, which nginx aliases for
// X-Accel offload.
func (s *MediaSourceService) MountPrefix() string {
	return s.mountPrefix
}

// isWithin reports whether path equals root or lies strictly beneath it.
// Comparing against root+separator (not a bare prefix) prevents sibling-prefix
// collisions such as /mnt/host/D-extra slipping past a /mnt/host/D guard.
func isWithin(path, root string) bool {
	path, root = filepath.Clean(path), filepath.Clean(root)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}
