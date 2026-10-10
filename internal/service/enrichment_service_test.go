package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/scraper"
)

func TestEnrichVideo_NoCode(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: id, OriginalFilename: "family_trip.mp4"}, nil
		},
		SetEnrichmentStatusFunc: func(_ context.Context, id, status string) error {
			if status != model.EnrichmentNoCode {
				t.Errorf("status = %q, want %q", status, model.EnrichmentNoCode)
			}
			return nil
		},
	}
	svc := NewEnrichmentService(
		nil,
		videoRepo,
		&mock.ActressRepository{},
		&mock.SuggestionRepository{},
		&mock.TagRepository{},
		&mock.MinIOClient{},
		&mock.Notifier{},
	)
	err := svc.EnrichVideo(context.Background(), "v1", "u1")
	if err == nil {
		t.Fatal("want ErrCodeNotFound, got nil")
	}
}

func TestEnrichVideo_WritesSuggestion(t *testing.T) {
	var created *model.MetadataSuggestion
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: id, OriginalFilename: "DASD-626.mp4"}, nil
		},
		SetEnrichmentStatusFunc: func(_ context.Context, id, status string) error { return nil },
	}
	sugRepo := &mock.SuggestionRepository{
		CreateFunc: func(_ context.Context, s *model.MetadataSuggestion) error {
			created = s
			return nil
		},
	}
	fakeScraper := &mock.Scraper{
		SourceValue: "javbus",
		ScrapeByCodeFunc: func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
			return &model.EnrichedMetadata{Code: code, Title: "T", Maker: "M"}, nil
		},
	}
	svc := NewEnrichmentService(
		[]scraper.MetadataScraper{fakeScraper},
		videoRepo,
		&mock.ActressRepository{},
		sugRepo,
		&mock.TagRepository{},
		&mock.MinIOClient{},
		&mock.Notifier{},
	)
	if err := svc.EnrichVideo(context.Background(), "v1", "u1"); err != nil {
		t.Fatal(err)
	}
	if created == nil || created.Source != "javbus" || created.Code != "DASD-626" {
		t.Fatalf("suggestion not created correctly: %+v", created)
	}
}

func TestEnrichVideo_AllFailed(t *testing.T) {
	var recordedStatus string
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: id, OriginalFilename: "DASD-626.mp4"}, nil
		},
		SetEnrichmentStatusFunc: func(_ context.Context, id, status string) error {
			recordedStatus = status
			return nil
		},
	}
	failingScraper := &mock.Scraper{
		SourceValue: "javbus",
		ScrapeByCodeFunc: func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
			return nil, model.ErrSourceUnavailable
		},
	}
	sugRepo := &mock.SuggestionRepository{
		CreateFunc: func(_ context.Context, s *model.MetadataSuggestion) error {
			t.Error("suggestion should not be created when all sources fail")
			return nil
		},
	}
	svc := NewEnrichmentService(
		[]scraper.MetadataScraper{failingScraper},
		videoRepo,
		&mock.ActressRepository{},
		sugRepo,
		&mock.TagRepository{},
		&mock.MinIOClient{},
		&mock.Notifier{},
	)
	err := svc.EnrichVideo(context.Background(), "v1", "u1")
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
	if !errors.Is(err, model.ErrSourceUnavailable) {
		t.Errorf("errors.Is(err, model.ErrSourceUnavailable) = false, want true; err = %v", err)
	}
	if recordedStatus != model.EnrichmentFailed {
		t.Errorf("status = %q, want %q", recordedStatus, model.EnrichmentFailed)
	}
}

