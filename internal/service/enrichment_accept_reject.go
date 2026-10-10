package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/steven/vaultflix/internal/model"
)

// AcceptSuggestion applies a staged Metadata Suggestion to its Video in one
// transaction: the Video's Metadata, its Performers, its genre Tags, and the
// Suggestion's deletion all happen or none do. override fields take precedence
// over the Suggestion's payload.
// Returns model.ErrNotFound (wrapped) if the Suggestion does not exist or
// belongs to a different Video.
func (s *EnrichmentService) AcceptSuggestion(ctx context.Context, videoID, suggestionID string, override model.SuggestionOverride) error {
	sug, err := s.suggestionRepo.GetByID(ctx, suggestionID)
	if err != nil {
		return fmt.Errorf("get suggestion %s: %w", suggestionID, err)
	}
	if sug.VideoID != videoID {
		return fmt.Errorf("suggestion %s does not belong to video %s: %w", suggestionID, videoID, model.ErrNotFound)
	}
	if err := s.suggestionRepo.Apply(ctx, suggestionApplication(sug, override)); err != nil {
		return fmt.Errorf("accept suggestion %s: %w", suggestionID, err)
	}
	return nil
}

// suggestionApplication turns a Suggestion plus the user's overrides into the
// writes accepting it makes.
func suggestionApplication(sug *model.MetadataSuggestion, override model.SuggestionOverride) model.SuggestionApplication {
	p := sug.Payload
	title := p.Title
	if override.Title != nil {
		title = *override.Title
	}
	genres := p.Genres
	if override.Genres != nil {
		genres = override.Genres
	}
	app := model.SuggestionApplication{
		SuggestionID: sug.ID,
		VideoID:      sug.VideoID,
		Metadata: model.VideoMetadataUpdate{
			Code: p.Code, Title: title, ReleaseDate: p.ReleaseDate, RuntimeMinutes: p.RuntimeMinutes,
			Maker: p.Maker, Label: p.Label, Series: p.Series, CoverKey: p.CoverURL,
		},
	}
	for _, a := range p.Actresses {
		app.Performers = append(app.Performers, model.Actress{NameJa: a.NameJa, NameRomaji: a.NameRomaji, AvatarKey: a.AvatarURL})
	}
	for _, g := range genres {
		if g != "" {
			app.Genres = append(app.Genres, g)
		}
	}
	return app
}

// autoAcceptHighestPriority accepts the suggestion from the highest-priority
// source available for the video, with no field overrides. Priority is
// determined by the order of s.scrapers (index 0 = highest). If no scraper
// matches a suggestion's source, falls back to the first suggestion.
// Returns model.ErrNotFound (wrapped) when the video has no suggestions.
// Used by the batch auto-accept path.
func (s *EnrichmentService) autoAcceptHighestPriority(ctx context.Context, videoID, userID string) error {
	sugs, err := s.ListSuggestions(ctx, videoID)
	if err != nil {
		return fmt.Errorf("auto-accept list suggestions for %s: %w", videoID, err)
	}
	if len(sugs) == 0 {
		return fmt.Errorf("auto-accept video %s: %w", videoID, model.ErrNotFound)
	}

	// Pick the suggestion whose Source matches the highest-priority scraper.
	var picked *model.MetadataSuggestion
	for _, sc := range s.scrapers {
		for i := range sugs {
			if sugs[i].Source == sc.Source() {
				picked = &sugs[i]
				break
			}
		}
		if picked != nil {
			break
		}
	}
	// Fall back to the first suggestion if no scraper-source match found.
	if picked == nil {
		picked = &sugs[0]
	}

	return s.AcceptSuggestion(ctx, videoID, picked.ID, model.SuggestionOverride{})
}

// ListSuggestions returns all staged MetadataSuggestion rows for a given video.
// Returns an empty slice (not ErrNotFound) when the video has no pending suggestions.
func (s *EnrichmentService) ListSuggestions(ctx context.Context, videoID string) ([]model.MetadataSuggestion, error) {
	suggestions, err := s.suggestionRepo.GetByVideoID(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("list suggestions for video %s: %w", videoID, err)
	}
	return suggestions, nil
}

// RejectSuggestion deletes a staged suggestion. If the video has no remaining
// suggestions after the deletion, resets the enrichment status to none so the
// video can be re-enriched.
// Returns model.ErrNotFound if the suggestion does not exist or belongs to a different video.
func (s *EnrichmentService) RejectSuggestion(ctx context.Context, videoID, suggestionID string) error {
	sug, err := s.suggestionRepo.GetByID(ctx, suggestionID)
	if err != nil {
		return fmt.Errorf("get suggestion %s: %w", suggestionID, err)
	}
	if sug.VideoID != videoID {
		return fmt.Errorf("suggestion %s does not belong to video %s: %w", suggestionID, videoID, model.ErrNotFound)
	}

	if err := s.suggestionRepo.Delete(ctx, suggestionID); err != nil {
		return fmt.Errorf("delete suggestion %s: %w", suggestionID, err)
	}

	remaining, err := s.suggestionRepo.GetByVideoID(ctx, videoID)
	if err != nil {
		return fmt.Errorf("get suggestions for video %s: %w", videoID, err)
	}
	if len(remaining) == 0 {
		if err := s.videoRepo.SetEnrichmentStatus(ctx, videoID, model.EnrichmentNone); err != nil {
			slog.Warn("reset enrichment status to none failed", "video_id", videoID, "error", err)
		}
	}
	return nil
}
