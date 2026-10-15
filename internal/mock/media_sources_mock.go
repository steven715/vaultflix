package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

// MediaSources is a hand-written fake of the Media Source lookup
// (MediaSourceService.GetByID) that knows only the sources it was given.
type MediaSources []*model.MediaSource

// GetByID returns the source with id, or model.ErrNotFound (wrapped).
func (m MediaSources) GetByID(ctx context.Context, id string) (*model.MediaSource, error) {
	for _, s := range m {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, fmt.Errorf("media source %s: %w", id, model.ErrNotFound)
}
