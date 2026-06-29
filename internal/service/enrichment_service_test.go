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
