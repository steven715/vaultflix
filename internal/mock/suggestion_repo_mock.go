package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

// SuggestionRepository is a hand-written mock for repository.SuggestionRepository.
// Set each Func field to override the corresponding method in tests.
type SuggestionRepository struct {
	CreateFunc       func(ctx context.Context, s *model.MetadataSuggestion) error
	GetByVideoIDFunc func(ctx context.Context, videoID string) ([]model.MetadataSuggestion, error)
	GetByIDFunc      func(ctx context.Context, id string) (*model.MetadataSuggestion, error)
	DeleteFunc       func(ctx context.Context, id string) error
}

func (m *SuggestionRepository) Create(ctx context.Context, s *model.MetadataSuggestion) error {
	if m.CreateFunc == nil {
		return fmt.Errorf("mock: CreateFunc not set")
	}
	return m.CreateFunc(ctx, s)
}

func (m *SuggestionRepository) GetByVideoID(ctx context.Context, videoID string) ([]model.MetadataSuggestion, error) {
	if m.GetByVideoIDFunc == nil {
		return nil, fmt.Errorf("mock: GetByVideoIDFunc not set")
	}
	return m.GetByVideoIDFunc(ctx, videoID)
}

func (m *SuggestionRepository) GetByID(ctx context.Context, id string) (*model.MetadataSuggestion, error) {
	if m.GetByIDFunc == nil {
		return nil, fmt.Errorf("mock: GetByIDFunc not set")
	}
	return m.GetByIDFunc(ctx, id)
}

func (m *SuggestionRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFunc == nil {
		return fmt.Errorf("mock: DeleteFunc not set")
	}
	return m.DeleteFunc(ctx, id)
}
