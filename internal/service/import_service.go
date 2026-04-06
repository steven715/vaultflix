package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
	"github.com/steven/vaultflix/internal/websocket"
)

var supportedExtensions = map[string]bool{
	".mp4": true,
	".mkv": true,
	".avi": true,
	".wmv": true,
	".mov": true,
}

type fileResult struct {
	Status string // "success", "skipped", "error"
	Error  string
}

type ImportService struct {
	videoRepo  repository.VideoRepository
	minioSvc   MinIOClient
	notifier   websocket.Notifier
	activeJobs sync.Map
	importMu   sync.Mutex
}

func NewImportService(videoRepo repository.VideoRepository, minioSvc MinIOClient, notifier websocket.Notifier) *ImportService {
	return &ImportService{
		videoRepo: videoRepo,
		minioSvc:  minioSvc,
		notifier:  notifier,
	}
}

// StartAsync builds a job and launches a background import, returning job info immediately.
// Only one import job may run at a time; duplicate calls return model.ErrConflict.
func (s *ImportService) StartAsync(ctx context.Context, source *model.MediaSource, userID string) (*model.ImportJob, error) {
	if !s.importMu.TryLock() {
		return nil, model.ErrConflict
	}

	job := &model.ImportJob{
		ID:          uuid.New().String(),
		SourceID:    source.ID,
		SourceLabel: source.Label,
		Status:      "running",
		Errors:      []model.ImportError{},
		StartedAt:   time.Now(),
	}
	s.activeJobs.Store(job.ID, job)

	go func() {
		defer s.importMu.Unlock()
		s.runImport(context.Background(), job, source, userID)
	}()

	return job, nil
}

func (s *ImportService) runImport(ctx context.Context, job *model.ImportJob, source *model.MediaSource, userID string) {
	defer func() {
		now := time.Now()
		job.FinishedAt = &now
		if job.Failed > 0 && job.Imported == 0 {
			job.Status = "failed"
		} else {
			job.Status = "completed"
		}
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeImportComplete,
			Payload: job,
		})
	}()

	files, err := s.scanVideoFiles(source.MountPath)
	if err != nil {
		job.Status = "failed"
		job.Errors = append(job.Errors, model.ImportError{
			FileName: source.MountPath,
			Error:    err.Error(),
		})
		s.notifier.SendToUser(userID, &websocket.Message{
			Type:    websocket.TypeImportError,
			Payload: map[string]string{"job_id": job.ID, "error": err.Error()},
		})
		return
	}

	job.Total = len(files)

	for i, filePath := range files {
		fileName := filepath.Base(filePath)

		s.notifier.SendToUser(userID, &websocket.Message{
			Type: websocket.TypeImportProgress,
			Payload: model.ImportProgress{
				JobID:    job.ID,
				FileName: fileName,
				Current:  i + 1,
				Total:    job.Total,
				Status:   "processing",
			},
		})

		result := s.processOneFile(ctx, source, filePath)

		job.Processed = i + 1
		switch result.Status {
		case "success":
			job.Imported++
		case "skipped":
			job.Skipped++
		case "error":
			job.Failed++
			job.Errors = append(job.Errors, model.ImportError{
				FileName: fileName,
				Error:    result.Error,
			})
		}

		s.notifier.SendToUser(userID, &websocket.Message{
			Type: websocket.TypeImportProgress,
			Payload: model.ImportProgress{
				JobID:    job.ID,
				FileName: fileName,
				Current:  i + 1,
				Total:    job.Total,
				Status:   result.Status,
				Error:    result.Error,
			},
		})
	}

	slog.Info("import completed",
		"source_id", source.ID,
		"source_label", source.Label,
		"total_scanned", job.Total,
		"imported", job.Imported,
		"skipped", job.Skipped,
		"failed", job.Failed,
	)
}

