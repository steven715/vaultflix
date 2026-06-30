package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
