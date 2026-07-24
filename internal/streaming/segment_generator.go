package streaming

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

// SegmentGenerator 產生單一 mpegts 分段檔。
// 失敗時回 error 且不保證 outPath 狀態;呼叫端以暫存檔 + rename 保證原子性。
type SegmentGenerator interface {
	// Generate 從 inputPath 的 start 秒起切出 duration 秒寫入 outPath。
	Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error
}

// FFmpegSegmentGenerator 以 ffmpeg -c copy 實作 SegmentGenerator。
type FFmpegSegmentGenerator struct{}

// NewFFmpegSegmentGenerator 建立 FFmpegSegmentGenerator。
func NewFFmpegSegmentGenerator() *FFmpegSegmentGenerator { return &FFmpegSegmentGenerator{} }

// buildSegmentArgs 產生單段 remux 的 ffmpeg 參數。
// -ss 在 -i 前(input seek;start 即 keyframe pts,落點精準)。
// -output_ts_offset 讓段內 PTS 對齊 manifest 位置(預設每段重置為 0 會讓 hls.js 對齊錯亂);
// 若特定容器 offset 行為異常,備案是改用 -copyts(見 spec Trade-offs)。
func buildSegmentArgs(inputPath, outPath string, start, duration float64) []string {
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-ss", formatSeconds(start),
		"-i", inputPath,
		"-t", formatSeconds(duration),
		"-c", "copy",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-output_ts_offset", formatSeconds(start),
		"-f", "mpegts",
		"-y", outPath,
	}
}

func formatSeconds(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

// Generate 執行 ffmpeg 產生分段;錯誤訊息附 stderr 尾段以利除錯。
func (g *FFmpegSegmentGenerator) Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", buildSegmentArgs(inputPath, outPath, start, duration)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg segment generate failed (start=%s): %w: %s",
			formatSeconds(start), err, tail(string(out), 500))
	}
	return nil
}

// tail 回傳字串最後 n 個 byte(錯誤訊息截斷用)。
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
