package mock

import (
	"context"
)

// HLSStreamer is a hand-written fake for service.HLSService as seen by the
// HLS handler. Err, when set, is returned by both calls.
type HLSStreamer struct {
	ManifestBytes []byte
	SegmentPath   string
	Err           error

	LastSegmentName string
}

func (m *HLSStreamer) Manifest(ctx context.Context, videoID string) ([]byte, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.ManifestBytes, nil
}

func (m *HLSStreamer) Segment(ctx context.Context, videoID, name string) (string, error) {
	m.LastSegmentName = name
	if m.Err != nil {
		return "", m.Err
	}
	return m.SegmentPath, nil
}
