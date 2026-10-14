package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
	"github.com/steven/vaultflix/internal/websocket"
)

func isSupportedExtension(ext string) bool {
	switch ext {
	case ".mp4", ".mkv", ".avi", ".wmv", ".mov":
		return true
	}
	return false
}

type ImportService struct {
	videoRepo repository.VideoRepository
	sources   mediaSourceReader
	media     mediaDeriver
	notifier  websocket.Notifier
	jobs      jobLane
	keyframes keyframeIndexer
}

// ImportServiceDeps is what an ImportService runs on; every field is required.
type ImportServiceDeps struct {
	Videos repository.VideoRepository
	// Sources looks up the Media Source an Import reads (MediaSourceService).
	Sources  mediaSourceReader
	Media    mediaDeriver
	Notifier websocket.Notifier
	// Lane runs the Import Jobs; in production the Media Lane shared with
	// Backfill Jobs (see NewMediaLane).
	Lane jobLane
	// Keyframes builds the Keyframe Index of each new remux Video.
	Keyframes keyframeIndexer
}

// NewImportService creates an ImportService. Production code gets one from
// NewMediaLane. Panics when a dependency is missing: a wiring mistake that
// must stop the server at startup, not crash it in the middle of a Job.
func NewImportService(d ImportServiceDeps) *ImportService {
	if m := d.missing(); m != "" {
		panic("service.NewImportService: missing " + m)
	}
	return &ImportService{
		videoRepo: d.Videos,
		sources:   d.Sources,
		media:     d.Media,
		notifier:  d.Notifier,
		jobs:      d.Lane,
		keyframes: d.Keyframes,
	}
}

func (d ImportServiceDeps) missing() string {
	switch {
	case d.Videos == nil:
		return "Videos"
	case d.Sources == nil:
		return "Sources"
	case d.Media == nil:
		return "Media"
	case d.Notifier == nil:
		return "Notifier"
	case d.Lane == nil:
		return "Lane"
	case d.Keyframes == nil:
		return "Keyframes"
	}
	return ""
}

// mediaSourceReader looks up Media Sources (implemented by *MediaSourceService).
type mediaSourceReader interface {
	// GetByID returns model.ErrNotFound (wrapped) for an unknown Media Source.
	GetByID(ctx context.Context, id string) (*model.MediaSource, error)
}

// JobKindImport tags Import Jobs on the JobRunner.
const JobKindImport = "import"

// StartAsync launches an Import Job for the Media Source sourceID in the
// background and returns a snapshot of it. Returns model.ErrNotFound (wrapped)
// for an unknown Media Source, model.ErrMediaSourceDisabled (wrapped) for a
// disabled one, and model.ErrConflict (wrapped) while the lane is busy.
func (s *ImportService) StartAsync(ctx context.Context, sourceID, userID string) (*model.ImportJob, error) {
	source, err := s.sources.GetByID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("start import of media source %s: %w", sourceID, err)
	}
	if !source.Enabled {
		return nil, fmt.Errorf("start import of media source %s: %w", sourceID, model.ErrMediaSourceDisabled)
	}
	job, err := s.jobs.Start(JobSpec{
		Kind: JobKindImport, Meta: source, Task: &importItems{svc: s, source: source},
		Reporter: &importReport{notifier: s.notifier, userID: userID},
	})
	if err != nil {
		return nil, fmt.Errorf("start import of media source %s: %w", sourceID, err)
	}
	return toImportJob(job), nil
}

// GetJob returns a snapshot of the latest Import Job if its ID is jobID.
// Returns model.ErrNotFound otherwise.
func (s *ImportService) GetJob(jobID string) (*model.ImportJob, error) {
	job := s.jobs.Latest()
	if job == nil || job.Kind != JobKindImport || job.ID != jobID {
		return nil, model.ErrNotFound
	}
	return toImportJob(job), nil
}

// Cancel asks the running Import Job jobID to stop after its current file;
// the files already imported stay. Cancelling a finished Job is a no-op.
// Returns model.ErrNotFound when jobID is not the latest Import Job.
func (s *ImportService) Cancel(ctx context.Context, jobID string) error {
	if _, err := s.GetJob(jobID); err != nil {
		return fmt.Errorf("cancel import job %s: %w", jobID, err)
	}
	if err := s.jobs.Cancel(jobID); err != nil {
		return fmt.Errorf("cancel import job %s: %w", jobID, err)
	}
	return nil
}

// GetActiveJob returns a snapshot of the running Import Job, or nil when none
// is running.
func (s *ImportService) GetActiveJob() *model.ImportJob {
	job := s.jobs.Latest()
	if job == nil || job.Kind != JobKindImport || job.Status != "running" {
		return nil
	}
	return toImportJob(job)
}

func (s *ImportService) scanVideoFiles(sourceDir string) ([]string, error) {
	var files []string

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to access path %s: %w", path, err)
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if isSupportedExtension(ext) {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", sourceDir, err)
	}

	return files, nil
}