func TestAcceptSuggestion_AppliesEverythingInOneCall(t *testing.T) {
	payload := model.EnrichedMetadata{
		Code: "DASD-626", Title: "原標題", Maker: "M",
		Genres:    []string{"巨乳"},
		Actresses: []model.ActressMeta{{NameJa: "女優A", NameRomaji: "Joyu A", AvatarURL: "actresses/k.jpg"}},
		CoverURL:  "covers/DASD-626-javbus.jpg",
	}
	var applied *model.SuggestionApplication
	sugRepo := &mock.SuggestionRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.MetadataSuggestion, error) {
			return &model.MetadataSuggestion{ID: id, VideoID: "v1", Source: "javbus", Code: "DASD-626", Payload: payload}, nil
		},
		ApplyFunc: func(_ context.Context, app model.SuggestionApplication) error {
			applied = &app
			return nil
		},
	}
	svc := NewEnrichmentService(nil, &mock.VideoRepository{}, &mock.ActressRepository{}, sugRepo, &mock.TagRepository{}, &mock.MinIOClient{}, &mock.Notifier{})
	newTitle := "覆寫標題"

	if err := svc.AcceptSuggestion(context.Background(), "v1", "s1", model.SuggestionOverride{Title: &newTitle, Genres: []string{"單體", ""}}); err != nil {
		t.Fatal(err)
	}
	if applied == nil {
		t.Fatal("Apply not called")
	}
	if applied.SuggestionID != "s1" || applied.VideoID != "v1" {
		t.Errorf("ids = %s/%s", applied.SuggestionID, applied.VideoID)
	}
	if m := applied.Metadata; m.Title != "覆寫標題" || m.Code != "DASD-626" || m.Maker != "M" || m.CoverKey != "covers/DASD-626-javbus.jpg" {
		t.Errorf("metadata = %+v (title override must win)", m)
	}
	if len(applied.Genres) != 1 || applied.Genres[0] != "單體" {
		t.Errorf("genres = %v, want the override without empty names", applied.Genres)
	}
	if len(applied.Performers) != 1 || applied.Performers[0] != (model.Actress{NameJa: "女優A", NameRomaji: "Joyu A", AvatarKey: "actresses/k.jpg"}) {
		t.Errorf("performers = %+v", applied.Performers)
	}
}

func TestAcceptSuggestion_ApplyFailurePropagates(t *testing.T) {
	sugRepo := &mock.SuggestionRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.MetadataSuggestion, error) {
			return &model.MetadataSuggestion{ID: id, VideoID: "v1"}, nil
		},
		ApplyFunc: func(context.Context, model.SuggestionApplication) error { return model.ErrNotFound },
	}
	svc := NewEnrichmentService(nil, &mock.VideoRepository{}, &mock.ActressRepository{}, sugRepo, &mock.TagRepository{}, &mock.MinIOClient{}, &mock.Notifier{})
	if err := svc.AcceptSuggestion(context.Background(), "v1", "s1", model.SuggestionOverride{}); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestAcceptSuggestion_NotFound(t *testing.T) {
	sugRepo := &mock.SuggestionRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.MetadataSuggestion, error) {
			return nil, model.ErrNotFound
		},
	}
	svc := NewEnrichmentService(nil, &mock.VideoRepository{}, &mock.ActressRepository{}, sugRepo, &mock.TagRepository{}, &mock.MinIOClient{}, &mock.Notifier{})
	err := svc.AcceptSuggestion(context.Background(), "v1", "s1", model.SuggestionOverride{})
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("errors.Is(err, model.ErrNotFound) = false, want true; err = %v", err)
	}
}

func TestAcceptSuggestion_WrongVideo(t *testing.T) {
	sugRepo := &mock.SuggestionRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.MetadataSuggestion, error) {
			return &model.MetadataSuggestion{ID: id, VideoID: "other-video"}, nil
		},
	}
	svc := NewEnrichmentService(nil, &mock.VideoRepository{}, &mock.ActressRepository{}, sugRepo, &mock.TagRepository{}, &mock.MinIOClient{}, &mock.Notifier{})
	err := svc.AcceptSuggestion(context.Background(), "v1", "s1", model.SuggestionOverride{})
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("errors.Is(err, model.ErrNotFound) = false, want true; err = %v", err)
	}
}

func TestRejectSuggestion_DeletesAndResetsStatusWhenLast(t *testing.T) {
	var deletedID string
	var statusSet string
	videoRepo := &mock.VideoRepository{
		SetEnrichmentStatusFunc: func(_ context.Context, id, status string) error {
			statusSet = status
			return nil
		},
	}
	sugRepo := &mock.SuggestionRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.MetadataSuggestion, error) {
			return &model.MetadataSuggestion{ID: id, VideoID: "v1"}, nil
		},
		DeleteFunc: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
		GetByVideoIDFunc: func(_ context.Context, videoID string) ([]model.MetadataSuggestion, error) {
			return []model.MetadataSuggestion{}, nil
		},
	}
	svc := NewEnrichmentService(nil, videoRepo, &mock.ActressRepository{}, sugRepo, &mock.TagRepository{}, &mock.MinIOClient{}, &mock.Notifier{})
	err := svc.RejectSuggestion(context.Background(), "v1", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if deletedID != "s1" {
		t.Errorf("deleted suggestion ID = %q, want s1", deletedID)
	}
	if statusSet != model.EnrichmentNone {
		t.Errorf("enrichment status = %q, want %q", statusSet, model.EnrichmentNone)
	}
}

