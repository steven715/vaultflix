package streaming

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

// segmentGenTimeout 是單段 ffmpeg 產生的逾時上限(-c copy 正常亞秒級,
// 逾時代表輸入檔或磁碟異常)。
const segmentGenTimeout = 60 * time.Second

// SegmentCache 管理 per-video 的 on-demand HLS 分段快取。
// 同段並發請求去重(單一 ffmpeg);清理雙軌:idle sweep(整片目錄閒置逾時)
// + LRU 容量上限(總量超過 maxBytes 踢最久未存取的整片目錄)。
type SegmentCache struct {
	gen         SegmentGenerator
	cacheDir    string
	idleTimeout time.Duration
	maxBytes    int64 // <=0 表示不設容量上限

	mu       sync.Mutex
	videos   map[string]*videoCacheState
	inflight map[string]chan struct{}
}

type videoCacheState struct {
	dir        string
	sizeBytes  int64
	lastAccess time.Time
}

// NewSegmentCache 建立 SegmentCache 並清空 cacheDir 既有內容
// (重啟後冷快取,讓記憶體內大小帳目與磁碟一致)。
func NewSegmentCache(gen SegmentGenerator, cacheDir string, idleTimeout time.Duration, maxBytes int64) (*SegmentCache, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache dir: %w", err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(cacheDir, e.Name())); err != nil {
			return nil, fmt.Errorf("failed to clear cache dir: %w", err)
		}
	}
	return &SegmentCache{
		gen:         gen,
		cacheDir:    cacheDir,
		idleTimeout: idleTimeout,
		maxBytes:    maxBytes,
		videos:      make(map[string]*videoCacheState),
		inflight:    make(map[string]chan struct{}),
	}, nil
}

// EnsureSegment 回傳分段檔路徑;未快取時產生。同段並發請求只跑一次 ffmpeg。
func (c *SegmentCache) EnsureSegment(ctx context.Context, videoID, inputPath string, idx int, seg model.SegmentBoundary) (string, error) {
	name := SegmentName(idx)
	key := videoID + "/" + name
	dir := filepath.Join(c.cacheDir, sanitizeKey(videoID))
	path := filepath.Join(dir, name)

	for {
		c.mu.Lock()
		if st, ok := c.videos[videoID]; ok {
			st.lastAccess = time.Now()
		}
		if _, err := os.Stat(path); err == nil {
			c.mu.Unlock()
			return path, nil
		}
		ch, busy := c.inflight[key]
		if !busy {
			ch = make(chan struct{})
			c.inflight[key] = ch
			c.mu.Unlock()
			break
		}
		c.mu.Unlock()
		select {
		case <-ch: // 產生者完成(成功或失敗),回圈重查
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	genPath, err := c.generate(ctx, videoID, inputPath, dir, path, seg)

	c.mu.Lock()
	close(c.inflight[key])
	delete(c.inflight, key)
	c.mu.Unlock()
	return genPath, err
}

// generate 以暫存檔 + rename 產生分段,更新帳目並觸發 LRU 淘汰。
func (c *SegmentCache) generate(ctx context.Context, videoID, inputPath, dir, path string, seg model.SegmentBoundary) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create video cache dir: %w", err)
	}
	tmp := path + ".tmp"
	genCtx, cancel := context.WithTimeout(ctx, segmentGenTimeout)
	defer cancel()
	if err := c.gen.Generate(genCtx, inputPath, tmp, seg.Start, seg.Duration); err != nil {
		if rmErr := os.Remove(tmp); rmErr != nil && !os.IsNotExist(rmErr) {
			slog.Warn("failed to remove tmp segment", "path", tmp, "error", rmErr)
		}
		return "", fmt.Errorf("failed to generate segment: %w", err)
	}
	info, err := os.Stat(tmp)
	if err != nil {
		return "", fmt.Errorf("generated segment missing: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("failed to finalize segment: %w", err)
	}

	c.mu.Lock()
	st, ok := c.videos[videoID]
	if !ok {
		st = &videoCacheState{dir: dir}
		c.videos[videoID] = st
	}
	st.sizeBytes += info.Size()
	st.lastAccess = time.Now()
	c.evictLocked(videoID)
	c.mu.Unlock()
	return path, nil
}

// evictLocked 在總量超過 maxBytes 時踢除最久未存取的整片目錄
// (跳過 current,以及任何仍有分段產生中的影片,避免刪掉正在寫入的目錄)。
// 呼叫端須持有 c.mu。
func (c *SegmentCache) evictLocked(current string) {
	if c.maxBytes <= 0 {
		return
	}
	for c.totalLocked() > c.maxBytes {
		victim := ""
		var oldest time.Time
		for id, st := range c.videos {
			if id == current || c.hasInflightLocked(id) {
				continue
			}
			if victim == "" || st.lastAccess.Before(oldest) {
				victim, oldest = id, st.lastAccess
			}
		}
		if victim == "" {
			return
		}
		st := c.videos[victim]
		if err := os.RemoveAll(st.dir); err != nil {
			slog.Error("failed to evict segment cache dir", "dir", st.dir, "error", err)
		}
		delete(c.videos, victim)
		slog.Info("segment cache evicted video", "video_id", victim, "freed_bytes", st.sizeBytes)
	}
}

// hasInflightLocked 回報 videoID 是否有任一分段正在產生中。
// 呼叫端須持有 c.mu(inflight 只在持有 c.mu 時被修改)。
func (c *SegmentCache) hasInflightLocked(videoID string) bool {
	prefix := videoID + "/"
	for key := range c.inflight {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (c *SegmentCache) totalLocked() int64 {
	var total int64
	for _, st := range c.videos {
		total += st.sizeBytes
	}
	return total
}

// Sweep 清理 lastAccess 早於 now-idleTimeout 的影片快取目錄。
func (c *SegmentCache) Sweep(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, st := range c.videos {
		if now.Sub(st.lastAccess) <= c.idleTimeout {
			continue
		}
		if err := os.RemoveAll(st.dir); err != nil {
			slog.Error("failed to remove idle cache dir", "dir", st.dir, "error", err)
		}
		delete(c.videos, id)
	}
}

// StartSweeper 每 idleTimeout/2 跑一次 Sweep,直到 ctx 取消。
func (c *SegmentCache) StartSweeper(ctx context.Context) {
	interval := c.idleTimeout / 2
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
				c.Sweep(t)
			}
		}
	}()
}

// sanitizeKey 把 videoID 變成安全的單層目錄名(避免路徑穿越)。
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
