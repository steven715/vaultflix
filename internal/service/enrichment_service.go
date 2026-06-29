package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
	"github.com/steven/vaultflix/internal/scraper"
	"github.com/steven/vaultflix/internal/scraper/avid"
	"github.com/steven/vaultflix/internal/websocket"
)

// EnrichmentService extracts a video code from a filename, scrapes metadata
// from registered sources, stages per-source MetadataSuggestion rows, uploads
// cover/avatar images to MinIO, and pushes WebSocket notifications.
type EnrichmentService struct {
	scrapers       []scraper.MetadataScraper
	videoRepo      repository.VideoRepository
	actressRepo    repository.ActressRepository
	suggestionRepo repository.SuggestionRepository
	tagRepo        repository.TagRepository
	minioSvc       MinIOClient
	notifier       websocket.Notifier

	// downloadImage downloads a URL to a temp file and returns the path.
	// Overridable in tests to avoid real HTTP. Defaults to defaultDownloadImage.
	downloadImage func(ctx context.Context, url string) (string, error)
}

// NewEnrichmentService constructs an EnrichmentService with real HTTP downloads.
func NewEnrichmentService(
	scrapers []scraper.MetadataScraper,
	videoRepo repository.VideoRepository,
	actressRepo repository.ActressRepository,
	suggestionRepo repository.SuggestionRepository,
	tagRepo repository.TagRepository,
	minioSvc MinIOClient,
	notifier websocket.Notifier,
) *EnrichmentService {
	return &EnrichmentService{
		scrapers:       scrapers,
		videoRepo:      videoRepo,
		actressRepo:    actressRepo,
		suggestionRepo: suggestionRepo,
		tagRepo:        tagRepo,
		minioSvc:       minioSvc,
		notifier:       notifier,
		downloadImage:  defaultDownloadImage,
	}
}

// EnrichVideo is the main entry point: it extracts a code, scrapes all sources,
// stages suggestions, uploads images, updates enrichment status, and sends WS
// notifications. Returns model.ErrCodeNotFound when the filename yields no code,
// or a wrapped error when all sources fail.
func (s *EnrichmentService) EnrichVideo(ctx context.Context, videoID, userID string) error {
	video, err := s.videoRepo.GetByID(ctx, videoID)
	if err != nil {
		return fmt.Errorf("get video %s: %w", videoID, err)
	}

	code, ok := avid.ExtractCode(video.OriginalFilename)
	if !ok {
		return s.handleNoCode(ctx, videoID, userID)
	}

	results, scrapeErr := s.scrapeAll(ctx, videoID, code)
	if len(results) == 0 {
		return s.handleAllFailed(ctx, videoID, userID, code, scrapeErr)
	}

	s.uploadImages(ctx, videoID, code, results)

	if err := s.stageSuggestions(ctx, videoID, code, results); err != nil {
		return err
	}

	return s.handleSuccess(ctx, videoID, userID, code, results)
}

// handleNoCode sets enrichment status to no_code, sends WS error, returns ErrCodeNotFound.
func (s *EnrichmentService) handleNoCode(ctx context.Context, videoID, userID string) error {
	if err := s.videoRepo.SetEnrichmentStatus(ctx, videoID, model.EnrichmentNoCode); err != nil {
		slog.Warn("set enrichment status no_code failed", "video_id", videoID, "error", err)
	}
	s.notifier.SendToUser(userID, &websocket.Message{
		Type:    websocket.TypeEnrichError,
		Payload: map[string]string{"video_id": videoID, "error": "no code in filename"},
	})
	return fmt.Errorf("enrich %s: %w", videoID, model.ErrCodeNotFound)
}

// scrapeAll calls each scraper, collects successful results, logs individual
// failures. Returns the collected results and the last error seen (if any).
func (s *EnrichmentService) scrapeAll(ctx context.Context, videoID, code string) ([]scraper.SourceResult, error) {
	var results []scraper.SourceResult
	var lastErr error
	for _, sc := range s.scrapers {
		data, err := sc.ScrapeByCode(ctx, code)
		if err != nil {
			slog.Warn("scrape source failed",
				"video_id", videoID,
				"code", code,
				"source", sc.Source(),
				"error", err,
			)
			lastErr = err
			continue
		}
		results = append(results, scraper.SourceResult{Source: sc.Source(), Data: data})
	}
	return results, lastErr
}

// handleAllFailed sets enrichment status to failed, sends WS error.
func (s *EnrichmentService) handleAllFailed(ctx context.Context, videoID, userID, code string, scrapeErr error) error {
	if err := s.videoRepo.SetEnrichmentStatus(ctx, videoID, model.EnrichmentFailed); err != nil {
		slog.Warn("set enrichment status failed", "video_id", videoID, "error", err)
	}
	s.notifier.SendToUser(userID, &websocket.Message{
		Type:    websocket.TypeEnrichError,
		Payload: map[string]string{"video_id": videoID, "code": code, "error": "all sources failed"},
	})
	if scrapeErr != nil {
		return fmt.Errorf("enrich %s: all sources failed (%v): %w", code, scrapeErr, model.ErrSourceUnavailable)
	}
	return fmt.Errorf("enrich %s: no scrapers configured", code)
}