func (s *ImportService) processOneFile(ctx context.Context, source *model.MediaSource, filePath string) fileResult {
	filename := filepath.Base(filePath)

	stat, err := os.Stat(filePath)
	if err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to stat file %s: %v", filename, err)}
	}
	fileSize := stat.Size()

	relPath, err := filepath.Rel(source.MountPath, filePath)
	if err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to calculate relative path for %s: %v", filename, err)}
	}
	relPath = filepath.ToSlash(relPath)

	_, err = s.videoRepo.FindBySourceAndPath(ctx, source.ID, relPath)
	if err == nil {
		slog.Info("video skipped, already imported",
			"file", filename,
			"source_id", source.ID,
			"file_path", relPath,
		)
		return fileResult{Status: "skipped"}
	}
	if !errors.Is(err, model.ErrNotFound) {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to check duplicate for %s: %v", filename, err)}
	}

	metadata, err := s.probeMetadata(ctx, filePath)
	if err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to probe metadata for %s: %v", filename, err)}
	}

	videoID := uuid.New().String()

	thumbnailPath, err := s.generateThumbnail(ctx, filePath, metadata.durationSeconds)
	if err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to generate thumbnail for %s: %v", filename, err)}
	}
	defer os.Remove(thumbnailPath)

	thumbnailObjectKey := fmt.Sprintf("thumbnails/%s.jpg", videoID)

	if err := s.minioSvc.UploadThumbnail(ctx, thumbnailObjectKey, thumbnailPath); err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to upload thumbnail for %s: %v", filename, err)}
	}

	title := strings.TrimSuffix(filename, filepath.Ext(filename))

	video := &model.Video{
		ID:               videoID,
		Title:            title,
		Description:      "",
		MinIOObjectKey:   "",
		ThumbnailKey:     thumbnailObjectKey,
		DurationSeconds:  metadata.durationSeconds,
		Resolution:       metadata.resolution,
		FileSizeBytes:    fileSize,
		MimeType:         metadata.mimeType,
		OriginalFilename: filename,
		SourceID:         &source.ID,
		FilePath:         &relPath,
	}

	if err := s.videoRepo.Create(ctx, video); err != nil {
		return fileResult{Status: "error", Error: fmt.Sprintf("failed to save video record for %s: %v", filename, err)}
	}

	slog.Info("video imported",
		"video_id", videoID,
		"file", filename,
		"source_id", source.ID,
		"file_path", relPath,
		"duration", metadata.durationSeconds,
		"resolution", metadata.resolution,
		"size_bytes", fileSize,
	)

	return fileResult{Status: "success"}
}

// GetJob returns the job with the given ID. Returns model.ErrNotFound if not found.
func (s *ImportService) GetJob(jobID string) (*model.ImportJob, error) {
	val, ok := s.activeJobs.Load(jobID)
	if !ok {
		return nil, model.ErrNotFound
	}
	return val.(*model.ImportJob), nil
}

// GetActiveJob returns the currently running job, if any. Returns nil when idle.
func (s *ImportService) GetActiveJob() *model.ImportJob {
	var active *model.ImportJob
	s.activeJobs.Range(func(key, value interface{}) bool {
		job := value.(*model.ImportJob)
		if job.Status == "running" {
			active = job
			return false
		}
		return true
	})
	return active
}

// LockForTest locks the import mutex for testing purposes.
func (s *ImportService) LockForTest() {
	s.importMu.Lock()
}

// UnlockForTest unlocks the import mutex for testing purposes.
func (s *ImportService) UnlockForTest() {
	s.importMu.Unlock()
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
		if supportedExtensions[ext] {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", sourceDir, err)
	}

	return files, nil
}

type videoMetadata struct {
	durationSeconds int
	resolution      string
	mimeType        string
}

type ffprobeOutput struct {
	Format  ffprobeFormat   `json:"format"`
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
	Size     string `json:"size"`
}

type ffprobeStream struct {
	CodecType string `json:"codec_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

func (s *ImportService) probeMetadata(ctx context.Context, filePath string) (*videoMetadata, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		return nil, fmt.Errorf("ffprobe failed: %w, stderr: %s", err, stderr)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	duration, _ := strconv.ParseFloat(probe.Format.Duration, 64)

	var resolution string
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			resolution = fmt.Sprintf("%dx%d", stream.Width, stream.Height)
			break
		}
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	mimeType := extensionToMIME(ext)

	return &videoMetadata{
		durationSeconds: int(duration),
		resolution:      resolution,
		mimeType:        mimeType,
	}, nil
}

func (s *ImportService) generateThumbnail(ctx context.Context, filePath string, durationSeconds int) (string, error) {
	seekTime := durationSeconds / 4
	if seekTime < 1 {
		seekTime = 1
	}

	tmpFile, err := os.CreateTemp("", "vaultflix-thumb-*.jpg")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for thumbnail: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-ss", strconv.Itoa(seekTime),
		"-i", filePath,
		"-vframes", "1",
		"-q:v", "2",
		"-y",
		tmpPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("ffmpeg thumbnail failed: %w, output: %s", err, string(output))
	}

	return tmpPath, nil
}

func extensionToMIME(ext string) string {
	switch ext {
	case ".mp4":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".mov":
		return "video/quicktime"
	default:
		return "application/octet-stream"
	}
}
