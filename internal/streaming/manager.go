package streaming

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manager 管理每個 (videoID,userID) 的即時 HLS session。
// 用 mutex 守護 map（非裸用），不需 actor/channel 複雜度。
type Manager struct {
	transcoder  Transcoder
	cacheDir    string
	idleTimeout time.Duration

	mu       sync.Mutex
	sessions map[string]*sessionState
}

type sessionState struct {
	dir        string
	proc       TranscodeProc
	lastAccess time.Time
}

// Session 是對外暴露的 session 視圖。
type Session struct{ Dir string }

func NewManager(t Transcoder, cacheDir string, idleTimeout time.Duration) *Manager {
	return &Manager{
		transcoder:  t,
		cacheDir:    cacheDir,
		idleTimeout: idleTimeout,
		sessions:    make(map[string]*sessionState),
	}
}

func sessionKey(videoID, userID string) string { return videoID + "|" + userID }

// EnsureSession 啟動或回傳既有 session，並更新 lastAccess。
func (m *Manager) EnsureSession(ctx context.Context, videoID, userID, inputPath string) (*Session, error) {
	key := sessionKey(videoID, userID)

	m.mu.Lock()
	defer m.mu.Unlock()

	if st, ok := m.sessions[key]; ok {
		st.lastAccess = time.Now()
		return &Session{Dir: st.dir}, nil
	}

	dir := filepath.Join(m.cacheDir, sanitizeKey(key))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create session dir: %w", err)
	}

	proc, err := m.transcoder.Start(ctx, inputPath, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("failed to start transcoder: %w", err)
	}

	m.sessions[key] = &sessionState{dir: dir, proc: proc, lastAccess: time.Now()}
	return &Session{Dir: dir}, nil
}

// Touch 更新 session 的 lastAccess（分段請求時呼叫）。
func (m *Manager) Touch(videoID, userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.sessions[sessionKey(videoID, userID)]; ok {
		st.lastAccess = time.Now()
	}
}

// SessionDir 回傳 session 暫存目錄；不存在回 ok=false。
func (m *Manager) SessionDir(videoID, userID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.sessions[sessionKey(videoID, userID)]
	if !ok {
		return "", false
	}
	return st.dir, true
}

// Sweep 清理 lastAccess 早於 now-idleTimeout 的 session。
func (m *Manager) Sweep(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, st := range m.sessions {
		if now.Sub(st.lastAccess) <= m.idleTimeout {
			continue
		}
		if err := st.proc.Stop(); err != nil {
			slog.Warn("failed to stop transcode proc", "key", key, "error", err)
		}
		if err := os.RemoveAll(st.dir); err != nil {
			slog.Warn("failed to remove session dir", "dir", st.dir, "error", err)
		}
		delete(m.sessions, key)
	}
}

// StartSweeper 每 idleTimeout/2 跑一次 Sweep，直到 ctx 取消。
func (m *Manager) StartSweeper(ctx context.Context) {
	interval := m.idleTimeout / 2
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				m.Sweep(t)
			}
		}
	}()
}

// sanitizeKey 把 session key 變成安全的單層目錄名（避免路徑穿越）。
func sanitizeKey(key string) string {
	out := make([]rune, 0, len(key))
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
