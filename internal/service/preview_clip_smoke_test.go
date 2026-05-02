//go:build smoke

package service

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

// Smoke tests for generatePreviewClip use a synthetic ffmpeg-generated source
// video so they are environment-independent (no fixture files required).
// They are gated behind -tags=smoke because they shell out to ffmpeg/ffprobe
// and would slow down the default test run. Run with:
//
//	go test -tags=smoke -v -run TestSmokePreview ./internal/service/...
//
// Requires ffmpeg + ffprobe in PATH.

func TestSmokePreview_LongVideo_ThreeSegmentConcat(t *testing.T) {
	const srcDuration = 120
	src := generateSyntheticVideo(t, srcDuration)
	defer os.Remove(src)

	out, err := generatePreviewClip(context.Background(), src, srcDuration)
	if err != nil {
		t.Fatalf("generatePreviewClip failed: %v", err)
	}
	defer os.Remove(out)

	dur := probeDurationSeconds(t, out)
	const expected = previewSegmentCount * previewSegmentDurationSecs
	if dur < expected-0.5 || dur > expected+0.5 {
		t.Errorf("expected duration ~%.1fs, got %.2fs", expected, dur)
	}
	t.Logf("long-path preview duration: %.2fs (expected ~%.1fs)", dur, expected)

	w, h := probeResolution(t, out)
	if w != previewWidthPixels {
		t.Errorf("expected width %d, got %d", previewWidthPixels, w)
	}
	if h%2 != 0 {
		t.Errorf("height must be even (yuv420p), got %d", h)
	}
	t.Logf("preview resolution: %dx%d", w, h)

	if !hasFaststart(t, out) {
		t.Errorf("preview missing faststart (moov atom not before mdat)")
	}
}

func TestSmokePreview_ShortVideo_FullTranscode(t *testing.T) {
	const srcDuration = 15
	src := generateSyntheticVideo(t, srcDuration)
	defer os.Remove(src)

	out, err := generatePreviewClip(context.Background(), src, srcDuration)
	if err != nil {
		t.Fatalf("generatePreviewClip failed: %v", err)
	}
	defer os.Remove(out)

	dur := probeDurationSeconds(t, out)
	if dur < 14.0 || dur > 16.0 {
		t.Errorf("short path expected ~%ds, got %.2fs", srcDuration, dur)
	}
	t.Logf("short-path preview duration: %.2fs (expected ~%ds)", dur, srcDuration)
}

func generateSyntheticVideo(t *testing.T, durationSecs int) string {
	t.Helper()
	f, err := os.CreateTemp("", "vaultflix-smoke-src-*.mp4")
	if err != nil {
		t.Fatalf("create temp src: %v", err)
	}
	path := f.Name()
	f.Close()

	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "testsrc=duration="+strconv.Itoa(durationSecs)+":size=1280x720:rate=30",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-y",
		path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(path)
		t.Fatalf("ffmpeg synth source failed: %v, output: %s", err, string(out))
	}
	return path
}

func probeResolution(t *testing.T, path string) (int, int) {
	t.Helper()
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-select_streams", "v:0",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe streams failed: %v", err)
	}
	var probe struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatalf("unmarshal ffprobe streams: %v", err)
	}
	if len(probe.Streams) == 0 {
		t.Fatalf("no video streams in %s", path)
	}
	return probe.Streams[0].Width, probe.Streams[0].Height
}

// hasFaststart returns true when the moov atom appears before the mdat atom.
// It scans only the first 64 KB which is enough since faststart places moov
// right after ftyp at the head of the file.
func hasFaststart(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open preview: %v", err)
	}
	defer f.Close()
	buf := make([]byte, 65536)
	n, _ := f.Read(buf)
	moov := -1
	mdat := -1
	for i := 0; i+4 <= n; i++ {
		if string(buf[i:i+4]) == "moov" && moov == -1 {
			moov = i
		}
		if string(buf[i:i+4]) == "mdat" && mdat == -1 {
			mdat = i
		}
	}
	if moov == -1 {
		return false
	}
	return mdat == -1 || moov < mdat
}

func probeDurationSeconds(t *testing.T, path string) float64 {
	t.Helper()
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe failed: %v", err)
	}
	var probe struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		t.Fatalf("unmarshal ffprobe: %v", err)
	}
	d, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil {
		t.Fatalf("parse duration %q: %v", probe.Format.Duration, err)
	}
	return d
}
