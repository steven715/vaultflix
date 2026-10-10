package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func previewVideo() *model.Video {
	src, fp := "s1", "movie.mkv"
	return &model.Video{ID: "v1", DurationSeconds: 100, SourceID: &src, FilePath: &fp}
}

func TestPreviewBackfill_ProcessOne_UploadsAndRecordsKey(t *testing.T) {
	var recordedKey string
	repo := &mock.VideoRepository{UpdatePreviewKeyFunc: func(_ context.Context, _, key string) error {
		recordedKey = key
		return nil
	}}
	minio := &mock.MinIOClient{UploadPreviewFunc: func(context.Context, string, string) error { return nil }}
	media := NewMediaProcessor(&mock.MediaTool{Dir: t.TempDir()}, minio)

	if err := NewPreviewBackfill(repo, mock.ResolveUnder("/mnt/host/D"), media).ProcessOne(context.Background(), previewVideo()); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if recordedKey != "previews/v1.mp4" {
		t.Errorf("recorded %q, want previews/v1.mp4", recordedKey)
	}
}

func TestPreviewBackfill_ProcessOne_Failures(t *testing.T) {
	tests := []struct {
		name  string
		files mediaFileResolver
		tool  *mock.MediaTool
		want  error // nil = any error
	}{
		{"disabled Media Source is skipped", mock.ResolveFailing(model.ErrMediaSourceDisabled), &mock.MediaTool{}, model.ErrMediaSourceDisabled},
		{"clip fails", mock.ResolveUnder("/mnt/host/D"), &mock.MediaTool{PreviewErr: errors.New("boom")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mock.VideoRepository{UpdatePreviewKeyFunc: func(context.Context, string, string) error {
				t.Error("preview key recorded despite failure")
				return nil
			}}
			media := NewMediaProcessor(tt.tool, &mock.MinIOClient{})
			err := NewPreviewBackfill(repo, tt.files, media).ProcessOne(context.Background(), previewVideo())
			if err == nil || (tt.want != nil && !errors.Is(err, tt.want)) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
