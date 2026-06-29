package service

import (
	"context"
	"errors"
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

func TestAcceptSuggestion_AppliesMetadataAndActressesAndGenres(t *testing.T) {
	payload := model.EnrichedMetadata{
		Code: "DASD-626", Title: "原標題", Maker: "M",
		Genres:    []string{"巨乳"},
		Actresses: []model.ActressMeta{{NameJa: "女優A"}},
	}
	var updated model.VideoMetadataUpdate
	var linkedActress, linkedTag bool
	videoRepo := &mock.VideoRepository{
		UpdateMetadataFunc: func(_ context.Context, id string, m model.VideoMetadataUpdate) error {
			updated = m
			return nil
		},
	}
	sugRepo := &mock.SuggestionRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.MetadataSuggestion, error) {
			return &model.MetadataSuggestion{ID: id, VideoID: "v1", Source: "javbus", Code: "DASD-626", Payload: payload}, nil
		},
		DeleteFunc: func(_ context.Context, id string) error { return nil },
	}
	actRepo := &mock.ActressRepository{
		UpsertFunc:          func(_ context.Context, a *model.Actress) error { a.ID = "a1"; return nil },
		AddVideoActressFunc: func(_ context.Context, v, a string) error { linkedActress = true; return nil },
	}
	tagRepo := &mock.TagRepository{
		GetOrCreateByNameFunc: func(_ context.Context, name, cat string) (*model.Tag, error) {
			return &model.Tag{ID: 7, Name: name, Category: cat}, nil
		},
		AddVideoTagFunc: func(_ context.Context, v string, id int) error { linkedTag = true; return nil },
	}
	svc := NewEnrichmentService(nil, videoRepo, actRepo, sugRepo, tagRepo, &mock.MinIOClient{}, &mock.Notifier{})
	newTitle := "覆寫標題"
	err := svc.AcceptSuggestion(context.Background(), "v1", "s1", model.SuggestionOverride{Title: &newTitle})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "覆寫標題" {
		t.Errorf("title = %q, want 覆寫標題 (override applied)", updated.Title)
	}
	if !linkedActress || !linkedTag {
		t.Errorf("actress linked=%v tag linked=%v", linkedActress, linkedTag)
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
