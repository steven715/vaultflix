package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// TagService lists and creates Tags and tags Videos with them.
type TagService struct {
	tags   repository.TagRepository
	videos repository.VideoRepository
}

// NewTagService creates a TagService.
func NewTagService(tags repository.TagRepository, videos repository.VideoRepository) *TagService {
	return &TagService{tags: tags, videos: videos}
}

// List returns the Tags of category ("" for all) with their Video counts.
func (s *TagService) List(ctx context.Context, category string) ([]model.TagWithCount, error) {
	tags, err := s.tags.List(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags (category %q): %w", category, err)
	}
	return tags, nil
}

// Create stores tag, filling its ID. Returns model.ErrAlreadyExists (wrapped)
// when a Tag of that name exists.
func (s *TagService) Create(ctx context.Context, tag *model.Tag) error {
	if err := s.tags.Create(ctx, tag); err != nil {
		return fmt.Errorf("failed to create tag %q: %w", tag.Name, err)
	}
	return nil
}

// AddVideoTag tags Video videoID with tag tagID. created is false when the
// Video already had the Tag, which still counts as success. Returns a
// *model.NotFoundError naming the "video" or "tag" that is missing (it
// matches model.ErrNotFound).
func (s *TagService) AddVideoTag(ctx context.Context, videoID string, tagID int) (created bool, err error) {
	if _, err := s.videos.GetByID(ctx, videoID); err != nil {
		return false, missing(err, "video", fmt.Sprintf("video %s", videoID))
	}
	if _, err := s.tags.GetByID(ctx, tagID); err != nil {
		return false, missing(err, "tag", fmt.Sprintf("tag %d", tagID))
	}
	err = s.tags.AddVideoTag(ctx, videoID, tagID)
	switch {
	case errors.Is(err, model.ErrConflict):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("failed to tag video %s with tag %d: %w", videoID, tagID, err)
	}
	return true, nil
}

// RemoveVideoTag untags Video videoID. Returns model.ErrNotFound (wrapped)
// when the Video did not have the Tag.
func (s *TagService) RemoveVideoTag(ctx context.Context, videoID string, tagID int) error {
	if err := s.tags.RemoveVideoTag(ctx, videoID, tagID); err != nil {
		return fmt.Errorf("failed to untag video %s from tag %d: %w", videoID, tagID, err)
	}
	return nil
}

// missing turns a lookup's ErrNotFound into a NotFoundError naming what, and
// wraps any other error with ctx.
func missing(err error, what, ctx string) error {
	if errors.Is(err, model.ErrNotFound) {
		return fmt.Errorf("%s: %w", ctx, &model.NotFoundError{What: what})
	}
	return fmt.Errorf("failed to get %s: %w", ctx, err)
}
