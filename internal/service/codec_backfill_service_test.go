package service

import (
	"context"
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

// fakeCodecRepo satisfies the subset of VideoRepository needed by CodecBackfillService.
type fakeCodecRepo struct {
	missing []model.Video
	updated map[string][2]string
}

func (r *fakeCodecRepo) ListMissingCodecs(_ context.Context, _ int) ([]model.Video, error) {
	return r.missing, nil
}

func (r *fakeCodecRepo) UpdateCodecs(_ context.Context, id, vc, ac string) error {
	if r.updated == nil {
		r.updated = make(map[string][2]string)
	}
	r.updated[id] = [2]string{vc, ac}
	return nil
}

// fakeSourceRepo satisfies the subset of MediaSourceRepository needed by CodecBackfillService.
type fakeSourceRepo struct {
	mount string
}

func (r *fakeSourceRepo) FindByID(_ context.Context, _ string) (*model.MediaSource, error) {
	return &model.MediaSource{MountPath: r.mount}, nil
}

func TestCodecBackfill_Run_UpdatesMissing(t *testing.T) {
	src := "s1"
	fp := "movie.mkv"
	repo := &fakeCodecRepo{
		missing: []model.Video{{ID: "v1", OriginalFilename: "movie.mkv", SourceID: &src, FilePath: &fp}},
	}
	svc := NewCodecBackfillService(repo, &fakeSourceRepo{mount: "/mnt/host/D"})
	svc.probe = func(_ context.Context, _ string) (string, string, error) {
		return "h264", "aac", nil
	}

	processed, failed, err := svc.Run(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if processed != 1 || failed != 0 {
		t.Errorf("processed=%d failed=%d, want 1/0", processed, failed)
	}
	if repo.updated["v1"] != [2]string{"h264", "aac"} {
		t.Errorf("v1 codecs = %v, want [h264 aac]", repo.updated["v1"])
	}
}
