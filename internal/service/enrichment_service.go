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
		return fmt.Errorf("enrich %s: all sources failed: %w", code, model.ErrSourceUnavailable)
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
