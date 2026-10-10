package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/scraper"
)

// A failed download stages no key: the source URL must never stand in for one.
func TestEnrichVideo_FailedImageDownloadStagesNoKey(t *testing.T) {
	videoRepo := &mock.VideoRepository{
		GetByIDFunc: func(_ context.Context, id string) (*model.Video, error) {
			return &model.Video{ID: id, OriginalFilename: "DASD-626.mp4"}, nil
		},
		SetEnrichmentStatusFunc: func(context.Context, string, string) error { return nil },
	}
	fakeScraper := &mock.Scraper{
		SourceValue: "javbus",
		ScrapeByCodeFunc: func(_ context.Context, code string) (*model.EnrichedMetadata, error) {
			return &model.EnrichedMetadata{
				Code: code, Title: "T", CoverURL: "https://example.com/cover.jpg",
				Actresses: []model.ActressMeta{{NameJa: "山田 花子", AvatarURL: "https://example.com/a.jpg"}},
			}, nil
		},
	}
	var staged *model.MetadataSuggestion
	sugRepo := &mock.SuggestionRepository{CreateFunc: func(_ context.Context, s *model.MetadataSuggestion) error { staged = s; return nil }}
	svc := NewEnrichmentService([]scraper.MetadataScraper{fakeScraper}, videoRepo, &mock.ActressRepository{}, sugRepo, &mock.TagRepository{}, &mock.MinIOClient{}, &mock.Notifier{})
	svc.downloadImage = func(context.Context, string) (string, error) { return "", errors.New("404") }

	if err := svc.EnrichVideo(context.Background(), "v1", "u1"); err != nil {
		t.Fatal(err)
	}
	p := staged.Payload
	if p.CoverKey != "" || p.Actresses[0].AvatarKey != "" {
		t.Errorf("keys staged for failed downloads: cover %q avatar %q", p.CoverKey, p.Actresses[0].AvatarKey)
	}
	if p.CoverURL != "https://example.com/cover.jpg" || p.Actresses[0].AvatarURL != "https://example.com/a.jpg" {
		t.Errorf("source URLs changed: %+v", p)
	}
}

func TestSuggestionApplication_ObjectKeys(t *testing.T) {
	tests := []struct {
		name                 string
		coverURL, coverKey   string
		avatarURL, avatarKey string
		wantCover            string
		wantAvatar           string
	}{
		{"keys win", "https://x/c.jpg", "covers/c.jpg", "https://x/a.jpg", "actresses/a.jpg", "covers/c.jpg", "actresses/a.jpg"},
		{"failed download: source URL is never a key", "https://x/c.jpg", "", "http://x/a.jpg", "", "", ""},
		{"upper-case scheme is still a URL", "HTTPS://x/c.jpg", "", "Http://x/a.jpg", "", "", ""},
		{"data URI placeholder is not a key", "data:image/gif;base64,R0lGOD", "", "data:image/png;base64,iVBOR", "", "", ""},
		{"relative scraped path is not a key", "img/cover.jpg", "", "pics/a.jpg", "", "", ""},
		// Suggestions staged before cover_key/avatar_key existed kept the key in the URL field.
		{"legacy row with key in URL field", "covers/c.jpg", "", "actresses/a.jpg", "", "covers/c.jpg", "actresses/a.jpg"},
		{"legacy prefixes are per kind", "actresses/a.jpg", "", "covers/c.jpg", "", "", ""},
		{"nothing scraped", "", "", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sug := &model.MetadataSuggestion{ID: "s1", VideoID: "v1", Payload: model.EnrichedMetadata{
				CoverURL: tt.coverURL, CoverKey: tt.coverKey,
				Actresses: []model.ActressMeta{{NameJa: "A", AvatarURL: tt.avatarURL, AvatarKey: tt.avatarKey}},
			}}
			app := suggestionApplication(sug, model.SuggestionOverride{})
			if app.Metadata.CoverKey != tt.wantCover || app.Performers[0].AvatarKey != tt.wantAvatar {
				t.Errorf("cover %q avatar %q, want %q / %q", app.Metadata.CoverKey, app.Performers[0].AvatarKey, tt.wantCover, tt.wantAvatar)
			}
		})
	}
}
