package service

import (
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
	"github.com/steven/vaultflix/internal/websocket"
)

// MediaPipeline is the Media Lane (ADR-0014) with the Jobs that run on it:
// Import Jobs and the four kinds of Backfill Job. They read the same disks,
// run ffmpeg/ffprobe and write the same Videos, so one runs at a time.
type MediaPipeline struct {
	Import   *ImportService
	Backfill *BackfillRunner
}

// MediaPipelineDeps is what the Media Lane's Jobs run on; every field is
// required.
type MediaPipelineDeps struct {
	Videos repository.VideoRepository
	// Files resolves a Video's file on its Media Source (MediaSourceService).
	Files     mediaFileResolver
	Media     mediaDeriver
	Keyframes keyframeIndexer
	Notifier  websocket.Notifier
}

// NewMediaPipeline puts the Import and Backfill Jobs on one Media Lane of
// their own; the only way their Jobs can share it is to come from here.
func NewMediaPipeline(d MediaPipelineDeps) *MediaPipeline {
	lane := NewJobRunner()
	return &MediaPipeline{
		Import: NewImportService(ImportServiceDeps{
			Videos: d.Videos, Media: d.Media, Notifier: d.Notifier, Lane: lane, Keyframes: d.Keyframes,
		}),
		Backfill: NewBackfillRunner(d.Notifier, map[model.BackfillKind]BackfillTask{
			model.BackfillPreview:  NewPreviewBackfill(d.Videos, d.Files, d.Media),
			model.BackfillCodec:    NewCodecBackfill(d.Videos, d.Files, d.Media, d.Keyframes),
			model.BackfillKeyframe: NewKeyframeBackfill(d.Keyframes, d.Videos, d.Files),
			model.BackfillCode:     NewCodeBackfill(d.Videos),
		}, lane),
	}
}
