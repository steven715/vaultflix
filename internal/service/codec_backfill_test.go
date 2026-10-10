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

func newTestCodecBackfill(repo *fakeCodecRepo, files mediaFileResolver) (*CodecBackfill, *mock.MediaTool) {
	tool := &mock.MediaTool{ProbeOutput: map[string][]byte{"movie.mkv": mock.ProbeJSONFor(60, "h264", "aac")}}
	return NewCodecBackfill(repo, files, NewMediaProcessor(tool, &mock.MinIOClient{})), tool
}

func TestCodecBackfill_ProcessOne_StoresProbedCodecs(t *testing.T) {
	repo := &fakeCodecRepo{}
	src, fp := "s1", "movie.mkv"

	svc, tool := newTestCodecBackfill(repo, mock.ResolveUnder("/mnt/host/D"))
	if err := svc.ProcessOne(context.Background(), &model.Video{ID: "v1", SourceID: &src, FilePath: &fp}); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if got := tool.Probed(); len(got) != 1 || got[0] != "/mnt/host/D/movie.mkv" {
		t.Errorf("probed %v, want the resolved /mnt/host/D/movie.mkv", got)
	}
	if repo.updated["v1"] != [2]string{"h264", "aac"} {
		t.Errorf("v1 codecs = %v, want [h264 aac]", repo.updated["v1"])
	}
}

func TestCodecBackfill_ProcessOne_Failures(t *testing.T) {
	src, fp := "s1", "movie.mkv"
	tests := []struct {
		name  string
		video *model.Video
		files mediaFileResolver
		want  error // nil = any error
	}{
		{"disabled Media Source is skipped", &model.Video{ID: "v1", SourceID: &src, FilePath: &fp}, mock.ResolveFailing(model.ErrMediaSourceDisabled), model.ErrMediaSourceDisabled},
		// A Video without a Media Source must fail, not panic the runner's goroutine.
		{"Video without Media Source", &model.Video{ID: "legacy"}, mock.ResolveUnder("/mnt/host/D"), nil},
		{"probe fails", &model.Video{ID: "v2", SourceID: &src, FilePath: strPtr("unknown.mkv")}, mock.ResolveUnder("/mnt/host/D"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeCodecRepo{}
			svc, _ := newTestCodecBackfill(repo, tt.files)
			err := svc.ProcessOne(context.Background(), tt.video)
			if err == nil || (tt.want != nil && !errors.Is(err, tt.want)) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			if len(repo.updated) != 0 {
				t.Errorf("codecs stored despite failure: %v", repo.updated)
			}
		})
	}
}
