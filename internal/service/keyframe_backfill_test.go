package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func newTestKeyframeBackfill(videos []model.Video, files mediaFileResolver) (*KeyframeBackfill, *fakeKeyframeRepo, *KeyframeService) {
	repo := newFakeKeyframeRepo()
	svc := NewKeyframeService(repo)
	return NewKeyframeBackfill(svc, &fakeKfVideoRepo{videos: videos}, files), repo, svc
}

func TestKeyframeBackfill_List_OnlyRemuxVideos(t *testing.T) {
	b, _, _ := newTestKeyframeBackfill([]model.Video{
		{ID: "remux1", OriginalFilename: "a.avi", VideoCodec: "h264", AudioCodec: "aac"},
		{ID: "direct1", OriginalFilename: "b.mp4", VideoCodec: "h264", AudioCodec: "aac"},
		{ID: "transcode1", OriginalFilename: "c.wmv", VideoCodec: "wmv3", AudioCodec: "wmav2"},
	}, mock.ResolveUnder("/mnt/host/D"))

	got, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].ID != "remux1" {
		t.Errorf("List = %+v, want only remux1", got)
	}
}

func TestKeyframeBackfill_ProcessOne_StoresIndex(t *testing.T) {
	b, repo, svc := newTestKeyframeBackfill(nil, mock.ResolveUnder("/mnt/host/D"))
	var probed string
	svc.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		probed = absPath
		return []float64{0, 8}, 16, nil
	}
	v := &model.Video{ID: "remux1", SourceID: strPtr("s1"), FilePath: strPtr("a.avi")}

	if err := b.ProcessOne(context.Background(), v); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if probed != "/mnt/host/D/a.avi" {
		t.Errorf("probed %q, want /mnt/host/D/a.avi", probed)
	}
	if _, ok := repo.stored["remux1"]; !ok {
		t.Error("Keyframe Index not stored")
	}
}

func TestKeyframeBackfill_ProcessOne_ProbeFailure(t *testing.T) {
	b, _, svc := newTestKeyframeBackfill(nil, mock.ResolveUnder("/mnt/host/D"))
	svc.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		return nil, 0, errors.New("boom")
	}
	v := &model.Video{ID: "remux1", SourceID: strPtr("s1"), FilePath: strPtr("a.avi")}

	if err := b.ProcessOne(context.Background(), v); err == nil {
		t.Error("ProcessOne succeeded although the probe failed")
	}
}

func TestKeyframeBackfill_ProcessOne_DisabledMediaSourceSkipsProbe(t *testing.T) {
	b, _, svc := newTestKeyframeBackfill(nil, mock.ResolveFailing(model.ErrMediaSourceDisabled))
	svc.probe = func(ctx context.Context, absPath string) ([]float64, float64, error) {
		t.Error("ffprobe ran for a Video on a disabled Media Source")
		return nil, 0, nil
	}
	v := &model.Video{ID: "remux1", SourceID: strPtr("s1"), FilePath: strPtr("a.avi")}

	if err := b.ProcessOne(context.Background(), v); !errors.Is(err, model.ErrMediaSourceDisabled) {
		t.Errorf("err = %v, want ErrMediaSourceDisabled", err)
	}
}
