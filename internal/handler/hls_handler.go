package handler

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

// hlsStreamer serves a remux Video as VOD HLS (implemented by
// *service.HLSService).
type hlsStreamer interface {
	// Manifest returns ErrNotRemux, ErrStreamPreparing or a
	// VideoService.ResolvePlayback error (all wrapped) on failure.
	Manifest(ctx context.Context, videoID string) ([]byte, error)
	// Segment additionally returns ErrInvalidInput for a malformed name and
	// ErrNotFound for an index past the last segment.
	Segment(ctx context.Context, videoID, name string) (string, error)
}

// HLSHandler 服務 VOD-on-the-fly 的 HLS manifest 與 on-demand 分段；
// 只負責 token 改寫與錯誤 → HTTP 對映，串流邏輯在 HLSService。
type HLSHandler struct {
	hls hlsStreamer
}

// NewHLSHandler 建立 HLSHandler。
func NewHLSHandler(hls hlsStreamer) *HLSHandler {
	return &HLSHandler{hls: hls}
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

// Playlist 回傳 VOD manifest，segment URI 內嵌 token。
// Keyframe Index 準備中時回 503 stream_not_ready（播放器稍後重試）。
// GET /api/videos/:id/hls/index.m3u8
func (h *HLSHandler) Playlist(c *gin.Context) {
	videoID := c.Param("id")
	manifest, err := h.hls.Manifest(c.Request.Context(), videoID)
	if err != nil {
		writeHLSError(c, videoID, err)
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl", rewritePlaylistTokens(manifest, c.Query("token")))
}

// Segment 回傳指定 HLS Segment（on-demand 產生 + 快取）。
// GET /api/videos/:id/hls/:segment
func (h *HLSHandler) Segment(c *gin.Context) {
	videoID := c.Param("id")
	path, err := h.hls.Segment(c.Request.Context(), videoID, c.Param("segment"))
	if err != nil {
		writeHLSError(c, videoID, err)
		return
	}
	c.Header("Content-Type", "video/mp2t")
	c.Header("Cache-Control", "no-cache")
	c.File(path)
}

// writeHLSError maps HLSService errors to HTTP responses.
func writeHLSError(c *gin.Context, videoID string, err error) {
	switch {
	case errors.Is(err, model.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "bad_request", Message: "invalid segment name"})
	case errors.Is(err, model.ErrStreamPreparing):
		c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: "stream_not_ready", Message: "preparing stream for first playback, please retry"})
	case errors.Is(err, model.ErrMediaSourceDisabled):
		c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: "source_unavailable", Message: "media source disabled"})
	case errors.Is(err, model.ErrNotRemux):
		c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "video is not served over HLS"})
	case errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrPathNotExist):
		c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "not_found", Message: "video or segment not found"})
	case errors.Is(err, model.ErrPathNotAllowed):
		c.JSON(http.StatusForbidden, model.ErrorResponse{Error: "path_not_allowed", Message: "path outside allowed area"})
	default:
		slog.Error("failed to serve hls", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal_error", Message: "failed to serve stream"})
	}
}
