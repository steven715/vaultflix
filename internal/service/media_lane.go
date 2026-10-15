package service

import (
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
	"github.com/steven/vaultflix/internal/websocket"
)

// MediaLane is the Media Lane (GLOSSARY.md「Job Lane」, ADR-0014) with the
// Jobs that run on it: Import Jobs and the four kinds of Backfill Job. They
// read the same disks, run ffmpeg/ffprobe and write the same Videos, so one
// runs at a time.
type MediaLane struct {
	Import   *ImportService
	Backfill *BackfillRunner
}

// MediaLaneDeps is what the Media Lane's Jobs run on; every field is required.
type MediaLaneDeps struct {
	Videos repository.VideoRepository
	// Sources looks up Media Sources and resolves a Video's file on its own
	// (MediaSourceService).
	Sources mediaSources
	// Media derives Media Info, Thumbnails and Previews (MediaProcessor).
	Media mediaDeriver
	// Keyframes builds Keyframe Indexes (KeyframeService).
	Keyframes keyframeIndexer
	Notifier  websocket.Notifier
}

// mediaSources is what the Media Lane's Jobs need of Media Sources
// (implemented by *MediaSourceService).
type mediaSources interface {
	mediaSourceReader
	mediaFileResolver
}

// NewMediaLane puts the Import and Backfill Jobs on one lane of their own;
// the only way their Jobs share it is to come from here. Panics when a
// dependency is missing: a wiring mistake that must stop the server at
// startup.
func NewMediaLane(d MediaLaneDeps) *MediaLane {
	if d.Sources == nil {
		panic("service.NewMediaLane: missing Sources")
	}
	lane := NewJobRunner()
	return &MediaLane{
		Import: NewImportService(ImportServiceDeps{
			Videos: d.Videos, Sources: d.Sources, Media: d.Media, Notifier: d.Notifier, Lane: lane, Keyframes: d.Keyframes,
		}),
		Backfill: NewBackfillRunner(d.Notifier, map[model.BackfillKind]BackfillTask{
			model.BackfillPreview:  NewPreviewBackfill(d.Videos, d.Sources, d.Media),
			model.BackfillCodec:    NewCodecBackfill(d.Videos, d.Sources, d.Media, d.Keyframes),
			model.BackfillKeyframe: NewKeyframeBackfill(d.Keyframes, d.Videos, d.Sources),
			model.BackfillCode:     NewCodeBackfill(d.Videos),
		}, lane),
	}
}