func TestEnrichVideo_AvatarUploadDoesNotOverwriteSharedKey(t *testing.T) {
	var avatarKeys []string
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: id, OriginalFilename: "DASD-626.mp4"}, nil
		},
		SetEnrichmentStatusFunc: func(_ context.Context, id, status string) error { return nil },
	}
	fakeScraper := &mock.Scraper{
		SourceValue: "javbus",
		ScrapeByCodeFunc: func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
			return &model.EnrichedMetadata{
				Code:      code,
				Title:     "T",
				Actresses: []model.ActressMeta{{NameJa: "山田 花子", AvatarURL: "https://example.com/a.jpg"}},
			}, nil
		},
	}
	var staged *model.MetadataSuggestion
	svc := NewEnrichmentService(
		[]scraper.MetadataScraper{fakeScraper},
		videoRepo,
		&mock.ActressRepository{},
		&mock.SuggestionRepository{
			CreateFunc: func(_ context.Context, s *model.MetadataSuggestion) error {
				staged = s
				return nil
			},
		},
		&mock.TagRepository{},
		&mock.MinIOClient{
			UploadActressAvatarFunc: func(_ context.Context, key, _ string) error {
				avatarKeys = append(avatarKeys, key)
				return nil
			},
		},
		&mock.Notifier{},
	)
	svc.downloadImage = func(_ context.Context, _ string) (string, error) {
		f, err := os.CreateTemp(t.TempDir(), "img-*.jpg")
		if err != nil {
			return "", err
		}
		f.Close()
		return f.Name(), nil
	}

	if err := svc.EnrichVideo(context.Background(), "v1", "u1"); err != nil {
		t.Fatal(err)
	}

	const want = "actresses/山田_花子-DASD-626-javbus.jpg"
	if len(avatarKeys) != 1 || avatarKeys[0] != want {
		t.Fatalf("avatar keys = %v, want [%s]", avatarKeys, want)
	}
	if got := staged.Payload.Actresses[0].AvatarURL; got != want {
		t.Errorf("staged avatar key = %q, want %q", got, want)
	}
}

func TestEnrichVideo_PrefersStoredCode(t *testing.T) {
	tests := []struct {
		name     string
		video    model.Video
		wantCode string
	}{
		{
			name:     "stored code wins over filename",
			video:    model.Video{ID: "v1", OriginalFilename: "DASD-626.mp4", Code: "SSIS-001"},
			wantCode: "SSIS-001",
		},
		{
			name:     "stored code used when filename has none",
			video:    model.Video{ID: "v1", OriginalFilename: "family_trip.mp4", Code: "SSIS-001"},
			wantCode: "SSIS-001",
		},
		{
			name:     "falls back to filename when no stored code",
			video:    model.Video{ID: "v1", OriginalFilename: "DASD-626.mp4"},
			wantCode: "DASD-626",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var scrapedCode string
			videoRepo := &mock.VideoRepository{
				GetByIDFunc: func(_ context.Context, id string) (*model.Video, error) {
					v := tt.video
					return &v, nil
				},
				SetEnrichmentStatusFunc: func(_ context.Context, id, status string) error { return nil },
			}
			fakeScraper := &mock.Scraper{
				SourceValue: "javbus",
				ScrapeByCodeFunc: func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
					scrapedCode = code
					return &model.EnrichedMetadata{Code: code, Title: "T"}, nil
				},
			}
			svc := NewEnrichmentService(
				[]scraper.MetadataScraper{fakeScraper},
				videoRepo,
				&mock.ActressRepository{},
				&mock.SuggestionRepository{
					CreateFunc: func(_ context.Context, s *model.MetadataSuggestion) error { return nil },
				},
				&mock.TagRepository{},
				&mock.MinIOClient{},
				&mock.Notifier{},
			)
			if err := svc.EnrichVideo(context.Background(), "v1", "u1"); err != nil {
				t.Fatal(err)
			}
			if scrapedCode != tt.wantCode {
				t.Errorf("scraped code = %q, want %q", scrapedCode, tt.wantCode)
			}
		})
	}
}
