package streaming

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

const (
	// PlaylistName 是每個 session 暫存目錄內的 HLS playlist 檔名。
	PlaylistName = "index.m3u8"
	// segmentPattern 是 ffmpeg 寫出的分段檔名樣板。
	segmentPattern = "seg%05d.ts"
)

// buildRemuxHLSArgs 產生「容器換殼不重新編碼」的 ffmpeg HLS 參數。
// 用 event playlist 讓 hls.js 能在分段陸續產出時即時開播。
func buildRemuxHLSArgs(inputPath, outDir string) []string {
	return []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-c", "copy",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "event",
		"-hls_segment_filename", filepath.Join(outDir, segmentPattern),
		filepath.Join(outDir, PlaylistName),
	}
}

// TranscodeProc 是一個進行中的 HLS 產出程序。
type TranscodeProc interface {
	// Stop 終止程序並回傳等待結果；對已結束的程序為 no-op。
	Stop() error
	// Done 在程序自然結束時關閉。
	Done() <-chan struct{}
}

// Transcoder 啟動把 inputPath 轉成 HLS（playlist + 分段）寫入 outDir 的程序。
// 找不到輸入或無法啟動時回傳 error。
type Transcoder interface {
	Start(ctx context.Context, inputPath, outDir string) (TranscodeProc, error)
}

// FFmpegTranscoder 以 -c copy 即時 remux 為 HLS。
type FFmpegTranscoder struct{}

func NewFFmpegTranscoder() *FFmpegTranscoder { return &FFmpegTranscoder{} }

func (t *FFmpegTranscoder) Start(ctx context.Context, inputPath, outDir string) (TranscodeProc, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", buildRemuxHLSArgs(inputPath, outDir)...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg remux: %w", err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	return &ffmpegProc{cmd: cmd, done: done}, nil
}

type ffmpegProc struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *ffmpegProc) Done() <-chan struct{} { return p.done }

func (p *ffmpegProc) Stop() error {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	<-p.done
	return nil
}
