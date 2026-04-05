package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

var supportedExtensions = map[string]bool{
	".mp4": true,
	".mkv": true,
	".avi": true,
	".wmv": true,
	".mov": true,
}

type ImportResult struct {
	TotalScanned int `json:"total_scanned"`
	Imported     int `json:"imported"`
	Skipped      int `json:"skipped"`
	Failed       int `json:"failed"`
}

type ImportService struct {
	videoRepo repository.VideoRepository
	minioSvc  MinIOClient
}

func NewImportService(videoRepo repository.VideoRepository, minioSvc MinIOClient) *ImportService {
	return &ImportService{
		videoRepo: videoRepo,
		minioSvc:  minioSvc,
	}
}

func (s *ImportService) Run(ctx context.Context, sourceDir string) (*ImportResult, error) {
	files, err := s.scanVideoFiles(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to scan video files: %w", err)
	}

	result := &ImportResult{
		TotalScanned: len(files),
	}

	for _, filePath := range files {
		err := s.processFile(ctx, filePath)
		if err != nil {
			if isSkipError(err) {
				result.Skipped++
				continue
			}
			slog.Error("failed to import video",
				"file", filePath,
				"error", err,
			)
			result.Failed++
			continue
		}
		result.Imported++
	}

	slog.Info("import completed",
		"total_scanned", result.TotalScanned,
		"imported", result.Imported,
		"skipped", result.Skipped,
		"failed", result.Failed,
	)

	return result, nil
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

type skipError struct {
	reason string
}

func (e *skipError) Error() string {
	return e.reason
}

func isSkipError(err error) bool {
	_, ok := err.(*skipError)
	return ok
}

func (s *ImportService) processFile(ctx context.Context, filePath string) error {
	filename := filepath.Base(filePath)

	stat, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}
	fileSize := stat.Size()

	exists, err := s.videoRepo.ExistsByFilenameAndSize(ctx, filename, fileSize)
	if err != nil {
		return fmt.Errorf("failed to check idempotency for %s: %w", filename, err)
	}
	if exists {
		slog.Info("video skipped, already imported",
			"file", filename,
			"size_bytes", fileSize,
		)
		return &skipError{reason: "already imported"}
	}

	metadata, err := s.probeMetadata(ctx, filePath)
	if err != nil {
		return fmt.Errorf("failed to probe metadata for %s: %w", filename, err)
	}

	videoID := uuid.New().String()

	thumbnailPath, err := s.generateThumbnail(ctx, filePath, metadata.durationSeconds)
	if err != nil {
		return fmt.Errorf("failed to generate thumbnail for %s: %w", filename, err)
	}
	defer os.Remove(thumbnailPath)

	videoObjectKey := fmt.Sprintf("videos/%s/%s", videoID, filename)
	thumbnailObjectKey := fmt.Sprintf("thumbnails/%s.jpg", videoID)

	if err := s.minioSvc.UploadVideo(ctx, videoObjectKey, filePath); err != nil {
		return fmt.Errorf("failed to upload video %s: %w", filename, err)
	}

	if err := s.minioSvc.UploadThumbnail(ctx, thumbnailObjectKey, thumbnailPath); err != nil {
		return fmt.Errorf("failed to upload thumbnail for %s: %w", filename, err)
	}

	title := strings.TrimSuffix(filename, filepath.Ext(filename))

	video := &model.Video{
		ID:               videoID,
		Title:            title,
		Description:      "",
		MinIOObjectKey:   videoObjectKey,
		ThumbnailKey:     thumbnailObjectKey,
		DurationSeconds:  metadata.durationSeconds,
		Resolution:       metadata.resolution,
		FileSizeBytes:    fileSize,
		MimeType:         metadata.mimeType,
		OriginalFilename: filename,
	}

	if err := s.videoRepo.Create(ctx, video); err != nil {
		return fmt.Errorf("failed to save video record for %s: %w", filename, err)
	}

	slog.Info("video imported",
		"video_id", videoID,
		"file", filename,
		"duration", metadata.durationSeconds,
		"resolution", metadata.resolution,
		"size_bytes", fileSize,
	)

	return nil
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
