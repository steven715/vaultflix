package mock

import (
	"context"

	"github.com/steven/vaultflix/internal/model"
)

type VideoRepository struct {
	ExistsByFilenameAndSizeFunc func(ctx context.Context, filename string, sizeBytes int64) (bool, error)
	CreateFunc                  func(ctx context.Context, video *model.Video) error
	ListFunc                    func(ctx context.Context, filter model.VideoFilter) ([]model.Video, int64, error)
	GetByIDFunc                 func(ctx context.Context, id string) (*model.Video, error)
	UpdateFunc                  func(ctx context.Context, id string, input model.UpdateVideoInput) error
	DeleteFunc                  func(ctx context.Context, id string) error
}

func (m *VideoRepository) ExistsByFilenameAndSize(ctx context.Context, filename string, sizeBytes int64) (bool, error) {
	return m.ExistsByFilenameAndSizeFunc(ctx, filename, sizeBytes)
}

func (m *VideoRepository) Create(ctx context.Context, video *model.Video) error {
	return m.CreateFunc(ctx, video)
}

func (m *VideoRepository) List(ctx context.Context, filter model.VideoFilter) ([]model.Video, int64, error) {
	return m.ListFunc(ctx, filter)
}

func (m *VideoRepository) GetByID(ctx context.Context, id string) (*model.Video, error) {
	return m.GetByIDFunc(ctx, id)
}

func (m *VideoRepository) Update(ctx context.Context, id string, input model.UpdateVideoInput) error {
	return m.UpdateFunc(ctx, id, input)
}

func (m *VideoRepository) Delete(ctx context.Context, id string) error {
	return m.DeleteFunc(ctx, id)
}
