package streaming

import (
	"strings"
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func TestBuildVODManifest_Structure(t *testing.T) {
	segs := []model.SegmentBoundary{
		{Start: 0, Duration: 8.341},
		{Start: 8.341, Duration: 6.0},
		{Start: 14.341, Duration: 3.2},
	}
	m := string(BuildVODManifest(segs))

	for _, want := range []string{
		"#EXTM3U",
		"#EXT-X-VERSION:3",
		"#EXT-X-PLAYLIST-TYPE:VOD",
		"#EXT-X-TARGETDURATION:9", // ceil(8.341)
		"#EXT-X-MEDIA-SEQUENCE:0",
		"#EXTINF:8.341000,",
		"seg00000.ts",
		"seg00002.ts",
		"#EXT-X-ENDLIST",
	} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest missing %q:\n%s", want, m)
		}
	}
	if strings.Count(m, "#EXTINF:") != 3 {
		t.Errorf("EXTINF count = %d, want 3", strings.Count(m, "#EXTINF:"))
	}
	// ENDLIST 必須是最後一個 directive
	if !strings.HasSuffix(strings.TrimSpace(m), "#EXT-X-ENDLIST") {
		t.Errorf("manifest does not end with ENDLIST:\n%s", m)
	}
}

func TestSegmentName_Format(t *testing.T) {
	if got := SegmentName(0); got != "seg00000.ts" {
		t.Errorf("SegmentName(0) = %q", got)
	}
	if got := SegmentName(42); got != "seg00042.ts" {
		t.Errorf("SegmentName(42) = %q", got)
	}
}
