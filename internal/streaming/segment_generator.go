package streaming

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// seekBackoff 是 input seek 刻意往前退的秒數。ffmpeg CLI 對含 B-frames 的輸入
	// (video_delay > 0)會把 -ss 目標自動減 3/23s(dts heuristic),在 mkv 上因此
	// 系統性落到前一個 keyframe —— 落點不可信。策略改為:保證落點嚴格早於 start,
	// 再由 segment muxer 在 keyframe 精準切出目標區間,丟棄提早讀入的部分。
	seekBackoff = 1.0
	// readMargin 是 -to 超讀的秒數:確保下一段的首 keyframe 被讀到並觸發切檔,
	// 讓目標區間的尾邊界精準(超讀的內容落在被丟棄的尾分片)。
	readMargin = 0.5
	// boundaryTol 是 CSV 分片時間與 keyframe pts 的比對容差(秒)。
	boundaryTol = 0.002
)

// SegmentGenerator 產生單一 mpegts 分段檔。
// 失敗時回 error 且不保證 outPath 狀態;呼叫端以暫存檔 + rename 保證原子性。
type SegmentGenerator interface {
	// Generate 從 inputPath 切出內容與 [start, start+duration) 對齊的分段寫入 outPath:
	// 首個 video 封包為 start 處的 keyframe,不含前一個 GOP 或下一段的首 keyframe。
	Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error
}

// FFmpegSegmentGenerator 以 ffmpeg -c copy 實作 SegmentGenerator。
type FFmpegSegmentGenerator struct{}

// NewFFmpegSegmentGenerator 建立 FFmpegSegmentGenerator。
func NewFFmpegSegmentGenerator() *FFmpegSegmentGenerator { return &FFmpegSegmentGenerator{} }

// buildSegmentArgs 產生單段 remux 的 ffmpeg 參數(segment muxer 二段裁切)。
// -ss 在 -i 前(input seek)但退 seekBackoff:落點只需 ≤ start,精準度不依賴 demuxer。
// -copyts 保留來源絕對 pts,使段內時間軸與 manifest 位置一致(取代 -output_ts_offset,
// 也讓 segment list 的分片時間為絕對值)。
// -segment_time 0 讓每個 GOP 自成一個分片,muxer 只在 keyframe 切檔(B-frames 安全);
// 事後由 CSV 清單挑出落在目標區間的分片。
func buildSegmentArgs(inputPath, partsDir string, start, duration float64) []string {
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-ss", formatSeconds(math.Max(0, start-seekBackoff)),
		"-i", inputPath,
		"-copyts",
		"-to", formatSeconds(start + duration + readMargin),
		"-c", "copy",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-f", "segment",
		"-segment_format", "mpegts",
		"-segment_time", "0",
		"-segment_list", filepath.Join(partsDir, "list.csv"),
		"-segment_list_type", "csv",
		"-y", filepath.Join(partsDir, "part%05d.ts"),
	}
}

func formatSeconds(v float64) string {
	return strconv.FormatFloat(v, 'f', 6, 64)
}

// Generate 執行 ffmpeg 產生 GOP 分片後,串接目標區間的分片為最終分段。
// 錯誤訊息附 stderr 尾段以利除錯。
func (g *FFmpegSegmentGenerator) Generate(ctx context.Context, inputPath, outPath string, start, duration float64) error {
	partsDir := outPath + ".parts"
	if err := os.MkdirAll(partsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create parts dir: %w", err)
	}
	defer os.RemoveAll(partsDir)

	cmd := exec.CommandContext(ctx, "ffmpeg", buildSegmentArgs(inputPath, partsDir, start, duration)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg segment generate failed (start=%s): %w: %s",
			formatSeconds(start), err, tail(string(out), 500))
	}
	parts, err := selectParts(filepath.Join(partsDir, "list.csv"), start, start+duration)
	if err != nil {
		return fmt.Errorf("failed to select parts (start=%s): %w", formatSeconds(start), err)
	}
	if err := concatParts(partsDir, parts, outPath); err != nil {
		return fmt.Errorf("failed to concat parts (start=%s): %w", formatSeconds(start), err)
	}
	return nil
}

// selectParts 從 segment muxer 的 CSV 清單(每行 filename,start_time,end_time)
// 挑出內容落在 [start, end) 的分片。-copyts 下 start_time 為來源絕對 pts;
// 例外是第一列恆為 0.000000(muxer 以 0 起算,非實際落點)—— 但 seekBackoff 保證
// start > 0 時第一列必是要丟棄的提早落點內容,不會被選中;start == 0 時 0 即正確值。
func selectParts(listPath string, start, end float64) ([]string, error) {
	data, err := os.ReadFile(listPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read segment list: %w", err)
	}
	var parts []string
	firstTS := math.NaN()
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 3 {
			continue
		}
		ts, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		if ts >= start-boundaryTol && ts < end-boundaryTol {
			if len(parts) == 0 {
				firstTS = ts
			}
			parts = append(parts, fields[0])
		}
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("no parts cover segment start %s", formatSeconds(start))
	}
	// 切段後驗證:首個選中分片必須就在 start 上,否則代表落點晚於預期(來源缺
	// 對應 keyframe 或 demuxer 行為異常),寧可回錯誤也不要送出錯位內容。
	if math.Abs(firstTS-start) > 0.1 {
		return nil, fmt.Errorf("segment content starts at %s, expected %s",
			formatSeconds(firstTS), formatSeconds(start))
	}
	return parts, nil
}

// concatParts 依序把 mpegts 分片以位元組串接寫入 outPath(mpegts 支援直接串接)。
func concatParts(partsDir string, parts []string, outPath string) error {
	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create output: %w", err)
	}
	for _, name := range parts {
		f, err := os.Open(filepath.Join(partsDir, name))
		if err != nil {
			out.Close()
			return fmt.Errorf("failed to open part %s: %w", name, err)
		}
		_, err = io.Copy(out, f)
		f.Close()
		if err != nil {
			out.Close()
			return fmt.Errorf("failed to append part %s: %w", name, err)
		}
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to finalize output: %w", err)
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
