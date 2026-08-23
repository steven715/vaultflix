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

// selectPartsFromList 是切段對齊的核心純邏輯,獨立於 ffmpeg 測試
// (ffmpeg 相依測試在無 ffmpeg 的 host 上會 skip,這裡保證 verify gate 有覆蓋)。
func TestSelectPartsFromList_Boundaries(t *testing.T) {
	// 模擬 -ss 11(=12-seekBackoff)落點提早後的典型清單:
	// 第一列恆為 0.000000(muxer quirk,實際內容是提早落點的丟棄段)
	midList := []segmentPart{
		{name: "part0.ts", start: 0, end: 11.999},
		{name: "part1.ts", start: 12.0, end: 13.999},
		{name: "part2.ts", start: 14.0, end: 15.999},
		{name: "part3.ts", start: 16.0, end: 17.999},
		{name: "part4.ts", start: 18.0, end: 18.666},
	}
	tests := []struct {
		name       string
		list       []segmentPart
		start, end float64
		want       []string
		wantErr    string
	}{
		{
			name: "mid segment excludes bogus first row and next boundary",
			list: midList, start: 12.0, end: 18.0,
			want: []string{"part1.ts", "part2.ts", "part3.ts"},
		},
		{
			name: "segment zero keeps genuine first row",
			list: []segmentPart{
				{name: "part0.ts", start: 0, end: 1.999},
				{name: "part1.ts", start: 2.0, end: 3.999},
				{name: "part2.ts", start: 6.0, end: 7.999},
			},
			start: 0, end: 6.0,
			want: []string{"part0.ts", "part1.ts"},
		},
		{
			name:  "no parts in range",
			list:  []segmentPart{{name: "part0.ts", start: 0, end: 5.0}},
			start: 12.0, end: 18.0,
			wantErr: "no parts cover",
		},
		{
			name: "late landing rejected instead of serving misaligned content",
			list: []segmentPart{
				{name: "part0.ts", start: 0, end: 13.999},
				{name: "part1.ts", start: 14.0, end: 15.999},
			},
			start: 12.0, end: 18.0,
			wantErr: "expected 12.000000",
		},
		{
			name: "gap from corrupt row rejected",
			list: []segmentPart{
				{name: "part0.ts", start: 0, end: 11.999},
				{name: "part1.ts", start: 12.0, end: 13.999},
				// part2 列損壞被 parseSegmentList 略過 → part1 與 part3 之間缺洞
				{name: "part3.ts", start: 17.0, end: 17.999},
			},
			start: 12.0, end: 18.0,
			wantErr: "gap between parts",
		},
		{
			// CSV end_time 是末封包 pts,低 fps 內容相鄰分片天然差一個 frame
			// interval(1fps → 1s)—— 不得誤判為缺洞
			name: "low fps frame-interval gaps tolerated",
			list: []segmentPart{
				{name: "part0.ts", start: 0, end: 11.0},
				{name: "part1.ts", start: 12.0, end: 15.0},
				{name: "part2.ts", start: 16.0, end: 17.0},
				{name: "part3.ts", start: 18.0, end: 20.0},
			},
			start: 12.0, end: 18.0,
			want: []string{"part1.ts", "part2.ts"},
		},
		{
			// 目標分片時間值小幅提早抖動(< maxStartDrift)仍要選中,
			// 不得因選取下界過緊被丟掉後再誤報「落點晚於預期」
			name: "small early jitter on target part accepted",
			list: []segmentPart{
				{name: "part0.ts", start: 0, end: 11.899},
				{name: "part1.ts", start: 11.95, end: 13.999},
				{name: "part2.ts", start: 14.0, end: 17.999},
				{name: "part3.ts", start: 18.0, end: 18.666},
			},
			start: 12.0, end: 18.0,
			want: []string{"part1.ts", "part2.ts"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectPartsFromList(tc.list, tc.start, tc.end)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("parts = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseSegmentList_SkipsMalformedRows(t *testing.T) {
	parts := parseSegmentList("part0.ts,0.000000,11.999000\n" +
		"garbage line\n" +
		"part1.ts,not-a-number,13.999000\n" +
		"part2.ts,14.000000,15.999000\n")
	if len(parts) != 2 || parts[0].name != "part0.ts" || parts[1].name != "part2.ts" {
		t.Errorf("parts = %v, want part0.ts and part2.ts only", parts)
	}
	if parts[1].start != 14.0 || parts[1].end != 15.999 {
		t.Errorf("part2 range = [%v, %v], want [14, 15.999]", parts[1].start, parts[1].end)
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
