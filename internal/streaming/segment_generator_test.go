package streaming

import (
	"strings"
	"testing"
)

func TestBuildSegmentArgs_Order(t *testing.T) {
	args := buildSegmentArgs("/mnt/host/D/x.mkv", "/cache/v1/seg00003.ts.parts", 25.023, 8.341)
	joined := strings.Join(args, " ")

	// input seek:-ss 必須在 -i 之前(靠容器索引直接跳,不線性讀),
	// 且刻意退 seekBackoff —— 落點只需 ≤ start,不信任 demuxer 落點精準度
	ssIdx := indexOf(args, "-ss")
	iIdx := indexOf(args, "-i")
	if ssIdx == -1 || iIdx == -1 || ssIdx > iIdx {
		t.Errorf("-ss must come before -i: %s", joined)
	}
	for _, want := range []string{
		"-ss 24.023000", // start - seekBackoff
		"-copyts",       // 保留絕對 pts,對齊 manifest 時間軸
		"-to 33.864000", // start + duration + readMargin
		"-c copy",
		"-muxdelay 0",
		"-muxpreload 0",
		"-f segment",
		"-segment_format mpegts",
		"-segment_time 0", // 每個 GOP 一個分片,只在 keyframe 切檔
		"-segment_list /cache/v1/seg00003.ts.parts/list.csv",
		"-segment_list_type csv",
		"/cache/v1/seg00003.ts.parts/part%05d.ts",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
	// 舊參數不得殘留:-output_ts_offset 被 -copyts 取代;-t 被絕對的 -to 取代
	// (-t 在落點提早時會從負的 rebase 時間起算,導致段尾多切)
	for _, forbidden := range []string{"-output_ts_offset", "-t "} {
		if strings.Contains(joined+" ", forbidden) {
			t.Errorf("args must not contain %q: %s", forbidden, joined)
		}
	}
}

func TestBuildSegmentArgs_SeekClampedAtZero(t *testing.T) {
	args := buildSegmentArgs("/mnt/host/D/x.mkv", "/cache/v1/seg00000.ts.parts", 0, 6.0)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-ss 0.000000") {
		t.Errorf("seek must clamp to 0 for first segment: %s", joined)
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
