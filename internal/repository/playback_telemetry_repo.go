package repository

import (
	"context"

	"github.com/steven/vaultflix/internal/model"
)

// PlaybackTelemetryRepository persists per-session playback quality summaries
// and reads windowed aggregates over them.
type PlaybackTelemetryRepository interface {
	// Insert upserts one session summary keyed by session_id (last write wins),
	// so a page-leave beacon and an unmount POST for the same session collapse
	// to one row. It returns model.ErrNotFound when the referenced video does
	// not exist (no row is written).
	Insert(ctx context.Context, in model.PlaybackTelemetryInput) error
	// Aggregate returns per-play_mode metrics within the trailing q.Days window,
	// optionally filtered to q.Scope. An empty window yields an empty slice and
	// a nil error.
	Aggregate(ctx context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error)
}
