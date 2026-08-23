package streaming

// 本檔測試需要真實 ffmpeg/ffprobe(無則 skip):驗證切出的分段「內容」與 manifest
// 宣告的邊界一致。重現的 bug:ffmpeg CLI 對含 B-frames 的輸入(video_delay > 0)
// 做 input seek 時會把 -ss 目標自動減 3/23 秒(dts heuristic),在 mkv 上因此
// 系統性落到前一個 keyframe —— 切出的段 = 前一整個 GOP + 自己 + 下一段首 keyframe,
// 與 manifest 錯位,hls.js 反覆覆寫 SourceBuffer 造成播放斷斷續續。

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			// 設 VAULTFLIX_REQUIRE_FFMPEG=1 的環境(test-integration 的 api 容器)
			// 存在的意義就是「用生產 ffmpeg 跑這些測試」,缺 ffmpeg 是 gate 失效,
			// 必須紅燈而非無聲 skip。
			if os.Getenv("VAULTFLIX_REQUIRE_FFMPEG") == "1" {
				t.Fatalf("%s not in PATH but VAULTFLIX_REQUIRE_FFMPEG=1", bin)
			}
			t.Skipf("%s not in PATH; run inside the api container or install ffmpeg locally", bin)
		}
	}
}

// makeMKVFixture 產生 30s 的測試 mkv(x264 + aac,GOP 2s)。
// bframes > 0 時輸出含 B-frames(video_delay > 0),觸發 ffmpeg CLI 的 input seek
// dts heuristic(落點提早);bframes = 0 模擬落點精準的來源(如部分 AVI/MP4)。
// 參數配方與 scripts/gen_fixtures.sh(整合測試 fixture 的重生腳本)保持一致:
// B-frames 是這隻 bug 的觸發條件,fixture 若失去 -bf 就測不到落點偏移。
func makeMKVFixture(t *testing.T, bframes int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), fmt.Sprintf("fixture_bf%d.mkv", bframes))
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-t", "30",
		"-c:v", "libx264", "-preset", "veryfast", "-bf", strconv.Itoa(bframes),
		"-g", "48", "-keyint_min", "48", "-sc_threshold", "0",
		"-c:a", "aac", "-b:a", "64k",
		"-y", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create fixture: %v: %s", err, out)
	}
	return path
}

// probeVideoPackets 回傳分段檔內 video stream 的首/末 pts 與所有 keyframe pts。
// 末封包偶爾 pts=N/A(mpegts 尾端 flush),取最後一個有效 pts。
func probeVideoPackets(t *testing.T, path string) (first, last float64, keyframes []float64) {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time,flags",
		"-of", "csv", path).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Fatalf("ffprobe failed for %s: %v: %s", path, err, ee.Stderr)
		}
		t.Fatalf("ffprobe failed for %s: %v", path, err)
	}
	first, last = math.NaN(), math.NaN()
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 3 || fields[0] != "packet" {
			continue
		}
		pts, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		if math.IsNaN(first) {
			first = pts
		}
		last = pts
		if strings.Contains(fields[2], "K") {
			keyframes = append(keyframes, pts)
		}
	}
	if math.IsNaN(first) {
		t.Fatalf("no video packets in %s", path)
	}
	return first, last, keyframes
}

// TestGenerate_ContentMatchesBoundary 對每個分段驗證核心契約:
// 切出的段內容 == manifest 宣告的 [Start, Start+Duration) 區間。
//   - 首個 video 封包必須是 keyframe 且 pts == Start(mpegts 90kHz 容差)
//   - 所有封包(含 keyframe)都落在區間內,不得包含前一個 GOP 或下一段的首 keyframe
func TestGenerate_ContentMatchesBoundary(t *testing.T) {
	requireFFmpeg(t)

	tests := []struct {
		name    string
		bframes int
	}{
		{name: "mkv with b-frames input seek lands early", bframes: 2},
		{name: "mkv without b-frames input seek lands exact", bframes: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := makeMKVFixture(t, tc.bframes)
			kf, total, err := ProbeKeyframes(context.Background(), input)
			if err != nil {
				t.Fatalf("ProbeKeyframes failed: %v", err)
			}
			segs := GroupSegments(kf, total, DefaultSegmentTarget)
			if len(segs) < 3 {
				t.Fatalf("fixture too short: only %d segments", len(segs))
			}

			gen := NewFFmpegSegmentGenerator()
			outDir := t.TempDir()
			for i, seg := range segs {
				outPath := filepath.Join(outDir, SegmentName(i))
				if err := gen.Generate(context.Background(), input, outPath, seg.Start, seg.Duration); err != nil {
					t.Fatalf("Generate segment %d failed: %v", i, err)
				}
				assertSegmentAligned(t, outPath, i, seg, kf[0])
			}
		})
	}
}

// assertSegmentAligned 驗證單一分段檔的內容與 manifest 宣告的 [Start, Start+Duration)
// 對齊:首封包是 Start 處的 keyframe、所有封包與 keyframe 不越界。
func assertSegmentAligned(t *testing.T, outPath string, i int, seg model.SegmentBoundary, firstKF float64) {
	t.Helper()
	const tol = 0.05 // mpegts 90kHz 轉換與 ffprobe 輸出的 ms 級容差
	first, last, kfs := probeVideoPackets(t, outPath)
	end := seg.Start + seg.Duration

	// 段 0 的 Start 被 GroupSegments 強制為 0,但檔案首個 keyframe
	// 可能因 reorder delay 晚於 0 —— 內容最早只能從那裡開始。
	// 段 0 另有 mpegts avoid_negative_ts 平移:首 keyframe 的 dts 為負
	// (pts 0 - reorder delay),muxer 把整段時間軸前移該量(數個 frame,
	// 與修法前的生產行為相同),故容差放寬到 0.25s。
	wantFirst, wantTol := math.Max(seg.Start, firstKF), tol
	if i == 0 {
		wantTol = 0.25
	}
	if math.Abs(first-wantFirst) > wantTol {
		t.Errorf("segment %d: first video pts = %.6f, manifest declares start %.6f", i, first, wantFirst)
	}
	if len(kfs) == 0 || math.Abs(kfs[0]-first) > tol {
		t.Errorf("segment %d: does not start with a keyframe (first pts %.6f, keyframes %v)", i, first, kfs)
	}
	if last >= end+tol {
		t.Errorf("segment %d: last video pts = %.6f exceeds declared end %.6f", i, last, end)
	}
	for _, k := range kfs {
		if k < seg.Start-tol || k >= end-tol {
			t.Errorf("segment %d: keyframe %.6f outside declared range [%.6f, %.6f)", i, k, seg.Start, end)
		}
	}
}
