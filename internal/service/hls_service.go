package service

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/streaming"
)

// playbackResolver resolves a Video to its on-disk path and Play Mode
// (implemented by *VideoService).
type playbackResolver interface {
	// ResolvePlayback follows VideoService.ResolvePlayback's error contract.
	ResolvePlayback(ctx context.Context, videoID string) (string, model.PlayMode, error)
}

// segmentIndex returns a Video's Segment Boundary table (implemented by
// *KeyframeService).
type segmentIndex interface {
	// Lookup returns model.ErrStreamPreparing (wrapped) while the Keyframe
	// Index is still being probed.
	Lookup(ctx context.Context, videoID, absPath string) ([]model.SegmentBoundary, error)
}

// segmentStore produces and caches HLS Segment files (implemented by
// *streaming.SegmentCache).
type segmentStore interface {
	EnsureSegment(ctx context.Context, videoID, inputPath string, idx int, seg model.SegmentBoundary) (string, error)
}

// HLSService serves remux Videos as VOD HLS: a manifest built from the
// Keyframe Index, and HLS Segments cut on demand. Callers only name a Video
// and, for a segment, the file name the manifest listed.
type HLSService struct {
	videos   playbackResolver
	index    segmentIndex
	segments segmentStore
}

// NewHLSService creates an HLSService.
func NewHLSService(videos playbackResolver, index segmentIndex, segments segmentStore) *HLSService {
	return &HLSService{videos: videos, index: index, segments: segments}
}

// Manifest returns the VOD m3u8 of a Video. Segment URIs are bare file names;
// the caller appends any auth query.
//
// Returns model.ErrNotRemux (wrapped) for a Video whose Play Mode is not remux;
// model.ErrStreamPreparing (wrapped) while the Keyframe Index is being probed;
// otherwise follows VideoService.ResolvePlayback's error contract.
func (s *HLSService) Manifest(ctx context.Context, videoID string) ([]byte, error) {
	_, segs, err := s.boundaries(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return streaming.BuildVODManifest(segs), nil
}

// Segment returns the path of a ready HLS Segment file, cutting it first if
// it is not cached. name is a file name listed by Manifest.
//
// Returns model.ErrInvalidInput (wrapped) if name is not a segment name;
// model.ErrNotFound (wrapped) if the index is past the last segment;
// otherwise the same errors as Manifest, or a wrapped generation error.
func (s *HLSService) Segment(ctx context.Context, videoID, name string) (string, error) {
	idx, ok := streaming.ParseSegmentName(name)
	if !ok {
		return "", fmt.Errorf("segment name %q: %w", name, model.ErrInvalidInput)
	}
	inputPath, segs, err := s.boundaries(ctx, videoID)
	if err != nil {
		return "", err
	}
	if idx >= len(segs) {
		return "", fmt.Errorf("segment %d of video %s (has %d): %w", idx, videoID, len(segs), model.ErrNotFound)
	}
	path, err := s.segments.EnsureSegment(ctx, videoID, inputPath, idx, segs[idx])
	if err != nil {
		return "", fmt.Errorf("failed to ensure segment %d of video %s: %w", idx, videoID, err)
	}
	return path, nil
}

// boundaries resolves a remux Video and loads its Segment Boundary table.
func (s *HLSService) boundaries(ctx context.Context, videoID string) (string, []model.SegmentBoundary, error) {
	inputPath, mode, err := s.videos.ResolvePlayback(ctx, videoID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to resolve playback of video %s: %w", videoID, err)
	}
	if mode != model.PlayModeRemux {
		return "", nil, fmt.Errorf("video %s is %s: %w", videoID, mode, model.ErrNotRemux)
	}
	segs, err := s.index.Lookup(ctx, videoID, inputPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to look up segments of video %s: %w", videoID, err)
	}
	return inputPath, segs, nil
}
