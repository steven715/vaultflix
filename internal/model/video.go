package model

import "time"

type Video struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	MinIOObjectKey   string    `json:"minio_object_key"`
	ThumbnailKey     string    `json:"thumbnail_key"`
	DurationSeconds  int       `json:"duration_seconds"`
	Resolution       string    `json:"resolution"`
	FileSizeBytes    int64     `json:"file_size_bytes"`
	MimeType         string    `json:"mime_type"`
	OriginalFilename string    `json:"original_filename"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	SourceID         *string   `json:"source_id,omitempty"`
	FilePath         *string   `json:"file_path,omitempty"`
}

type VideoWithTags struct {
	Video
	Tags         []Tag  `json:"tags"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

type VideoDetail struct {
	VideoWithTags
	StreamURL     string `json:"stream_url"`
	IsFavorited   bool   `json:"is_favorited"`
	WatchProgress int    `json:"watch_progress"`
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
