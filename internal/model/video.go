package model

import "time"

type Video struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	MinIOObjectKey   string     `json:"minio_object_key"`
	ThumbnailKey     string     `json:"thumbnail_key"`
	PreviewKey       string     `json:"preview_key"`
	DurationSeconds  int        `json:"duration_seconds"`
	Resolution       string     `json:"resolution"`
	FileSizeBytes    int64      `json:"file_size_bytes"`
	MimeType         string     `json:"mime_type"`
	VideoCodec       string     `json:"video_codec,omitempty"`
	AudioCodec       string     `json:"audio_codec,omitempty"`
	OriginalFilename string     `json:"original_filename"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	SourceID         *string    `json:"source_id,omitempty"`
	FilePath         *string    `json:"file_path,omitempty"`
	Code             string     `json:"code,omitempty"`
	ReleaseDate      *time.Time `json:"release_date,omitempty"`
	RuntimeMinutes   int        `json:"runtime_minutes,omitempty"`
	Maker            string     `json:"maker,omitempty"`
	Label            string     `json:"label,omitempty"`
	Series           string     `json:"series,omitempty"`
	CoverKey         string     `json:"cover_key,omitempty"`
	EnrichmentStatus string     `json:"enrichment_status"`
	EnrichedAt       *time.Time `json:"enriched_at,omitempty"`
}

type VideoWithTags struct {
	Video
	Tags         []Tag  `json:"tags"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	PreviewURL   string `json:"preview_url,omitempty"`
}

type VideoDetail struct {
	VideoWithTags
	StreamURL     string   `json:"stream_url"`
	PlayMode      PlayMode `json:"play_mode"`
	IsFavorited   bool     `json:"is_favorited"`
	WatchProgress int      `json:"watch_progress"`
}

type UpdateVideoInput struct {
	Title       string
	Description string
}

type VideoFilter struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
	Query     string
	TagIDs    []int
}

// PlayMode 表示前端應如何播放這部影片。
type PlayMode string

const (
	// PlayModeDirect：容器與編碼皆瀏覽器原生相容，直接 http.ServeFile。
	PlayModeDirect PlayMode = "direct"
	// PlayModeRemux：編碼相容、容器不相容，走即時 HLS -c copy。
	PlayModeRemux PlayMode = "remux"
	// PlayModeTranscode：編碼不相容，需真正轉碼（Phase 2 才支援）。
	PlayModeTranscode PlayMode = "transcode"
)
