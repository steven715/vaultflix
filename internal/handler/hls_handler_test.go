package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
)

func TestHLSSegment_RejectsInvalidName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &HLSHandler{} // 本 case 在路徑校驗就回 400，不觸及依賴
	r := gin.New()
	r.GET("/api/videos/:id/hls/:segment", h.Segment)

	// 使用單一路徑元件的路徑穿越嘗試（無 %2f 編碼斜線，Gin router 可正常繞送）
	// 編碼斜線 (%2f) 的嘗試由 Gin router 在到達 handler 前以 404 阻擋，是同等安全的；
	// 此測試驗證 regex guard 對非法名稱（含 .ts 但格式不符）正確回 400。
	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/..passwd.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHLSSegment_RejectsNonSegmentName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &HLSHandler{}
	r := gin.New()
	r.GET("/api/videos/:id/hls/:segment", h.Segment)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/index.m3u8.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
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

// --- fakes ---

type fakeResolver struct {
	path string
	err  error
}

func (f *fakeResolver) ResolveDiskPath(ctx context.Context, videoID string) (string, error) {
	return f.path, f.err
}

type fakeKeyframes struct {
	segs      []model.SegmentBoundary
	err       error
	triggered atomic.Int64
}

func (f *fakeKeyframes) GetSegments(ctx context.Context, videoID string) ([]model.SegmentBoundary, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.segs, nil
}

func (f *fakeKeyframes) TriggerProbe(videoID, absPath string) { f.triggered.Add(1) }

type fakeEnsurer struct {
	path string
	err  error
}

func (f *fakeEnsurer) EnsureSegment(ctx context.Context, videoID, inputPath string, idx int, seg model.SegmentBoundary) (string, error) {
	return f.path, f.err
}

func newHLSTestRouter(h *HLSHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/videos/:id/hls/index.m3u8", h.Playlist)
	r.GET("/api/videos/:id/hls/:segment", h.Segment)
	return r
}

func TestHLSPlaylist_ReturnsVODManifestWithTokens(t *testing.T) {
	kf := &fakeKeyframes{segs: []model.SegmentBoundary{
		{Start: 0, Duration: 8.341}, {Start: 8.341, Duration: 6.0},
	}}
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, kf, &fakeEnsurer{})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/index.m3u8?token=tok1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"#EXT-X-PLAYLIST-TYPE:VOD", "#EXT-X-ENDLIST", "seg00000.ts?token=tok1", "seg00001.ts?token=tok1"} {
		if !strings.Contains(body, want) {
			t.Errorf("playlist missing %q:\n%s", want, body)
		}
	}
}

func TestHLSPlaylist_NoIndexReturns503AndTriggersProbe(t *testing.T) {
	kf := &fakeKeyframes{err: model.ErrNotFound}
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, kf, &fakeEnsurer{})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/index.m3u8?token=t", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), "stream_not_ready") {
		t.Errorf("body missing stream_not_ready: %s", w.Body.String())
	}
	if kf.triggered.Load() != 1 {
		t.Errorf("probe triggered = %d, want 1", kf.triggered.Load())
	}
}

func TestHLSPlaylist_VideoNotFound(t *testing.T) {
	h := NewHLSHandler(&fakeResolver{err: model.ErrNotFound}, &fakeKeyframes{}, &fakeEnsurer{})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/nope/hls/index.m3u8", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHLSSegment_ServesGeneratedFile(t *testing.T) {
	segFile := filepath.Join(t.TempDir(), "seg00001.ts")
	if err := os.WriteFile(segFile, []byte("tsdata"), 0o644); err != nil {
		t.Fatal(err)
	}
	kf := &fakeKeyframes{segs: []model.SegmentBoundary{
		{Start: 0, Duration: 6}, {Start: 6, Duration: 6},
	}}
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, kf, &fakeEnsurer{path: segFile})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/seg00001.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "video/mp2t" {
		t.Errorf("content-type = %q, want video/mp2t", ct)
	}
	if w.Body.String() != "tsdata" {
		t.Errorf("body = %q, want tsdata", w.Body.String())
	}
}

func TestHLSSegment_IndexOutOfRange(t *testing.T) {
	kf := &fakeKeyframes{segs: []model.SegmentBoundary{{Start: 0, Duration: 6}}}
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, kf, &fakeEnsurer{})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/seg00007.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHLSSegment_NoIndexReturns404(t *testing.T) {
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, &fakeKeyframes{err: model.ErrNotFound}, &fakeEnsurer{})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/seg00000.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHLSSegment_EnsureFailureReturns500(t *testing.T) {
	kf := &fakeKeyframes{segs: []model.SegmentBoundary{{Start: 0, Duration: 6}}}
	h := NewHLSHandler(&fakeResolver{path: "/mnt/host/D/x.avi"}, kf, &fakeEnsurer{err: errors.New("ffmpeg exploded")})
	r := newHLSTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/videos/v1/hls/seg00000.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
