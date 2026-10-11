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

type fileResult struct {
	Status string // "success", "skipped", "error"
	Error  string
}

type ImportService struct {
	videoRepo repository.VideoRepository
	media     mediaDeriver
	notifier  websocket.Notifier
	jobs      *JobRunner     // runs Import Jobs, one at a time
	keyframes keyframeProber // optional;nil 時不觸發
}

func NewImportService(videoRepo repository.VideoRepository, media mediaDeriver, notifier websocket.Notifier) *ImportService {
	return &ImportService{
		videoRepo: videoRepo,
		media:     media,
		notifier:  notifier,
		jobs:      NewJobRunner(),
	}
}

// keyframeProber 觸發影片的非同步 keyframe 探測(由 *KeyframeService 實作)。
type keyframeProber interface {
	TriggerProbe(videoID, absPath string)
}

// SetKeyframeProber 注入 keyframe prober(比照 VideoService.SetUserServices 的後注入模式)。
func (s *ImportService) SetKeyframeProber(p keyframeProber) {
	s.keyframes = p
}

// jobKindImport tags Import Jobs on the JobRunner.
const jobKindImport = "import"

// StartAsync launches an Import Job for source in the background and returns
// a snapshot of it. Only one Import Job runs at a time; a second call returns
// model.ErrConflict.
func (s *ImportService) StartAsync(ctx context.Context, source *model.MediaSource, userID string) (*model.ImportJob, error) {
	job, err := s.jobs.Start(jobKindImport, source, importItems{svc: s, source: source},
		&importReport{notifier: s.notifier, userID: userID})
	if err != nil {
		return nil, fmt.Errorf("start import of media source %s: %w", source.ID, err)
	}
	return toImportJob(job), nil
}

// GetJob returns a snapshot of the latest Import Job if its ID is jobID.
// Returns model.ErrNotFound otherwise.
func (s *ImportService) GetJob(jobID string) (*model.ImportJob, error) {
	job := s.jobs.Latest()
	if job == nil || job.Kind != jobKindImport || job.ID != jobID {
		return nil, model.ErrNotFound
	}
	return toImportJob(job), nil
}

// GetActiveJob returns a snapshot of the running Import Job, or nil when none
// is running.
func (s *ImportService) GetActiveJob() *model.ImportJob {
	job := s.jobs.Latest()
	if job == nil || job.Kind != jobKindImport || job.Status != "running" {
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
