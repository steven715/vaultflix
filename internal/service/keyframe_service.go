package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/streaming"
)

// probeTimeout 是單片 keyframe 探測上限(冷讀約 15s/GB,6GB 檔約 90s,留足裕度)。
const probeTimeout = 30 * time.Minute

// keyframeProbeFunc 回傳 (keyframe pts 列表, 總長秒數, error)。
type keyframeProbeFunc func(ctx context.Context, absPath string) ([]float64, float64, error)

// keyframeIndexRepo 是 KeyframeService 所需的邊界表存取子集。
type keyframeIndexRepo interface {
	// Get 不存在時回 model.ErrNotFound。
	Get(ctx context.Context, videoID string) (*model.KeyframeIndex, error)
	Upsert(ctx context.Context, idx *model.KeyframeIndex) error
}

// KeyframeService 提供 Keyframe Index 查詢與非同步探測(去重);
// 對既有 Video 的批次補算見 KeyframeBackfill。
type KeyframeService struct {
	repo  keyframeIndexRepo
	probe keyframeProbeFunc

	mu       sync.Mutex
	inflight map[string]struct{}
}

// NewKeyframeService 建立 KeyframeService(使用真實 ffprobe 探測)。
func NewKeyframeService(repo keyframeIndexRepo) *KeyframeService {
	return &KeyframeService{
		repo:     repo,
		probe:    streaming.ProbeKeyframes,
		inflight: make(map[string]struct{}),
	}
}

// LookupOrProbe 回傳 Video 的 Segment Boundary 表。Keyframe Index 尚不存在時,
// 以 absPath 觸發背景探測並回 model.ErrStreamPreparing(呼叫端稍後重試);
// 其他讀取失敗回 wrapped error。
func (s *KeyframeService) LookupOrProbe(ctx context.Context, videoID, absPath string) ([]model.SegmentBoundary, error) {
	idx, err := s.repo.Get(ctx, videoID)
	if errors.Is(err, model.ErrNotFound) {
		s.TriggerProbe(videoID, absPath)
		return nil, fmt.Errorf("keyframe index of video %s: %w", videoID, model.ErrStreamPreparing)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get keyframe index of video %s: %w", videoID, err)
	}
	return idx.Segments, nil
}

// TriggerProbe 非同步探測 absPath 並寫入邊界表;同片探測進行中時重複觸發為 no-op。
func (s *KeyframeService) TriggerProbe(videoID, absPath string) {
	s.mu.Lock()
	if _, busy := s.inflight[videoID]; busy {
		s.mu.Unlock()
		return
	}
	s.inflight[videoID] = struct{}{}
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.inflight, videoID)
			s.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		defer cancel()
		if err := s.IndexFile(ctx, videoID, absPath); err != nil {
			slog.Warn("keyframe probe failed", "video_id", videoID, "error", err)
		}
	}()
}

// IndexFile 同步探測 absPath、分組成 Segment Boundary 並寫入 Keyframe Index。
// 失敗回 wrapped error(探測、空表或寫入失敗)。
func (s *KeyframeService) IndexFile(ctx context.Context, videoID, absPath string) error {
	start := time.Now()
	kf, total, err := s.probe(ctx, absPath)
	if err != nil {
		return fmt.Errorf("probe failed: %w", err)
	}
	segs := streaming.GroupSegments(kf, total, streaming.DefaultSegmentTarget)
	if len(segs) == 0 {
		return fmt.Errorf("empty segment table for video %s", videoID)
	}
	idx := &model.KeyframeIndex{VideoID: videoID, Segments: segs, ProbedAt: time.Now()}
	if err := s.repo.Upsert(ctx, idx); err != nil {
		return fmt.Errorf("failed to store keyframe index: %w", err)
	}
	slog.Info("keyframe index stored",
		"video_id", videoID,
		"segments", len(segs),
		"elapsed_ms", time.Since(start).Milliseconds(),
	)
	return nil
}
