package model

import "time"

// VideoSummary is a lightweight video representation for list views (favorites, recommendations).
type VideoSummary struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	ThumbnailKey    string `json:"-"`
	DurationSeconds int    `json:"duration_seconds"`
	Resolution      string `json:"resolution"`
	FileSizeBytes   int64  `json:"file_size_bytes"`
	CreatedAt       time.Time `json:"created_at"`
}

// VideoSummaryWithURL is the service-layer response with presigned thumbnail URL.
type VideoSummaryWithURL struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	ThumbnailURL    string    `json:"thumbnail_url,omitempty"`
	DurationSeconds int       `json:"duration_seconds"`
	Resolution      string    `json:"resolution"`
	FileSizeBytes   int64     `json:"file_size_bytes"`
	CreatedAt       time.Time `json:"created_at"`
}
