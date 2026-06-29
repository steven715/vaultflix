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
	FindBySourceAndPathFunc     func(ctx context.Context, sourceID string, filePath string) (*model.Video, error)
	ListMissingPreviewsFunc     func(ctx context.Context) ([]model.Video, error)
	UpdatePreviewKeyFunc        func(ctx context.Context, id string, previewKey string) error
	UpdateMetadataFunc          func(ctx context.Context, id string, m model.VideoMetadataUpdate) error
	SetEnrichmentStatusFunc     func(ctx context.Context, id, status string) error
	ListByEnrichmentStatusFunc  func(ctx context.Context, status string) ([]model.Video, error)
	SeedCodeFunc                func(ctx context.Context, id, code, status string) error
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

func (m *VideoRepository) FindBySourceAndPath(ctx context.Context, sourceID string, filePath string) (*model.Video, error) {
	if m.FindBySourceAndPathFunc == nil {
		return nil, fmt.Errorf("mock: FindBySourceAndPathFunc not set")
	}
	return m.FindBySourceAndPathFunc(ctx, sourceID, filePath)
}

func (m *VideoRepository) ListMissingPreviews(ctx context.Context) ([]model.Video, error) {
	if m.ListMissingPreviewsFunc == nil {
		return nil, fmt.Errorf("mock: ListMissingPreviewsFunc not set")
	}
	return m.ListMissingPreviewsFunc(ctx)
}

func (m *VideoRepository) UpdatePreviewKey(ctx context.Context, id string, previewKey string) error {
	if m.UpdatePreviewKeyFunc == nil {
		return fmt.Errorf("mock: UpdatePreviewKeyFunc not set")
	}
	return m.UpdatePreviewKeyFunc(ctx, id, previewKey)
}

func (m *VideoRepository) UpdateMetadata(ctx context.Context, id string, upd model.VideoMetadataUpdate) error {
	if m.UpdateMetadataFunc == nil {
		return fmt.Errorf("mock: UpdateMetadataFunc not set")
	}
	return m.UpdateMetadataFunc(ctx, id, upd)
}

func (m *VideoRepository) SetEnrichmentStatus(ctx context.Context, id, status string) error {
	if m.SetEnrichmentStatusFunc == nil {
		return fmt.Errorf("mock: SetEnrichmentStatusFunc not set")
	}
	return m.SetEnrichmentStatusFunc(ctx, id, status)
}

func (m *VideoRepository) ListByEnrichmentStatus(ctx context.Context, status string) ([]model.Video, error) {
	if m.ListByEnrichmentStatusFunc == nil {
		return nil, fmt.Errorf("mock: ListByEnrichmentStatusFunc not set")
	}
	return m.ListByEnrichmentStatusFunc(ctx, status)
}

func (m *VideoRepository) SeedCode(ctx context.Context, id, code, status string) error {
	if m.SeedCodeFunc == nil {
		return fmt.Errorf("mock: SeedCodeFunc not set")
	}
	return m.SeedCodeFunc(ctx, id, code, status)
}
