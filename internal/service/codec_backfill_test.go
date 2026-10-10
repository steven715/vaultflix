package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
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

func TestCodecBackfill_ProcessOne_StoresProbedCodecs(t *testing.T) {
	repo := &fakeCodecRepo{}
	svc := NewCodecBackfill(repo, mock.ResolveUnder("/mnt/host/D"))
	var probed string
	svc.probe = func(_ context.Context, absPath string) (string, string, error) {
		probed = absPath
		return "h264", "aac", nil
	}
	src, fp := "s1", "movie.mkv"

	if err := svc.ProcessOne(context.Background(), &model.Video{ID: "v1", SourceID: &src, FilePath: &fp}); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if probed != "/mnt/host/D/movie.mkv" {
		t.Errorf("probed %q, want /mnt/host/D/movie.mkv", probed)
	}
	if repo.updated["v1"] != [2]string{"h264", "aac"} {
		t.Errorf("v1 codecs = %v, want [h264 aac]", repo.updated["v1"])
	}
}

func TestCodecBackfill_ProcessOne_DisabledMediaSourceSkipsProbe(t *testing.T) {
	svc := NewCodecBackfill(&fakeCodecRepo{}, mock.ResolveFailing(model.ErrMediaSourceDisabled))
	svc.probe = func(_ context.Context, _ string) (string, string, error) {
		t.Error("ffprobe ran for a Video on a disabled Media Source")
		return "", "", nil
	}
	src, fp := "s1", "movie.mkv"

	err := svc.ProcessOne(context.Background(), &model.Video{ID: "v1", SourceID: &src, FilePath: &fp})
	if !errors.Is(err, model.ErrMediaSourceDisabled) {
		t.Errorf("err = %v, want ErrMediaSourceDisabled", err)
	}
}

// A Video without a Media Source must fail, not panic the runner's goroutine.
func TestCodecBackfill_ProcessOne_VideoWithoutMediaSourceFails(t *testing.T) {
	svc := NewCodecBackfill(&fakeCodecRepo{}, mock.ResolveUnder("/mnt/host/D"))
	if err := svc.ProcessOne(context.Background(), &model.Video{ID: "legacy"}); err == nil {
		t.Error("ProcessOne succeeded for a Video without a Media Source")
	}
}
