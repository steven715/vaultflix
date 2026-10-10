package mock

import (
	"path/filepath"
	"sync"
)

// KeyframeProber is a hand-written fake for the Import's keyframe prober; it
// records the base name of every file it was asked to index.
type KeyframeProber struct {
	mu    sync.Mutex
	files []string
}

func (m *KeyframeProber) TriggerProbe(videoID, absPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files = append(m.files, filepath.Base(absPath))
}

// Files returns the base names TriggerProbe received, in call order.
func (m *KeyframeProber) Files() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.files...)
}
