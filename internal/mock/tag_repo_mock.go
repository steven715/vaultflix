package mock

import (
	"context"

	"github.com/steven/vaultflix/internal/model"
)

type TagRepository struct {
	ListFunc          func(ctx context.Context, category string) ([]model.TagWithCount, error)
	CreateFunc        func(ctx context.Context, tag *model.Tag) error
	GetByIDFunc       func(ctx context.Context, id int) (*model.Tag, error)
	GetByVideoIDFunc  func(ctx context.Context, videoID string) ([]model.Tag, error)
	GetByVideoIDsFunc func(ctx context.Context, videoIDs []string) (map[string][]model.Tag, error)
	AddVideoTagFunc   func(ctx context.Context, videoID string, tagID int) error
	RemoveVideoTagFunc func(ctx context.Context, videoID string, tagID int) error
}

func (m *TagRepository) List(ctx context.Context, category string) ([]model.TagWithCount, error) {
	return m.ListFunc(ctx, category)
}

func (m *TagRepository) Create(ctx context.Context, tag *model.Tag) error {
	return m.CreateFunc(ctx, tag)
}

func (m *TagRepository) GetByID(ctx context.Context, id int) (*model.Tag, error) {
	return m.GetByIDFunc(ctx, id)
}

func (m *TagRepository) GetByVideoID(ctx context.Context, videoID string) ([]model.Tag, error) {
	return m.GetByVideoIDFunc(ctx, videoID)
}

func (m *TagRepository) GetByVideoIDs(ctx context.Context, videoIDs []string) (map[string][]model.Tag, error) {
	return m.GetByVideoIDsFunc(ctx, videoIDs)
}

func (m *TagRepository) AddVideoTag(ctx context.Context, videoID string, tagID int) error {
	return m.AddVideoTagFunc(ctx, videoID, tagID)
}

func (m *TagRepository) RemoveVideoTag(ctx context.Context, videoID string, tagID int) error {
	return m.RemoveVideoTagFunc(ctx, videoID, tagID)
}
