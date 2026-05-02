package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const (
	previewSegmentCount        = 3
	previewSegmentDurationSecs = 2.5
	previewSingleClipMaxSecs   = 30
	previewWidthPixels         = 854
	previewCRF                 = "28"
)

var previewSegmentTimestampRatios = [previewSegmentCount]float64{0.10, 0.50, 0.85}

// generatePreviewClip produces a low-bitrate, no-audio MP4 preview from
// srcPath. Videos shorter than 30 seconds are transcoded in their entirety;
// longer videos use three 2.5-second segments at 10%/50%/85% concatenated
// together. The returned path points at a temp file the caller owns and must
// os.Remove after use. On error, all intermediate temp files (including the
// final output) are cleaned up before returning.
func generatePreviewClip(ctx context.Context, srcPath string, durationSeconds int) (string, error) {
	finalFile, err := os.CreateTemp("", "vaultflix-preview-*.mp4")
	if err != nil {
		return "", fmt.Errorf("create temp preview file: %w", err)
	}
	finalPath := finalFile.Name()
	finalFile.Close()

	if durationSeconds < previewSingleClipMaxSecs {
		if err := transcodePreviewFull(ctx, srcPath, finalPath); err != nil {
			os.Remove(finalPath)
			return "", err
		}
		return finalPath, nil
	}

	if err := transcodePreviewSegments(ctx, srcPath, durationSeconds, finalPath); err != nil {
		os.Remove(finalPath)
		return "", err
	}
	return finalPath, nil
}

func transcodePreviewFull(ctx context.Context, srcPath, outPath string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", srcPath,
		"-c:v", "libx264",
		"-crf", previewCRF,
		"-an",
		"-vf", fmt.Sprintf("scale=%d:-2,setsar=1", previewWidthPixels),
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		"-y",
		outPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg full preview transcode failed: %w, output: %s", err, string(output))
	}
	return nil
}

func transcodePreviewSegments(ctx context.Context, srcPath string, durationSeconds int, outPath string) error {
	segPaths := make([]string, 0, previewSegmentCount)
	defer func() {
		for _, p := range segPaths {
			os.Remove(p)
		}
	}()

	for i, ratio := range previewSegmentTimestampRatios {
		segFile, err := os.CreateTemp("", fmt.Sprintf("vaultflix-preview-seg%d-*.mp4", i))
		if err != nil {
			return fmt.Errorf("create temp segment file: %w", err)
		}
		segPath := segFile.Name()
		segFile.Close()
		segPaths = append(segPaths, segPath)

		startSecs := float64(durationSeconds) * ratio
		if err := extractPreviewSegment(ctx, srcPath, startSecs, segPath); err != nil {
			return err
		}
	}

	listPath, err := writeConcatList(segPaths)
	if err != nil {
		return err
	}
	defer os.Remove(listPath)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c", "copy",
		"-movflags", "+faststart",
		"-y",
		outPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat failed: %w, output: %s", err, string(output))
	}
	return nil
}

func extractPreviewSegment(ctx context.Context, srcPath string, startSeconds float64, outPath string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-ss", strconv.FormatFloat(startSeconds, 'f', 3, 64),
		"-i", srcPath,
		"-t", strconv.FormatFloat(previewSegmentDurationSecs, 'f', 3, 64),
		"-c:v", "libx264",
		"-crf", previewCRF,
		"-an",
		"-vf", fmt.Sprintf("scale=%d:-2,setsar=1", previewWidthPixels),
		"-pix_fmt", "yuv420p",
		"-y",
		outPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg segment extract failed at %.2fs: %w, output: %s", startSeconds, err, string(output))
	}
	return nil
}

func writeConcatList(segPaths []string) (string, error) {
	listFile, err := os.CreateTemp("", "vaultflix-preview-list-*.txt")
	if err != nil {
		return "", fmt.Errorf("create temp concat list: %w", err)
	}
	listPath := listFile.Name()

	for _, p := range segPaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			listFile.Close()
			os.Remove(listPath)
			return "", fmt.Errorf("resolve segment path: %w", err)
		}
		if _, err := fmt.Fprintf(listFile, "file '%s'\n", abs); err != nil {
			listFile.Close()
			os.Remove(listPath)
			return "", fmt.Errorf("write concat list: %w", err)
		}
	}
	if err := listFile.Close(); err != nil {
		os.Remove(listPath)
		return "", fmt.Errorf("close concat list: %w", err)
	}
	return listPath, nil
}
