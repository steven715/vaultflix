package mock

import (
	"context"

	"github.com/steven/vaultflix/internal/model"
)

// PlaybackResolver is a hand-written fake for VideoService.ResolvePlayback.
type PlaybackResolver struct {
	Path string
	Mode model.PlayMode
	Err  error
}

func (m *PlaybackResolver) ResolvePlayback(ctx context.Context, videoID string) (string, model.PlayMode, error) {
	return m.Path, m.Mode, m.Err
}