// uploadImages downloads and uploads cover/avatar images for each source result,
// swapping CoverURL/AvatarURL to MinIO keys in-place.
// Empty URLs are skipped; failures are logged and the URL is left as-is.
func (s *EnrichmentService) uploadImages(ctx context.Context, videoID, code string, results []scraper.SourceResult) {
	for i := range results {
		res := results[i].Data
		source := results[i].Source
		if res == nil {
			continue
		}
		if res.CoverURL != "" {
			key := fmt.Sprintf("covers/%s-%s.jpg", code, source)
			if coverKey, ok := s.downloadAndUploadCover(ctx, videoID, res.CoverURL, key); ok {
				res.CoverURL = coverKey
			}
		}
		for j := range res.Actresses {
			a := &res.Actresses[j]
			if a.AvatarURL != "" {
				key := fmt.Sprintf("actresses/%s.jpg", sanitizeName(a.NameJa))
				if avatarKey, ok := s.downloadAndUploadAvatar(ctx, videoID, a.AvatarURL, key); ok {
					a.AvatarURL = avatarKey
				}
			}
		}
	}
}

// downloadAndUploadCover downloads url, uploads it as a cover, returns (key, true) on success.
func (s *EnrichmentService) downloadAndUploadCover(ctx context.Context, videoID, url, key string) (string, bool) {
	tmp, err := s.downloadImage(ctx, url)
	if err != nil {
		slog.Warn("download cover failed", "video_id", videoID, "url", url, "error", err)
		return "", false
	}
	defer os.Remove(tmp)
	if err := s.minioSvc.UploadCover(ctx, key, tmp); err != nil {
		slog.Warn("upload cover failed", "video_id", videoID, "key", key, "error", err)
		return "", false
	}
	return key, true
}

// downloadAndUploadAvatar downloads url, uploads it as an actress avatar, returns (key, true) on success.
func (s *EnrichmentService) downloadAndUploadAvatar(ctx context.Context, videoID, url, key string) (string, bool) {
	tmp, err := s.downloadImage(ctx, url)
	if err != nil {
		slog.Warn("download avatar failed", "video_id", videoID, "url", url, "error", err)
		return "", false
	}
	defer os.Remove(tmp)
	if err := s.minioSvc.UploadActressAvatar(ctx, key, tmp); err != nil {
		slog.Warn("upload avatar failed", "video_id", videoID, "key", key, "error", err)
		return "", false
	}
	return key, true
}

// stageSuggestions creates one MetadataSuggestion row per source result.
func (s *EnrichmentService) stageSuggestions(ctx context.Context, videoID, code string, results []scraper.SourceResult) error {
	for _, r := range results {
		if r.Data == nil {
			continue
		}
		sug := &model.MetadataSuggestion{
			VideoID: videoID,
			Source:  r.Source,
			Code:    code,
			Payload: *r.Data,
			Status:  "pending",
		}
		if err := s.suggestionRepo.Create(ctx, sug); err != nil {
			return fmt.Errorf("create suggestion for %s source %s: %w", code, r.Source, err)
		}
	}
	return nil
}

// handleSuccess sets enrichment status to suggested, sends WS complete with merged preview.
func (s *EnrichmentService) handleSuccess(ctx context.Context, videoID, userID, code string, results []scraper.SourceResult) error {
	if err := s.videoRepo.SetEnrichmentStatus(ctx, videoID, model.EnrichmentSuggested); err != nil {
		slog.Warn("set enrichment status suggested failed", "video_id", videoID, "error", err)
	}
	merged := scraper.MergeByPriority(results)
	s.notifier.SendToUser(userID, &websocket.Message{
		Type:    websocket.TypeEnrichComplete,
		Payload: map[string]interface{}{"video_id": videoID, "code": code, "preview": merged},
	})
	slog.Info("video enriched",
		"video_id", videoID,
		"code", code,
		"sources", len(results),
		"status", model.EnrichmentSuggested,
	)
	return nil
}

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

// sanitizeName replaces characters that are unsafe in object key paths with underscores.
func sanitizeName(name string) string {
	r := strings.NewReplacer("/", "_", " ", "_", ".", "_")
	return r.Replace(name)
}

// defaultDownloadImage performs a real HTTP GET and writes the body to a temp file.
// The caller is responsible for removing the returned path.
func defaultDownloadImage(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp("", "vaultflix-img-*.jpg")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmp.Close()
	return tmp.Name(), nil
}
