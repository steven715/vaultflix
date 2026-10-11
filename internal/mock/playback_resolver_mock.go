package mock

import (
	"context"

	"github.com/steven/vaultflix/internal/model"
)

// PlaybackResolver is a hand-written fake for VideoService.ResolvePlayback.
type PlaybackResolver struct {
	Path            string
	LegacyObjectKey string
	Mode            model.PlayMode
	Err             error
}

func (m *PlaybackResolver) ResolvePlayback(ctx context.Context, videoID string) (*model.PlaybackSource, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return &model.PlaybackSource{Path: m.Path, LegacyObjectKey: m.LegacyObjectKey, PlayMode: m.Mode}, nil
}
