package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// mediaTool is the ffprobe/ffmpeg port: ffmpegTool runs the real binaries,
// mock.MediaTool fakes them in tests. Every temp path it returns belongs to
// the caller, who must remove it.
type mediaTool interface {
	// ProbeJSON returns `ffprobe -show_format -show_streams` JSON for path, or
	// a wrapped error when ffprobe fails.
	ProbeJSON(ctx context.Context, path string) ([]byte, error)
	// ExtractFrame writes one JPEG frame at atSecond to a temp file, or returns
	// a wrapped ffmpeg error.
	ExtractFrame(ctx context.Context, path string, atSecond int) (string, error)
	// CutPreview writes a Preview clip of path to a temp file, or returns a
	// wrapped ffmpeg error (leaving no temp files behind).
	CutPreview(ctx context.Context, path string, durationSeconds int) (string, error)
}

// MediaProcessor derives the data that comes from a Video's file: Media Info,
// the Thumbnail and the Preview. It owns ffprobe parsing, the temp files ffmpeg
// writes, the object key of each asset and its upload. Import and the
// Backfills share it.
type MediaProcessor struct {
	tool  mediaTool
	minio MinIOClient
}

// NewMediaProcessor creates a MediaProcessor.
func NewMediaProcessor(tool mediaTool, minio MinIOClient) *MediaProcessor {
	return &MediaProcessor{tool: tool, minio: minio}
}

// ProbeMediaInfo reads the Media Info of the file at path; the MIME type also
// depends on the file's extension (see mimeTypeFor). Returns a wrapped error
// when probing or parsing fails.
func (p *MediaProcessor) ProbeMediaInfo(ctx context.Context, path string) (model.MediaInfo, error) {
	raw, err := p.tool.ProbeJSON(ctx, path)
	if err != nil {
		return model.MediaInfo{}, fmt.Errorf("failed to probe %s: %w", path, err)
	}
	info, err := parseProbeOutput(raw, strings.ToLower(filepath.Ext(path)))
	if err != nil {
		return model.MediaInfo{}, fmt.Errorf("failed to read media info of %s: %w", path, err)
	}
	return info, nil
}

// MakeThumbnail grabs a frame a quarter into f, uploads it as f's Thumbnail
// and returns its object key, or a wrapped ffmpeg/upload error.
func (p *MediaProcessor) MakeThumbnail(ctx context.Context, f model.MediaFile) (string, error) {
	at := f.DurationSeconds / 4
	if at < 1 {
		at = 1
	}
	tmp, err := p.tool.ExtractFrame(ctx, f.Path, at)
	if err != nil {
		return "", fmt.Errorf("failed to extract thumbnail of video %s: %w", f.VideoID, err)
	}
	defer removeTemp(tmp, f.VideoID)

	key := fmt.Sprintf("thumbnails/%s.jpg", f.VideoID)
	if err := p.minio.UploadThumbnail(ctx, key, tmp); err != nil {
		return "", fmt.Errorf("failed to upload thumbnail of video %s: %w", f.VideoID, err)
	}
	return key, nil
}

// MakePreview cuts f's Preview, uploads it and returns its object key, or a
// wrapped ffmpeg/upload error.
func (p *MediaProcessor) MakePreview(ctx context.Context, f model.MediaFile) (string, error) {
	tmp, err := p.tool.CutPreview(ctx, f.Path, f.DurationSeconds)
	if err != nil {
		return "", fmt.Errorf("failed to cut preview of video %s: %w", f.VideoID, err)
	}
	defer removeTemp(tmp, f.VideoID)

	key := fmt.Sprintf("previews/%s.mp4", f.VideoID)
	if err := p.minio.UploadPreview(ctx, key, tmp); err != nil {
		return "", fmt.Errorf("failed to upload preview of video %s: %w", f.VideoID, err)
	}
	return key, nil
}

func removeTemp(path, videoID string) {
	if err := os.Remove(path); err != nil {
		slog.Warn("failed to remove temp media file", "video_id", videoID, "path", path, "error", err)
	}
}

type ffprobeOutput struct {
	Format  ffprobeFormat   `json:"format"`
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
}

type ffprobeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// parseProbeOutput reads Media Info from ffprobe JSON; ext is lower-case
// (e.g. ".mp4"). The first video and first audio stream win.
func parseProbeOutput(raw []byte, ext string) (model.MediaInfo, error) {
	var probe ffprobeOutput
	if err := json.Unmarshal(raw, &probe); err != nil {
		return model.MediaInfo{}, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}
	// A missing or malformed duration is reported as 0 rather than failing:
	// some containers omit it and the file is still playable.
	duration, _ := strconv.ParseFloat(probe.Format.Duration, 64)

	info := model.MediaInfo{DurationSeconds: int(duration)}
	for _, stream := range probe.Streams {
		switch {
		case stream.CodecType == "video" && info.VideoCodec == "":
			info.VideoCodec = stream.CodecName
			info.Resolution = fmt.Sprintf("%dx%d", stream.Width, stream.Height)
		case stream.CodecType == "audio" && info.AudioCodec == "":
			info.AudioCodec = stream.CodecName
		}
	}
	info.MimeType = mimeTypeFor(ext, info.VideoCodec, info.AudioCodec)
	return info, nil
}

// mimeTypeFor returns "video/mp4" for direct-play videos (h264+aac in mp4/mov),
// falling back to extensionToMIME for everything else, so an .avi is never
// labelled in a way browsers refuse to play.
func mimeTypeFor(ext, videoCodec, audioCodec string) string {
	if ClassifyPlayMode(ext, videoCodec, audioCodec) == model.PlayModeDirect {
		return "video/mp4"
	}
	return extensionToMIME(ext)
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
