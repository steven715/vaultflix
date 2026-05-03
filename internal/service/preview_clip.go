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

	// Re-encode at concat instead of -c copy: byte-level concat (-c copy)
	// fails when source-derived segments differ in SPS/PPS even after
	// uniform encoder params, which we observed at ~38% on .wmv sources.
	// Re-encoding adds ~2-3s per preview but eliminates the format-specific
	// failure class. Segments are already 480p so no -vf scale here.
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c:v", "libx264",
		"-crf", previewCRF,
		"-an",
		"-pix_fmt", "yuv420p",
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

// extractPreviewSegment extracts a 2.5-second clip starting at startSeconds.
// First tries input seek (-ss before -i), which is fast but silently produces
// an empty output for some containers (observed at ~38% on .wmv sources where
// ffmpeg returns exit 0 yet writes only the MP4 container header). When the
// fast path produces no usable bytes, falls back to output seek (-ss after
// -i): slower because it decodes from the start, but bulletproof.
func extractPreviewSegment(ctx context.Context, srcPath string, startSeconds float64, outPath string) error {
	if err := runSegmentFFmpeg(ctx, srcPath, startSeconds, outPath, true); err != nil {
		return err
	}
	if segmentHasVideoBytes(outPath) {
		return nil
	}
	if err := runSegmentFFmpeg(ctx, srcPath, startSeconds, outPath, false); err != nil {
		return err
	}
	if !segmentHasVideoBytes(outPath) {
		return fmt.Errorf("ffmpeg segment extract produced empty output at %.2fs after both seek modes", startSeconds)
	}
	return nil
}

func runSegmentFFmpeg(ctx context.Context, srcPath string, startSeconds float64, outPath string, inputSeek bool) error {
	startArg := strconv.FormatFloat(startSeconds, 'f', 3, 64)
	args := []string{}
	if inputSeek {
		args = append(args, "-ss", startArg, "-i", srcPath)
	} else {
		args = append(args, "-i", srcPath, "-ss", startArg)
	}
	args = append(args,
		"-t", strconv.FormatFloat(previewSegmentDurationSecs, 'f', 3, 64),
		"-c:v", "libx264",
		"-crf", previewCRF,
		"-an",
		"-vf", fmt.Sprintf("scale=%d:-2,setsar=1", previewWidthPixels),
		"-pix_fmt", "yuv420p",
		"-y",
		outPath,
	)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		mode := "input-seek"
		if !inputSeek {
			mode = "output-seek"
		}
		return fmt.Errorf("ffmpeg segment extract (%s) failed at %.2fs: %w, output: %s", mode, startSeconds, err, string(output))
	}
	return nil
}

// segmentHasVideoBytes returns true when path contains more than a bare MP4
// container. An empty preview segment (no decoded frames) is ~260 bytes — a
// 1 KB threshold cleanly separates that from any real 2.5s 480p clip, which
// is at minimum tens of KB.
func segmentHasVideoBytes(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Size() > 1024
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
