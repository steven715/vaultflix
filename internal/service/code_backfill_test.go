package service

import (
	"context"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestCodeBackfill_List_OnlyUnenrichedVideos(t *testing.T) {
	repo := &mock.VideoRepository{
		ListByEnrichmentStatusFunc: func(_ context.Context, status string) ([]model.Video, error) {
			if status != model.EnrichmentNone {
				t.Errorf("listed status %q, want %q", status, model.EnrichmentNone)
			}
			return []model.Video{{ID: "v1"}}, nil
		},
	}
	got, err := NewCodeBackfill(repo).List(context.Background())
	if err != nil || len(got) != 1 {
		t.Errorf("List = %v, %v", got, err)
	}
}

func TestCodeBackfill_ProcessOne_SeedsCodeFromFilename(t *testing.T) {
	tests := []struct {
		filename   string
		wantCode   string
		wantStatus string
	}{
		{"DASD-626.mp4", "DASD-626", model.EnrichmentPending},
		{"家庭聚會.mp4", "", model.EnrichmentNoCode},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			var gotCode, gotStatus string
			repo := &mock.VideoRepository{
				SeedCodeFunc: func(_ context.Context, id, code, status string) error {
					gotCode, gotStatus = code, status
					return nil
				},
			}
			err := NewCodeBackfill(repo).ProcessOne(context.Background(), &model.Video{ID: "v1", OriginalFilename: tt.filename})
			if err != nil {
				t.Fatalf("ProcessOne: %v", err)
			}
			if gotCode != tt.wantCode || gotStatus != tt.wantStatus {
				t.Errorf("seeded (%q, %q), want (%q, %q)", gotCode, gotStatus, tt.wantCode, tt.wantStatus)
			}
		})
	}
}

func TestCodeFromFilename_CodeMakesVideoPending(t *testing.T) {
	cases := []struct {
		filename   string
		wantCode   string
		wantStatus string
	}{
		{"DASD-626.mp4", "DASD-626", model.EnrichmentPending},
		{"FC2-PPV-1234567.mkv", "FC2-PPV-1234567", model.EnrichmentPending},
		{"家庭聚會.mp4", "", model.EnrichmentNoCode},
		{"random_home_video.mp4", "", model.EnrichmentNoCode},
	}
	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			code, status := codeFromFilename(tc.filename)
			if code != tc.wantCode || status != tc.wantStatus {
				t.Errorf("got (%q, %q), want (%q, %q)", code, status, tc.wantCode, tc.wantStatus)
			}
		})
	}
}
