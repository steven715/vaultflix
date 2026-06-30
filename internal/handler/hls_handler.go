package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
	"github.com/steven/vaultflix/internal/streaming"
)

var segmentNameRe = regexp.MustCompile(`^seg\d{5}\.ts$`)

// HLSHandler 服務即時 remux 的 HLS playlist 與分段。
type HLSHandler struct {
	videoService *service.VideoService
	manager      *streaming.Manager
}

// NewHLSHandler 建立 HLSHandler，注入 video service 與 streaming manager。
func NewHLSHandler(videoSvc *service.VideoService, mgr *streaming.Manager) *HLSHandler {
	return &HLSHandler{videoService: videoSvc, manager: mgr}
}

// Playlist 啟動/取得 HLS session 並回傳 playlist 檔案。
// GET /api/videos/:id/hls/index.m3u8
func (h *HLSHandler) Playlist(c *gin.Context) {
	ctx := c.Request.Context()
	videoID := c.Param("id")
	userID := c.GetString("user_id")

	inputPath, err := h.videoService.ResolveDiskPath(ctx, videoID)
	if err != nil {
		h.writePathError(c, videoID, err)
		return
	}

	sess, err := h.manager.EnsureSession(ctx, videoID, userID, inputPath)
	if err != nil {
		slog.Error("failed to ensure hls session", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to start stream",
		})
		return
	}

	playlistPath := filepath.Join(sess.Dir, streaming.PlaylistName)
	// ffmpeg 漸進寫入 playlist；初次請求時檔案可能尚未出現，短暫輪詢等待。
	if !waitForFile(playlistPath, 10*time.Second) {
		c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{
			Error:   "stream_not_ready",
			Message: "stream is starting, please retry",
		})
		return
	}

	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.Header("Cache-Control", "no-cache")
	c.File(playlistPath)
}

// Segment 回傳指定 .ts 分段檔案。
// GET /api/videos/:id/hls/:segment
func (h *HLSHandler) Segment(c *gin.Context) {
	videoID := c.Param("id")
	userID := c.GetString("user_id")
	seg := c.Param("segment")

	if !segmentNameRe.MatchString(seg) {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "invalid segment name",
		})
		return
	}

	dir, ok := h.manager.SessionDir(videoID, userID)
	if !ok {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error:   "not_found",
			Message: "no active stream session",
		})
		return
	}
	h.manager.Touch(videoID, userID)

	segPath := filepath.Join(dir, seg)
	// 二重防護：即使 regex 通過，確認解析後路徑仍在 session 目錄內。
	if filepath.Dir(segPath) != filepath.Clean(dir) {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error:   "bad_request",
			Message: "invalid segment path",
		})
		return
	}
	if _, err := os.Stat(segPath); err != nil {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error:   "not_found",
			Message: "segment not found",
		})
		return
	}

	c.Header("Content-Type", "video/mp2t")
	c.File(segPath)
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

// waitForFile polls until path exists or timeout expires.
func waitForFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
