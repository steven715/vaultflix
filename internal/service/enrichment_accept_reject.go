package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/steven/vaultflix/internal/model"
)

// AcceptSuggestion applies the staged suggestion's metadata to the video,
// upserts actresses and links them, gets-or-creates genre tags and links them,
// then deletes the suggestion. override fields take precedence over suggestion payload.
// Returns model.ErrNotFound if the suggestion does not exist or belongs to a different video.
func (s *EnrichmentService) AcceptSuggestion(ctx context.Context, videoID, suggestionID string, override model.SuggestionOverride) error {
	sug, err := s.suggestionRepo.GetByID(ctx, suggestionID)
	if err != nil {
		return fmt.Errorf("get suggestion %s: %w", suggestionID, err)
	}
	if sug.VideoID != videoID {
		return fmt.Errorf("suggestion %s does not belong to video %s: %w", suggestionID, videoID, model.ErrNotFound)
	}

	title := sug.Payload.Title
	if override.Title != nil {
		title = *override.Title
	}
	genres := sug.Payload.Genres
	if override.Genres != nil {
		genres = override.Genres
	}

	if err := s.applyVideoMetadata(ctx, videoID, title, sug.Payload); err != nil {
		return err
	}
	if err := s.linkActresses(ctx, videoID, sug.Payload.Actresses); err != nil {
		return err
	}
	if err := s.linkGenres(ctx, videoID, genres); err != nil {
		return err
	}
	if err := s.suggestionRepo.Delete(ctx, suggestionID); err != nil {
		return fmt.Errorf("delete suggestion %s: %w", suggestionID, err)
	}
	return nil
}

// applyVideoMetadata updates the video's scalar metadata fields.
func (s *EnrichmentService) applyVideoMetadata(ctx context.Context, videoID, title string, payload model.EnrichedMetadata) error {
	upd := model.VideoMetadataUpdate{
		Code:           payload.Code,
		Title:          title,
		ReleaseDate:    payload.ReleaseDate,
		RuntimeMinutes: payload.RuntimeMinutes,
		Maker:          payload.Maker,
		Label:          payload.Label,
		Series:         payload.Series,
		CoverKey:       payload.CoverURL,
	}
	if err := s.videoRepo.UpdateMetadata(ctx, videoID, upd); err != nil {
		return fmt.Errorf("update video %s metadata: %w", videoID, err)
	}
	return nil
}

// linkActresses upserts each actress and links her to the video.
func (s *EnrichmentService) linkActresses(ctx context.Context, videoID string, actresses []model.ActressMeta) error {
	for _, a := range actresses {
		actress := &model.Actress{
			NameJa:     a.NameJa,
			NameRomaji: a.NameRomaji,
			AvatarKey:  a.AvatarURL,
		}
		if err := s.actressRepo.Upsert(ctx, actress); err != nil {
			return fmt.Errorf("upsert actress %q: %w", a.NameJa, err)
		}
		if err := s.actressRepo.AddVideoActress(ctx, videoID, actress.ID); err != nil {
			return fmt.Errorf("link actress %s to video %s: %w", actress.ID, videoID, err)
		}
	}
	return nil
}

// linkGenres gets-or-creates each genre tag and links it to the video.
func (s *EnrichmentService) linkGenres(ctx context.Context, videoID string, genres []string) error {
	for _, name := range genres {
		if name == "" {
			continue
		}
		tag, err := s.tagRepo.GetOrCreateByName(ctx, name, "genre")
		if err != nil {
			return fmt.Errorf("get or create genre tag %q: %w", name, err)
		}
		if err := s.tagRepo.AddVideoTag(ctx, videoID, tag.ID); err != nil {
			return fmt.Errorf("link genre tag %d to video %s: %w", tag.ID, videoID, err)
		}
	}
	return nil
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
