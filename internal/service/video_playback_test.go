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
		MimeType: "video/x-matroska", SourceID: strPtr("s1"), FilePath: strPtr("sub/a.mkv")}
	svc := NewVideoService(playbackVideoRepo(v), mock.ResolveUnder("/mnt/host/D"), &mock.TagRepository{}, &mock.MinIOClient{})

	src, err := svc.ResolvePlayback(context.Background(), "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := model.PlaybackSource{Path: "/mnt/host/D/sub/a.mkv", PlayMode: model.PlayModeRemux, MimeType: "video/x-matroska"}
	if *src != want {
		t.Errorf("source = %+v, want %+v", *src, want)
	}
}

func TestResolvePlayback_LegacyVideoGivesItsObjectKey(t *testing.T) {
	v := &model.Video{ID: "v1", OriginalFilename: "a.mp4", MinIOObjectKey: "videos/v1/a.mp4", MimeType: "video/mp4"}
	svc := NewVideoService(playbackVideoRepo(v), mock.ResolveUnder("/mnt/host/D"), &mock.TagRepository{}, &mock.MinIOClient{})

	src, err := svc.ResolvePlayback(context.Background(), "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.Path != "" || src.LegacyObjectKey != "videos/v1/a.mp4" || src.MimeType != "video/mp4" {
		t.Errorf("source = %+v, want the legacy object key and no path", *src)
	}
}

func TestResolvePlayback_VideoMissing(t *testing.T) {
	svc := NewVideoService(playbackVideoRepo(nil), mock.ResolveUnder("/mnt/host/D"), &mock.TagRepository{}, &mock.MinIOClient{})
	if _, err := svc.ResolvePlayback(context.Background(), "v1"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// Broken data is an error worth logging, never a "Video not found".
func TestResolvePlayback_IntegrityProblemsAreNotNotFound(t *testing.T) {
	tests := []struct {
		name  string
		video *model.Video
		files mediaFileResolver
	}{
		{"neither a Media Source nor a MinIO object", &model.Video{ID: "v1", OriginalFilename: "a.mkv"}, mock.ResolveUnder("/mnt/host/D")},
		{"its Media Source row is gone", &model.Video{ID: "v1", OriginalFilename: "a.mkv", SourceID: strPtr("s1"), FilePath: strPtr("a.mkv")},
			mock.ResolveFailing(model.ErrNotFound)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewVideoService(playbackVideoRepo(tt.video), tt.files, &mock.TagRepository{}, &mock.MinIOClient{})
			_, err := svc.ResolvePlayback(context.Background(), "v1")
			if err == nil || errors.Is(err, model.ErrNotFound) {
				t.Errorf("err = %v, want an error that is not ErrNotFound", err)
			}
		})
	}
}

func TestResolvePlayback_PropagatesResolveError(t *testing.T) {
	v := &model.Video{ID: "v1", OriginalFilename: "a.mkv", SourceID: strPtr("s1"), FilePath: strPtr("a.mkv")}
	svc := NewVideoService(playbackVideoRepo(v), mock.ResolveFailing(model.ErrMediaSourceDisabled), &mock.TagRepository{}, &mock.MinIOClient{})

	_, err := svc.ResolvePlayback(context.Background(), "v1")
	if !errors.Is(err, model.ErrMediaSourceDisabled) {
		t.Errorf("err = %v, want ErrMediaSourceDisabled", err)
	}
}
