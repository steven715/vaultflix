package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// AllowedMountPrefix is the production mount prefix for media sources.
const AllowedMountPrefix = "/mnt/host/"

type MediaSourceService struct {
	repo        repository.MediaSourceRepository
	mountPrefix string
}

func NewMediaSourceService(repo repository.MediaSourceRepository, mountPrefix string) *MediaSourceService {
	return &MediaSourceService{repo: repo, mountPrefix: mountPrefix}
}

// ValidateMountPath validates that path is within the allowed mount prefix,
// contains no path traversal components, exists on the filesystem, and is a directory.
func (s *MediaSourceService) ValidateMountPath(path string) error {
	cleaned := filepath.Clean(path)
	if !strings.HasPrefix(cleaned, strings.TrimSuffix(s.mountPrefix, string(filepath.Separator))) {
		return model.ErrPathNotAllowed
	}
	if cleaned != path {
		return fmt.Errorf("path contains invalid components: %w", model.ErrPathNotAllowed)
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return fmt.Errorf("%s: %w", cleaned, model.ErrPathNotExist)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %w", cleaned, model.ErrPathNotAllowed)
	}
	return nil
}

func (s *MediaSourceService) List(ctx context.Context) ([]model.MediaSource, error) {
	sources, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list media sources: %w", err)
	}
	return sources, nil
}

func (s *MediaSourceService) GetByID(ctx context.Context, id string) (*model.MediaSource, error) {
	source, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get media source %s: %w", id, err)
	}
	return source, nil
}

func (s *MediaSourceService) Create(ctx context.Context, source *model.MediaSource) error {
	if err := s.ValidateMountPath(source.MountPath); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, source); err != nil {
		return fmt.Errorf("failed to create media source: %w", err)
	}
	return nil
}

func (s *MediaSourceService) Update(ctx context.Context, id string, label string, enabled bool) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get media source %s: %w", id, err)
	}
	existing.Label = label
	existing.Enabled = enabled
	if err := s.repo.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update media source %s: %w", id, err)
	}
	return nil
}

func (s *MediaSourceService) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete media source %s: %w", id, err)
	}
	return nil
}
