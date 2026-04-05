package mock

import (
	"context"
	"fmt"

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
	if m.ExistsByFilenameAndSizeFunc == nil {
		return false, fmt.Errorf("mock: ExistsByFilenameAndSizeFunc not set")
	}
	return m.ExistsByFilenameAndSizeFunc(ctx, filename, sizeBytes)
}

func (m *VideoRepository) Create(ctx context.Context, video *model.Video) error {
	if m.CreateFunc == nil {
		return fmt.Errorf("mock: CreateFunc not set")
	}
	return m.CreateFunc(ctx, video)
}

func (m *VideoRepository) List(ctx context.Context, filter model.VideoFilter) ([]model.Video, int64, error) {
	if m.ListFunc == nil {
		return nil, 0, fmt.Errorf("mock: ListFunc not set")
	}
	return m.ListFunc(ctx, filter)
}

func (m *VideoRepository) GetByID(ctx context.Context, id string) (*model.Video, error) {
	if m.GetByIDFunc == nil {
		return nil, fmt.Errorf("mock: GetByIDFunc not set")
	}
	return m.GetByIDFunc(ctx, id)
}

func (m *VideoRepository) Update(ctx context.Context, id string, input model.UpdateVideoInput) error {
	if m.UpdateFunc == nil {
		return fmt.Errorf("mock: UpdateFunc not set")
	}
	return m.UpdateFunc(ctx, id, input)
}

func (m *VideoRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc == nil {
		return fmt.Errorf("mock: DeleteFunc not set")
	}
	return m.DeleteFunc(ctx, id)
}
