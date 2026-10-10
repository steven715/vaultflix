package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/streaming"
)

var threeSegments = []model.SegmentBoundary{
	{Start: 0, Duration: 8.341},
	{Start: 8.341, Duration: 6.0},
	{Start: 14.341, Duration: 3.5},
}

// newTestHLSService wires HLSService to a real SegmentCache in a temp dir; only
// ffmpeg (SegmentGenerator) is faked.
func newTestHLSService(t *testing.T, videos *mock.PlaybackResolver, index *mock.KeyframeLookup) (*HLSService, *mock.SegmentGenerator) {
	t.Helper()
	gen := &mock.SegmentGenerator{}
	cache, err := streaming.NewSegmentCache(gen, t.TempDir(), time.Minute, 0)
	if err != nil {
		t.Fatalf("NewSegmentCache: %v", err)
	}
	return NewHLSService(videos, index, cache), gen
}

func remuxVideo() *mock.PlaybackResolver {
	return &mock.PlaybackResolver{Path: "/mnt/host/D/a.mkv", Mode: model.PlayModeRemux}
}

func TestHLSManifest_ListsEverySegment(t *testing.T) {
	svc, _ := newTestHLSService(t, remuxVideo(), &mock.KeyframeLookup{Segments: threeSegments})

	manifest, err := svc.Manifest(context.Background(), "v1")
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	for _, want := range []string{"#EXTINF:8.341000,", "#EXTINF:3.500000,", "#EXT-X-ENDLIST"} {
		if !strings.Contains(string(manifest), want) {
			t.Errorf("manifest missing %q:\n%s", want, manifest)
		}
	}
}

// Every name the manifest lists must be accepted by Segment and cut at the
// boundary the manifest declared for it.
func TestHLSSegment_EveryManifestNameIsServable(t *testing.T) {
	svc, gen := newTestHLSService(t, remuxVideo(), &mock.KeyframeLookup{Segments: threeSegments})
	manifest, err := svc.Manifest(context.Background(), "v1")
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}

	var names []string
	for _, line := range strings.Split(string(manifest), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			names = append(names, line)
		}
	}
	if len(names) != len(threeSegments) {
		t.Fatalf("manifest lists %d segments, want %d", len(names), len(threeSegments))
	}
	for _, name := range names {
		if _, err := svc.Segment(context.Background(), "v1", name); err != nil {
			t.Fatalf("Segment(%q): %v", name, err)
		}
	}

	calls := gen.Calls()
	if len(calls) != len(threeSegments) {
		t.Fatalf("generator calls = %d, want %d", len(calls), len(threeSegments))
	}
	for i, want := range threeSegments {
		if calls[i] != want {
			t.Errorf("segment %d cut at %+v, want %+v", i, calls[i], want)
		}
	}
}

func TestHLSService_RejectsNonRemuxWithoutTouchingIndex(t *testing.T) {
	for _, mode := range []model.PlayMode{model.PlayModeDirect, model.PlayModeTranscode} {
		t.Run(string(mode), func(t *testing.T) {
			index := &mock.KeyframeLookup{Segments: threeSegments}
			svc, _ := newTestHLSService(t, &mock.PlaybackResolver{Path: "/mnt/host/D/a.mp4", Mode: mode}, index)

			if _, err := svc.Manifest(context.Background(), "v1"); !errors.Is(err, model.ErrNotRemux) {
				t.Errorf("Manifest err = %v, want ErrNotRemux", err)
			}
			if _, err := svc.Segment(context.Background(), "v1", "seg00000.ts"); !errors.Is(err, model.ErrNotRemux) {
				t.Errorf("Segment err = %v, want ErrNotRemux", err)
			}
			if index.Calls() != 0 {
				t.Errorf("Keyframe Index looked up %d times for a %s video", index.Calls(), mode)
			}
		})
	}
}

func TestHLSSegment_InvalidName(t *testing.T) {
	svc, _ := newTestHLSService(t, remuxVideo(), &mock.KeyframeLookup{Segments: threeSegments})
	for _, name := range []string{"seg1.ts", "seg00000.mp4", "../seg00000.ts", "index.m3u8"} {
		if _, err := svc.Segment(context.Background(), "v1", name); !errors.Is(err, model.ErrInvalidInput) {
			t.Errorf("Segment(%q) err = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestHLSSegment_OutOfRange(t *testing.T) {
	svc, gen := newTestHLSService(t, remuxVideo(), &mock.KeyframeLookup{Segments: threeSegments})

	_, err := svc.Segment(context.Background(), "v1", "seg00003.ts")
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if len(gen.Calls()) != 0 {
		t.Error("ffmpeg ran for an out-of-range segment")
	}
}

func TestHLSService_PropagatesUpstreamErrors(t *testing.T) {
	tests := []struct {
		name   string
		videos *mock.PlaybackResolver
		index  *mock.KeyframeLookup
		want   error
	}{
		{"Keyframe Index preparing", remuxVideo(), &mock.KeyframeLookup{Err: model.ErrStreamPreparing}, model.ErrStreamPreparing},
		{"Media Source disabled", &mock.PlaybackResolver{Err: model.ErrMediaSourceDisabled}, &mock.KeyframeLookup{}, model.ErrMediaSourceDisabled},
		{"Video missing", &mock.PlaybackResolver{Err: model.ErrNotFound}, &mock.KeyframeLookup{}, model.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTestHLSService(t, tt.videos, tt.index)
			if _, err := svc.Manifest(context.Background(), "v1"); !errors.Is(err, tt.want) {
				t.Errorf("Manifest err = %v, want %v", err, tt.want)
			}
			if _, err := svc.Segment(context.Background(), "v1", "seg00000.ts"); !errors.Is(err, tt.want) {
				t.Errorf("Segment err = %v, want %v", err, tt.want)
			}
		})
	}
}
