package model

import "time"

// WatchSession is one continuous viewing of a video, accumulated via heartbeats.
type WatchSession struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	VideoID              string    `json:"video_id"`
	WatchedSeconds       int       `json:"watched_seconds"`
	MaxProgressSeconds   int       `json:"max_progress_seconds"`
	VideoDurationSeconds int       `json:"video_duration_seconds"`
	StartedAt            time.Time `json:"started_at"`
	LastHeartbeatAt      time.Time `json:"last_heartbeat_at"`
}

// HeartbeatInput is one heartbeat's accumulated playback delta for a session.
type HeartbeatInput struct {
	SessionID       string
	UserID          string
	VideoID         string
	PlayedDelta     int
	PositionSeconds int
}
