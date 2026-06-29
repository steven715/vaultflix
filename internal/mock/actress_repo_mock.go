package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

// ActressRepository is a hand-written mock for repository.ActressRepository.
// Set each Func field to override the corresponding method in tests.
type ActressRepository struct {
	UpsertFunc          func(ctx context.Context, a *model.Actress) error
	AddVideoActressFunc func(ctx context.Context, videoID, actressID string) error
	GetByVideoIDFunc    func(ctx context.Context, videoID string) ([]model.Actress, error)
}

func (m *ActressRepository) Upsert(ctx context.Context, a *model.Actress) error {
	if m.UpsertFunc == nil {
		return fmt.Errorf("mock: UpsertFunc not set")
	}
	return m.UpsertFunc(ctx, a)
}

func (m *ActressRepository) AddVideoActress(ctx context.Context, videoID, actressID string) error {
	if m.AddVideoActressFunc == nil {
		return fmt.Errorf("mock: AddVideoActressFunc not set")
	}
	return m.AddVideoActressFunc(ctx, videoID, actressID)
}

func (m *ActressRepository) GetByVideoID(ctx context.Context, videoID string) ([]model.Actress, error) {
	if m.GetByVideoIDFunc == nil {
		return nil, fmt.Errorf("mock: GetByVideoIDFunc not set")
	}
	return m.GetByVideoIDFunc(ctx, videoID)
}
