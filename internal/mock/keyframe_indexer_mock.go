package mock

import (
	"context"

	"github.com/steven/vaultflix/internal/model"
)

// KeyframeIndexer is a hand-written fake for KeyframeService.EnsureIndex that
// builds nothing, for tests that don't look at Keyframe Indexes.
type KeyframeIndexer struct{}

func (m *KeyframeIndexer) EnsureIndex(ctx context.Context, v *model.Video, absPath string) error {
	return nil
}
