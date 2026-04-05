package model

import "time"

type WatchHistory struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	VideoID         string    `json:"video_id"`
	ProgressSeconds int       `json:"progress_seconds"`
	Completed       bool      `json:"completed"`
	WatchedAt       time.Time `json:"watched_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// WatchHistoryWithVideo is returned by ListByUser JOIN query.
// Contains video basic info to avoid extra API calls.
type WatchHistoryWithVideo struct {
	ID              string    `json:"id"`
	VideoID         string    `json:"video_id"`
	Title           string    `json:"title"`
	ThumbnailKey    string    `json:"-"`
	DurationSeconds int       `json:"duration_seconds"`
	ProgressSeconds int       `json:"progress_seconds"`
	Completed       bool      `json:"completed"`
	WatchedAt       time.Time `json:"watched_at"`
}

// WatchHistoryItem is the service-layer response with presigned thumbnail URL.
type WatchHistoryItem struct {
	ID              string    `json:"id"`
	VideoID         string    `json:"video_id"`
	Title           string    `json:"title"`
	ThumbnailURL    string    `json:"thumbnail_url,omitempty"`
	DurationSeconds int       `json:"duration_seconds"`
	ProgressSeconds int       `json:"progress_seconds"`
	Completed       bool      `json:"completed"`
	WatchedAt       time.Time `json:"watched_at"`
}
