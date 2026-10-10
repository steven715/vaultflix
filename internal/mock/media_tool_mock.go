package mock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// MediaTool is a hand-written fake for the service-side mediaTool port
// (ffprobe/ffmpeg). ProbeOutput is keyed by file base name; ExtractFrame and
// CutPreview write real temp files under Dir so callers' cleanup can be
// observed.
type MediaTool struct {
	Dir         string
	ProbeOutput map[string][]byte
	ProbeErr    error
	FrameErr    error
	PreviewErr  error

	mu       sync.Mutex
	created  []string
	probed   []string
	frameAts []int
}

func (m *MediaTool) ProbeJSON(ctx context.Context, path string) ([]byte, error) {
	m.mu.Lock()
	m.probed = append(m.probed, path)
	m.mu.Unlock()
	if m.ProbeErr != nil {
		return nil, m.ProbeErr
	}
	out, ok := m.ProbeOutput[filepath.Base(path)]
	if !ok {
		return nil, fmt.Errorf("mock: no probe output for %s", path)
	}
	return out, nil
}

func (m *MediaTool) ExtractFrame(ctx context.Context, path string, atSecond int) (string, error) {
	m.mu.Lock()
	m.frameAts = append(m.frameAts, atSecond)
	m.mu.Unlock()
	if m.FrameErr != nil {
		return "", m.FrameErr
	}
	return m.tempFile("frame-*.jpg")
}

func (m *MediaTool) CutPreview(ctx context.Context, path string, durationSeconds int) (string, error) {
	if m.PreviewErr != nil {
		return "", m.PreviewErr
	}
	return m.tempFile("preview-*.mp4")
}

// Probed returns every path ProbeJSON was asked about.
func (m *MediaTool) Probed() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.probed...)
}

// FrameAts returns the second of every ExtractFrame call.
func (m *MediaTool) FrameAts() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]int(nil), m.frameAts...)
}

// Created returns every temp file the fake handed out.
func (m *MediaTool) Created() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.created...)
}

func (m *MediaTool) tempFile(pattern string) (string, error) {
	f, err := os.CreateTemp(m.Dir, pattern)
	if err != nil {
		return "", fmt.Errorf("mock: create temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("mock: close temp file: %w", err)
	}
	m.mu.Lock()
	m.created = append(m.created, f.Name())
	m.mu.Unlock()
	return f.Name(), nil
}

// ProbeJSONFor builds ffprobe -show_format -show_streams JSON for one video
// and one audio stream.
func ProbeJSONFor(durationSeconds float64, videoCodec, audioCodec string) []byte {
	return []byte(fmt.Sprintf(`{"format":{"duration":"%.1f"},"streams":[`+
		`{"codec_type":"video","codec_name":%q,"width":1920,"height":1080},`+
		`{"codec_type":"audio","codec_name":%q}]}`, durationSeconds, videoCodec, audioCodec))
}
