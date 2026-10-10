package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// FFmpegTool is the production mediaTool: it shells out to ffprobe/ffmpeg
// (Preview cutting lives in preview_clip.go).
type FFmpegTool struct{}

// NewFFmpegTool returns the mediaTool that runs the real ffprobe/ffmpeg.
func NewFFmpegTool() *FFmpegTool {
	return &FFmpegTool{}
}

func (t *FFmpegTool) ProbeJSON(ctx context.Context, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet", "-print_format", "json", "-show_format", "-show_streams", path)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		return nil, fmt.Errorf("ffprobe failed: %w, stderr: %s", err, stderr)
	}
	return out, nil
}

func (t *FFmpegTool) ExtractFrame(ctx context.Context, path string, atSecond int) (string, error) {
	tmp, err := os.CreateTemp("", "vaultflix-thumb-*.jpg")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for thumbnail: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		removeTemp(tmpPath, "")
		return "", fmt.Errorf("failed to close temp thumbnail file: %w", err)
	}

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-ss", strconv.Itoa(atSecond), "-i", path, "-vframes", "1", "-q:v", "2", "-y", tmpPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		removeTemp(tmpPath, "")
		return "", fmt.Errorf("ffmpeg thumbnail failed: %w, output: %s", err, string(output))
	}
	return tmpPath, nil
}

func (t *FFmpegTool) CutPreview(ctx context.Context, path string, durationSeconds int) (string, error) {
	return generatePreviewClip(ctx, path, durationSeconds)
}
