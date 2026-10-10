package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func newHLSTestRouter(h *HLSHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/videos/:id/hls/index.m3u8", h.Playlist)
	r.GET("/api/videos/:id/hls/:segment", h.Segment)
	return r
}

func serveHLS(h *HLSHandler, url string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	newHLSTestRouter(h).ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	return w
}

func TestHLSPlaylist_ReturnsManifestWithTokens(t *testing.T) {
	h := NewHLSHandler(&mock.HLSStreamer{ManifestBytes: []byte("#EXTM3U\nseg00000.ts\n#EXT-X-ENDLIST\n")})

	w := serveHLS(h, "/api/videos/v1/hls/index.m3u8?token=tok1")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/vnd.apple.mpegurl" {
		t.Errorf("content-type = %q", ct)
	}
	if !strings.Contains(w.Body.String(), "seg00000.ts?token=tok1") {
		t.Errorf("body missing tokenised segment URI:\n%s", w.Body.String())
	}
}

func TestHLSSegment_ServesFile(t *testing.T) {
	segFile := filepath.Join(t.TempDir(), "seg00001.ts")
	if err := os.WriteFile(segFile, []byte("tsdata"), 0o644); err != nil {
		t.Fatal(err)
	}
	streamer := &mock.HLSStreamer{SegmentPath: segFile}
	h := NewHLSHandler(streamer)

	w := serveHLS(h, "/api/videos/v1/hls/seg00001.ts")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "video/mp2t" {
		t.Errorf("content-type = %q, want video/mp2t", ct)
	}
	if w.Body.String() != "tsdata" {
		t.Errorf("body = %q, want tsdata", w.Body.String())
	}
	if streamer.LastSegmentName != "seg00001.ts" {
		t.Errorf("segment name passed = %q, want seg00001.ts", streamer.LastSegmentName)
	}
}

func TestWriteHLSError_MapsSentinelsToHTTP(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{model.ErrInvalidInput, http.StatusBadRequest, "bad_request"},
		{model.ErrStreamPreparing, http.StatusServiceUnavailable, "stream_not_ready"},
		{model.ErrMediaSourceDisabled, http.StatusServiceUnavailable, "source_unavailable"},
		{model.ErrNotRemux, http.StatusNotFound, "not_found"},
		{model.ErrNotFound, http.StatusNotFound, "not_found"},
		{model.ErrPathNotExist, http.StatusNotFound, "not_found"},
		{model.ErrPathNotAllowed, http.StatusForbidden, "path_not_allowed"},
		{fmt.Errorf("ffmpeg exploded"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tt := range tests {
		for _, url := range []string{"/api/videos/v1/hls/index.m3u8", "/api/videos/v1/hls/seg00000.ts"} {
			t.Run(tt.wantCode+" "+url, func(t *testing.T) {
				h := NewHLSHandler(&mock.HLSStreamer{Err: fmt.Errorf("wrapped: %w", tt.err)})

				w := serveHLS(h, url)

				if w.Code != tt.wantStatus {
					t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
				}
				if !strings.Contains(w.Body.String(), `"error":"`+tt.wantCode+`"`) {
					t.Errorf("body = %s, want error %q", w.Body.String(), tt.wantCode)
				}
			})
		}
	}
}

func TestRewritePlaylistTokens(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		token    string
		wantLine string // a line that must appear in output
	}{
		{
			name:     "comment lines pass through unchanged",
			raw:      "#EXTM3U\n#EXT-X-VERSION:3\n",
			token:    "mytoken",
			wantLine: "#EXTM3U",
		},
		{
			name:     "segment URI gets token appended",
			raw:      "#EXTM3U\nseg00000.ts\n",
			token:    "mytoken",
			wantLine: "seg00000.ts?token=mytoken",
		},
		{
			name:     "empty token leaves raw unchanged",
			raw:      "#EXTM3U\nseg00000.ts\n",
			token:    "",
			wantLine: "seg00000.ts",
		},
		{
			name:     "segment URI already with query gets amp-token",
			raw:      "#EXTM3U\nseg00000.ts?foo=bar\n",
			token:    "mytoken",
			wantLine: "seg00000.ts?foo=bar&token=mytoken",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(rewritePlaylistTokens([]byte(tc.raw), tc.token))
			found := false
			for _, line := range splitLines(got) {
				if line == tc.wantLine {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("rewritePlaylistTokens output:\n%s\ndoes not contain expected line: %q", got, tc.wantLine)
			}
		})
	}
}

// splitLines splits s by newline, filtering empty strings.
func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if line != "" {
				out = append(out, line)
			}
			start = i + 1
		}
	}
	if start < len(s) && s[start:] != "" {
		out = append(out, s[start:])
	}
	return out
}
