package model

import "time"

// DailyRecommendation represents a manually curated recommendation for a specific date.
type DailyRecommendation struct {
	ID             string    `json:"id"`
	VideoID        string    `json:"video_id"`
	RecommendDate  time.Time `json:"recommend_date"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
}

// RecommendationWithVideo is the repository-layer JOIN result
// containing recommendation metadata and video basic info.
type RecommendationWithVideo struct {
	ID              string    `json:"id"`
	VideoID         string    `json:"video_id"`
	RecommendDate   time.Time `json:"recommend_date"`
	SortOrder       int       `json:"sort_order"`
	Title           string    `json:"title"`
	ThumbnailKey    string    `json:"-"`
	DurationSeconds int       `json:"duration_seconds"`
	Resolution      string    `json:"resolution"`
	FileSizeBytes   int64     `json:"file_size_bytes"`
}

// RecommendationItem is the service-layer response with presigned thumbnail URL.
type RecommendationItem struct {
	ID              string `json:"id"`
	VideoID         string `json:"video_id"`
	Title           string `json:"title"`
	ThumbnailURL    string `json:"thumbnail_url,omitempty"`
	DurationSeconds int    `json:"duration_seconds"`
	Resolution      string `json:"resolution"`
	FileSizeBytes   int64  `json:"file_size_bytes"`
	SortOrder       int    `json:"sort_order"`
	IsFallback      bool   `json:"is_fallback"`
}
