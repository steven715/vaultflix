package streaming

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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
	// boundaryTol 是 CSV 分片時間與 keyframe pts 的比對容差(秒),
	// 涵蓋 ffprobe 六位小數輸出與 mpegts 90kHz 轉換的捨入誤差。
	boundaryTol = 0.002
	// maxStartDrift 是首個選中分片與 start 允許的最大偏差(秒)。超過代表落點晚於
	// 預期(來源缺對應 keyframe 或 demuxer 行為異常),與 boundaryTol 刻意分開:
	// boundaryTol 是「同一個 keyframe 的時間值誤差」,這裡是「落在別的 keyframe」的判準。
	maxStartDrift = 0.1
	// continuityTol 是相鄰分片 end→start 允許的最大空隙(秒)。CSV 的 end_time 是
	// 分片末封包的 pts,與下一分片起點天然差一個 frame interval —— 低 fps 內容
	// (如 1fps 的簡報/監控片)可達 1s,故容差取 2s:仍能攔下整個 GOP 消失的缺洞
	// (清單列損壞),不誤殺慢速 frame rate 的正常內容。
	continuityTol = 2.0
	// segmentListName 是 segment muxer 輸出的 CSV 清單檔名(buildSegmentArgs 與
	// Generate 共用,單一定義避免兩處字面值漂移)。
	segmentListName = "list.csv"
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
		"-segment_list", filepath.Join(partsDir, segmentListName),
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
	defer func() {
		if err := os.RemoveAll(partsDir); err != nil {
			slog.Warn("failed to remove parts dir", "path", partsDir, "error", err)
		}
	}()

	cmd := exec.CommandContext(ctx, "ffmpeg", buildSegmentArgs(inputPath, partsDir, start, duration)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg segment generate failed (start=%s): %w: %s",
			formatSeconds(start), err, tail(string(out), 500))
	}
	list, err := os.ReadFile(filepath.Join(partsDir, segmentListName))
	if err != nil {
		return fmt.Errorf("failed to read segment list (start=%s): %w", formatSeconds(start), err)
	}
	parts, err := selectPartsFromList(parseSegmentList(string(list)), start, start+duration)
	if err != nil {
		return fmt.Errorf("failed to select parts (start=%s): %w", formatSeconds(start), err)
	}
	if err := concatParts(partsDir, parts, outPath); err != nil {
		return fmt.Errorf("failed to concat parts (start=%s): %w", formatSeconds(start), err)
	}
	return nil
}

// segmentPart 是 segment muxer CSV 清單的一列:分片檔名與其內容的絕對時間範圍。
type segmentPart struct {
	name       string
	start, end float64
}

// parseSegmentList 解析 segment muxer 的 CSV 清單(每行 filename,start_time,end_time)。
// 無法解析的列直接略過;因此消失的「中段」分片由 selectPartsFromList 的連續性驗證
// 攔下,消失的「末段」分片則無從偵測(fail-open)—— 該情境需要 ffmpeg 輸出損壞的
// 清單,實務上未觀測過,且尾端硬驗證會誤殺 duration metadata 偏大的舊檔末段。
func parseSegmentList(data string) []segmentPart {
	var parts []segmentPart
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 3 {
			continue
		}
		start, err1 := strconv.ParseFloat(fields[1], 64)
		end, err2 := strconv.ParseFloat(fields[2], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		parts = append(parts, segmentPart{name: fields[0], start: start, end: end})
	}
	return parts
}

// selectPartsFromList 挑出 start_time 落在 [start, end) 的分片。-copyts 下
// start_time 為來源絕對 pts;例外是第一列恆為 0.000000(muxer 以 0 起算,非實際
// 落點)—— seekBackoff 保證 start > 0 時第一列必是要丟棄的提早落點內容,不會被
// 選中;start == 0 時 0 即正確值。已知限制(fail-loud):影片首個 keyframe 晚於
// seekBackoff 的檔案(內容延遲 6s+ 才開始,GroupSegments 仍強制段 0 從 0 起算)
// 會在此回錯誤 —— 該檔類的 remux 在修法前也是壞的(時間軸整段錯位)。
// 選取下界放寬到 maxStartDrift(與驗收一致):目標分片的時間值若有小幅提早抖動
// 仍被選中,再由後方檢查統一把關;上界維持 boundaryTol,避免把下一段起點吃進來。
// 切段後驗證:首個分片必須就在 start 上(落點晚於預期即報錯),相鄰分片必須連續
// (清單列損壞造成的中段缺洞即報錯)—— 寧可回錯誤也不要送出錯位或缺內容的分段。
func selectPartsFromList(all []segmentPart, start, end float64) ([]string, error) {
	var names []string
	var prev segmentPart
	for _, p := range all {
		if p.start < start-maxStartDrift || p.start >= end-boundaryTol {
			continue
		}
		if len(names) == 0 && p.start-start > maxStartDrift {
			return nil, fmt.Errorf("segment content starts at %s, expected %s",
				formatSeconds(p.start), formatSeconds(start))
		}
		if len(names) > 0 && p.start-prev.end > continuityTol {
			return nil, fmt.Errorf("gap between parts %s (ends %s) and %s (starts %s)",
				prev.name, formatSeconds(prev.end), p.name, formatSeconds(p.start))
		}
		names = append(names, p.name)
		prev = p
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no parts cover segment start %s", formatSeconds(start))
	}
	return names, nil
}

// concatParts 依序把 mpegts 分片以位元組串接寫入 outPath(mpegts 支援直接串接)。
// 單一分片(GOP ≥ 段目標長度的常見情形)直接 rename,免二次讀寫。
func concatParts(partsDir string, parts []string, outPath string) error {
	if len(parts) == 1 {
		if err := os.Rename(filepath.Join(partsDir, parts[0]), outPath); err == nil {
			return nil
		}
		// rename 失敗(同目錄下理論上不會)退回複製路徑
	}
	out, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create output: %w", err)
	}
	// 錯誤路徑靠 defer 收(重複 Close 是無害的 ErrClosed);寫入完成與否看下方顯式 Close。
	defer out.Close()
	for _, name := range parts {
		f, err := os.Open(filepath.Join(partsDir, name))
		if err != nil {
			return fmt.Errorf("failed to open part %s: %w", name, err)
		}
		_, err = io.Copy(out, f)
		f.Close()
		if err != nil {
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
