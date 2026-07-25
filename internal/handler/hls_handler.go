package handler

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/streaming"
)

var segmentNameRe = regexp.MustCompile(`^seg(\d{5})\.ts$`)

// diskPathResolver 解析影片在容器內的絕對路徑（由 *service.VideoService 實作）。
type diskPathResolver interface {
	ResolveDiskPath(ctx context.Context, videoID string) (string, error)
}

// keyframeProvider 提供邊界表查詢與非同步探測（由 *service.KeyframeService 實作）。
type keyframeProvider interface {
	// GetSegments 無邊界表時回 model.ErrNotFound。
	GetSegments(ctx context.Context, videoID string) ([]model.SegmentBoundary, error)
	TriggerProbe(videoID, absPath string)
}

// segmentEnsurer 確保分段存在並回傳檔案路徑（由 *streaming.SegmentCache 實作）。
type segmentEnsurer interface {
	EnsureSegment(ctx context.Context, videoID, inputPath string, idx int, seg model.SegmentBoundary) (string, error)
}

// HLSHandler 服務 VOD-on-the-fly 的 HLS manifest 與 on-demand 分段。
type HLSHandler struct {
	videoService diskPathResolver
	keyframes    keyframeProvider
	segments     segmentEnsurer
}

// NewHLSHandler 建立 HLSHandler。
func NewHLSHandler(videoSvc diskPathResolver, kf keyframeProvider, segs segmentEnsurer) *HLSHandler {
	return &HLSHandler{videoService: videoSvc, keyframes: kf, segments: segs}
}

// rewritePlaylistTokens rewrites segment URIs in an m3u8 playlist to append
// a token query parameter. Lines starting with '#' or blank lines pass through
// unchanged. Non-comment, non-empty lines (segment URIs) get '?token=<escaped>'
// appended if they do not already contain '?', or '&token=<escaped>' if they do.
// If token is empty the raw bytes are returned unchanged.
func rewritePlaylistTokens(raw []byte, token string) []byte {
	if token == "" {
		return raw
	}
	escaped := url.QueryEscape(token)
	var buf bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			buf.WriteString(line)
		} else {
			if strings.Contains(line, "?") {
				buf.WriteString(line + "&token=" + escaped)
			} else {
				buf.WriteString(line + "?token=" + escaped)
			}
		}
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// Playlist 由邊界表組出完整 VOD manifest（segment URI 內嵌 token）。
// 無邊界表時回 503 stream_not_ready 並觸發背景探測（首播 lazy fallback）。
// GET /api/videos/:id/hls/index.m3u8
func (h *HLSHandler) Playlist(c *gin.Context) {
	ctx := c.Request.Context()
	videoID := c.Param("id")
	token := c.Query("token")

	inputPath, err := h.videoService.ResolveDiskPath(ctx, videoID)
	if err != nil {
		h.writePathError(c, videoID, err)
		return
	}

	segs, err := h.keyframes.GetSegments(ctx, videoID)
	if errors.Is(err, model.ErrNotFound) {
		h.keyframes.TriggerProbe(videoID, inputPath)
		c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{
			Error:   "stream_not_ready",
			Message: "preparing stream for first playback, please retry",
		})
		return
	}
	if err != nil {
		slog.Error("failed to load keyframe index", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to load stream index",
		})
		return
	}

	manifest := streaming.BuildVODManifest(segs)
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl", rewritePlaylistTokens(manifest, token))
}

// Segment 確保並回傳指定分段（on-demand 產生 + 快取）。
// GET /api/videos/:id/hls/:segment
func (h *HLSHandler) Segment(c *gin.Context) {
	ctx := c.Request.Context()
	videoID := c.Param("id")
	segName := c.Param("segment")

	m := segmentNameRe.FindStringSubmatch(segName)
	if m == nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "invalid segment name",
		})
		return
	}
	idx, err := strconv.Atoi(m[1])
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "invalid segment index",
		})
		return
	}

	inputPath, err := h.videoService.ResolveDiskPath(ctx, videoID)
	if err != nil {
		h.writePathError(c, videoID, err)
		return
	}

	segs, err := h.keyframes.GetSegments(ctx, videoID)
	if errors.Is(err, model.ErrNotFound) {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error:   "not_found",
			Message: "no stream index for video",
		})
		return
	}
	if err != nil {
		slog.Error("failed to load keyframe index", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to load stream index",
		})
		return
	}
	if idx >= len(segs) {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error:   "not_found",
			Message: "segment out of range",
		})
		return
	}

	path, err := h.segments.EnsureSegment(ctx, videoID, inputPath, idx, segs[idx])
	if err != nil {
		slog.Error("failed to ensure segment", "error", err, "video_id", videoID, "segment", segName)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to generate segment",
		})
		return
	}

	c.Header("Content-Type", "video/mp2t")
	c.Header("Cache-Control", "no-cache")
	c.File(path)
}

// writePathError maps ResolveDiskPath errors to HTTP responses.
func (h *HLSHandler) writePathError(c *gin.Context, videoID string, err error) {
	switch {
	case errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrPathNotExist):
		c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "video not playable"})
	case errors.Is(err, model.ErrConflict):
		c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: "source_unavailable", Message: "media source disabled"})
	case errors.Is(err, model.ErrPathNotAllowed):
		c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "path_not_allowed", Message: "path outside allowed area"})
	default:
		slog.Error("failed to resolve disk path", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to access video"})
	}
}
