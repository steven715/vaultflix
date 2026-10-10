package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func playbackVideoRepo(v *model.Video) *mock.VideoRepository {
	return &mock.VideoRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
			if v == nil || id != v.ID {
				return nil, model.ErrNotFound
			}
			return v, nil
		},
	}
}

func TestResolvePlayback_ReturnsPathAndPlayMode(t *testing.T) {
	v := &model.Video{ID: "v1", OriginalFilename: "a.mkv", VideoCodec: "h264", AudioCodec: "aac",
		SourceID: strPtr("s1"), FilePath: strPtr("sub/a.mkv")}
	svc := NewVideoService(playbackVideoRepo(v), mock.ResolveUnder("/mnt/host/D"), &mock.TagRepository{}, &mock.MinIOClient{})

	path, mode, err := svc.ResolvePlayback(context.Background(), "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/mnt/host/D/sub/a.mkv" {
		t.Errorf("path = %q, want /mnt/host/D/sub/a.mkv", path)
	}
	if mode != model.PlayModeRemux {
		t.Errorf("mode = %q, want remux", mode)
	}
}

func TestResolvePlayback_NotFound(t *testing.T) {
	tests := []struct {
		name  string
		video *model.Video
	}{
		{"video missing", nil},
		{"legacy video without Media Source", &model.Video{ID: "v1", OriginalFilename: "a.mkv"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewVideoService(playbackVideoRepo(tt.video), mock.ResolveUnder("/mnt/host/D"), &mock.TagRepository{}, &mock.MinIOClient{})
			_, _, err := svc.ResolvePlayback(context.Background(), "v1")
			if !errors.Is(err, model.ErrNotFound) {
				t.Errorf("err = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestResolvePlayback_PropagatesResolveError(t *testing.T) {
	v := &model.Video{ID: "v1", OriginalFilename: "a.mkv", SourceID: strPtr("s1"), FilePath: strPtr("a.mkv")}
	svc := NewVideoService(playbackVideoRepo(v), mock.ResolveFailing(model.ErrMediaSourceDisabled), &mock.TagRepository{}, &mock.MinIOClient{})

	_, _, err := svc.ResolvePlayback(context.Background(), "v1")
	if !errors.Is(err, model.ErrMediaSourceDisabled) {
		t.Errorf("err = %v, want ErrMediaSourceDisabled", err)
	}
}
