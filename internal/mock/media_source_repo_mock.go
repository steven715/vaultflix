package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

type MediaSourceRepository struct {
	ListFunc     func(ctx context.Context) ([]model.MediaSource, error)
	FindByIDFunc func(ctx context.Context, id string) (*model.MediaSource, error)
	CreateFunc   func(ctx context.Context, source *model.MediaSource) error
	UpdateFunc   func(ctx context.Context, source *model.MediaSource) error
	DeleteFunc   func(ctx context.Context, id string) error
}

func (m *MediaSourceRepository) List(ctx context.Context) ([]model.MediaSource, error) {
	if m.ListFunc == nil {
		return nil, fmt.Errorf("mock: ListFunc not set")
	}
	return m.ListFunc(ctx)
}

func (m *MediaSourceRepository) FindByID(ctx context.Context, id string) (*model.MediaSource, error) {
	if m.FindByIDFunc == nil {
		return nil, fmt.Errorf("mock: FindByIDFunc not set")
	}
	return m.FindByIDFunc(ctx, id)
}

func (m *MediaSourceRepository) Create(ctx context.Context, source *model.MediaSource) error {
	if m.CreateFunc == nil {
		return fmt.Errorf("mock: CreateFunc not set")
	}
	return m.CreateFunc(ctx, source)
}

func (m *MediaSourceRepository) Update(ctx context.Context, source *model.MediaSource) error {
	if m.UpdateFunc == nil {
		return fmt.Errorf("mock: UpdateFunc not set")
	}
	return m.UpdateFunc(ctx, source)
}

func (m *MediaSourceRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc == nil {
		return fmt.Errorf("mock: DeleteFunc not set")
	}
	return m.DeleteFunc(ctx, id)
}
