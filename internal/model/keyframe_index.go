package model

import "time"

// SegmentBoundary describes a time range (in seconds) of an HLS segment in the original file.
// Start must be a keyframe pts (first segment is 0), Duration varies based on keyframe distribution.
type SegmentBoundary struct {
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
}

// KeyframeIndex is a keyframe-aligned segment boundary table for a video.
type KeyframeIndex struct {
	VideoID  string
	Segments []SegmentBoundary
	ProbedAt time.Time
}
