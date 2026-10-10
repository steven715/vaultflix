package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func previewVideo() *model.Video {
	src, fp := "s1", "movie.mkv"
	return &model.Video{ID: "v1", DurationSeconds: 100, SourceID: &src, FilePath: &fp}
}

func TestPreviewBackfill_ProcessOne_UploadsAndRecordsKey(t *testing.T) {
	var uploadedKey, recordedKey string
	repo := &mock.VideoRepository{UpdatePreviewKeyFunc: func(_ context.Context, id, key string) error {
		recordedKey = key
		return nil
	}}
	minio := &mock.MinIOClient{UploadPreviewFunc: func(_ context.Context, key, _ string) error {
		uploadedKey = key
		return nil
	}}
	b := NewPreviewBackfill(repo, mock.ResolveUnder("/mnt/host/D"), minio)
	var clipped string
	b.generatePreview = func(_ context.Context, src string, _ int) (string, error) {
		clipped = src
		f, err := os.CreateTemp(t.TempDir(), "preview-*.mp4")
		if err != nil {
			return "", err
		}
		f.Close()
		return f.Name(), nil
	}

	if err := b.ProcessOne(context.Background(), previewVideo()); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if clipped != "/mnt/host/D/movie.mkv" {
		t.Errorf("clipped %q, want /mnt/host/D/movie.mkv", clipped)
	}
	if uploadedKey != "previews/v1.mp4" || recordedKey != "previews/v1.mp4" {
		t.Errorf("uploaded %q, recorded %q, want previews/v1.mp4 for both", uploadedKey, recordedKey)
	}
}

func TestPreviewBackfill_ProcessOne_DisabledMediaSourceSkipsFfmpeg(t *testing.T) {
	b := NewPreviewBackfill(&mock.VideoRepository{}, mock.ResolveFailing(model.ErrMediaSourceDisabled), &mock.MinIOClient{})
	b.generatePreview = func(context.Context, string, int) (string, error) {
		t.Error("ffmpeg ran for a Video on a disabled Media Source")
		return "", nil
	}

	if err := b.ProcessOne(context.Background(), previewVideo()); !errors.Is(err, model.ErrMediaSourceDisabled) {
		t.Errorf("err = %v, want ErrMediaSourceDisabled", err)
	}
}

func TestPreviewBackfill_ProcessOne_ClipFailureUploadsNothing(t *testing.T) {
	minio := &mock.MinIOClient{UploadPreviewFunc: func(context.Context, string, string) error {
		t.Error("uploaded although clipping failed")
		return nil
	}}
	b := NewPreviewBackfill(&mock.VideoRepository{}, mock.ResolveUnder("/mnt/host/D"), minio)
	b.generatePreview = func(context.Context, string, int) (string, error) { return "", errors.New("boom") }

	if err := b.ProcessOne(context.Background(), previewVideo()); err == nil {
		t.Error("ProcessOne succeeded although clipping failed")
	}
}
