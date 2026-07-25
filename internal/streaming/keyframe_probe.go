package streaming

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/steven/vaultflix/internal/model"
)

// DefaultSegmentTarget is the target segment length in seconds; actual segment length varies based on keyframe distribution.
const DefaultSegmentTarget = 6.0

// ProbeKeyframes scans the video at packet level (no decoding) to get keyframe pts and total duration in seconds.
// Cost is approximately 15s/GB (I/O bound); caller is responsible for timeout and async handling.
func ProbeKeyframes(ctx context.Context, absPath string) ([]float64, float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "packet=pts_time,dts_time,flags:format=duration",
		"-of", "csv",
		absPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, 0, fmt.Errorf("ffprobe keyframe scan failed for %s: %w", absPath, err)
	}
	return parseKeyframeProbe(string(out))
}

// parseKeyframeProbe parses csv output: `packet,<pts_time>,<dts_time>,<flags>` and `format,<duration>`.
// pts 優先;舊式 AVI muxer 只寫 dts 不寫 pts(pts_time=N/A),此時 fallback 用 dts
// (-c copy 的 -ss 對 AVI 走 index/dts seek,dts 作為切點與 pts 等效)。
func parseKeyframeProbe(out string) ([]float64, float64, error) {
	var kf []float64
	var total float64
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		switch {
		case len(fields) >= 4 && fields[0] == "packet":
			if !strings.Contains(fields[3], "K") {
				continue
			}
			ts, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				ts, err = strconv.ParseFloat(fields[2], 64)
			}
			if err != nil {
				continue // pts 與 dts 皆為 N/A 的 packet 跳過
			}
			kf = append(kf, ts)
		case len(fields) >= 2 && fields[0] == "format":
			if d, err := strconv.ParseFloat(fields[1], 64); err == nil {
				total = d
			}
		}
	}
	if len(kf) == 0 {
		return nil, 0, fmt.Errorf("no keyframes found in probe output")
	}
	if total <= 0 {
		return nil, 0, fmt.Errorf("no valid duration in probe output")
	}
	return kf, total, nil
}

// GroupSegments groups keyframe pts into contiguous VOD segment boundaries based on target segment length.
// First segment is forced to start at 0; each internal boundary is a keyframe pts (-c copy only cuts at keyframes).
// Last segment is padded to total; tail shorter than 1s is merged into previous segment.
// Returns nil if total <= 0.
func GroupSegments(kfPts []float64, total, target float64) []model.SegmentBoundary {
	if total <= 0 {
		return nil
	}
	var segs []model.SegmentBoundary
	cur := 0.0
	for _, pts := range kfPts {
		if pts-cur >= target {
			segs = append(segs, model.SegmentBoundary{Start: cur, Duration: pts - cur})
			cur = pts
		}
	}
	if total-cur >= 1.0 || len(segs) == 0 {
		segs = append(segs, model.SegmentBoundary{Start: cur, Duration: total - cur})
	} else {
		last := &segs[len(segs)-1]
		last.Duration = total - last.Start
	}
	return segs
}
