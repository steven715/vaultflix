package streaming

import (
	"strings"
	"testing"
)

func TestBuildSegmentArgs_Order(t *testing.T) {
	args := buildSegmentArgs("/mnt/host/D/x.avi", "/cache/v1/seg00003.ts", 25.023, 8.341)
	joined := strings.Join(args, " ")

	// input seek:-ss 必須在 -i 之前(靠容器索引直接跳,不線性讀)
	ssIdx := indexOf(args, "-ss")
	iIdx := indexOf(args, "-i")
	if ssIdx == -1 || iIdx == -1 || ssIdx > iIdx {
		t.Errorf("-ss must come before -i: %s", joined)
	}
	for _, want := range []string{
		"-ss 25.023000",
		"-t 8.341000",
		"-c copy",
		"-output_ts_offset 25.023000",
		"-muxdelay 0",
		"-muxpreload 0",
		"-f mpegts",
		"/cache/v1/seg00003.ts",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}
