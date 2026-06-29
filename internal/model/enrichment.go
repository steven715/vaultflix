package model

import "time"

// EnrichBatchRequest configures a batch enrichment run.
type EnrichBatchRequest struct {
	Status     string `json:"status"`      // which enrichment_status to process; "" defaults to pending
	AutoAccept bool   `json:"auto_accept"` // if true, auto-apply the highest-priority suggestion per video (no staging review)
	Limit      int    `json:"limit"`       // cap number of videos processed this run; 0 = no cap (process all)
}

// EnrichJob represents the state of an in-progress or completed batch
// enrichment run (in-memory; not persisted). Only one job runs per process;
// concurrent StartBatchAsync calls return model.ErrConflict.
// Phase 1 omits an Errors slice — the Failed counter suffices; per-video
// errors are logged with slog.Warn and visible in progress WS messages.
type EnrichJob struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"` // running | completed | cancelled | failed
	Total          int        `json:"total"`
	Processed      int        `json:"processed"`
	Succeeded      int        `json:"succeeded"`
	Failed         int        `json:"failed"`
	CurrentVideoID string     `json:"current_video_id,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

// EnrichedMetadata 是單一來源 scrape 出的 canonical 結果（也是 suggestion payload 的形狀）。
type EnrichedMetadata struct {
	Code           string        `json:"code"`
	Title          string        `json:"title"`
	ReleaseDate    *time.Time    `json:"release_date,omitempty"`
	RuntimeMinutes int           `json:"runtime_minutes"`
	Maker          string        `json:"maker"`
	Label          string        `json:"label"`
	Series         string        `json:"series"`
	Genres         []string      `json:"genres"`
	Actresses      []ActressMeta `json:"actresses"`
	CoverURL       string        `json:"cover_url"`
}

// ActressMeta 是 scrape 出的女優資料（尚未落表）。
type ActressMeta struct {
	NameJa     string `json:"name_ja"`
	NameRomaji string `json:"name_romaji"`
	AvatarURL  string `json:"avatar_url"`
}

// Actress 對應 actresses 表。
type Actress struct {
	ID         string    `json:"id"`
	NameJa     string    `json:"name_ja"`
	NameRomaji string    `json:"name_romaji"`
	AvatarKey  string    `json:"avatar_key"`
	CreatedAt  time.Time `json:"created_at"`
}

// MetadataSuggestion 對應 metadata_suggestions 表。
type MetadataSuggestion struct {
	ID        string           `json:"id"`
	VideoID   string           `json:"video_id"`
	Source    string           `json:"source"`
	Code      string           `json:"code"`
	Payload   EnrichedMetadata `json:"payload"`
	FetchedAt time.Time        `json:"fetched_at"`
	Status    string           `json:"status"`
}

// VideoMetadataUpdate 是 accept 後要套到 videos 的 scalar 欄位。
type VideoMetadataUpdate struct {
	Code           string
	Title          string
	ReleaseDate    *time.Time
	RuntimeMinutes int
	Maker          string
	Label          string
	Series         string
	CoverKey       string
}

// SuggestionOverride 是 accept 時使用者對欄位的覆寫（空欄沿用 suggestion 原值）。
type SuggestionOverride struct {
	Title  *string  `json:"title,omitempty"`
	Genres []string `json:"genres,omitempty"`
}

// Enrichment 狀態常數。
const (
	EnrichmentNone      = "none"
	EnrichmentPending   = "pending"
	EnrichmentSuggested = "suggested"
	EnrichmentEnriched  = "enriched"
	EnrichmentFailed    = "failed"
	EnrichmentNoCode    = "no_code"
)
