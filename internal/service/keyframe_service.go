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

// keyframeIndexer builds a Video's Keyframe Index when its Play Mode needs
// one (implemented by *KeyframeService).
type keyframeIndexer interface {
	// EnsureIndex probes absPath and stores v's Keyframe Index if v plays by
	// remux, and does nothing otherwise. Returns a wrapped probe/store error.
	EnsureIndex(ctx context.Context, v *model.Video, absPath string) error
}

// KeyframeService 擁有 Keyframe Index:何種 Video 需要它、何時建立。
// Media Lane 上的 Job(Import、Codec/Keyframe Backfill)以 EnsureIndex 同步建立;
// 播放時仍缺少的,由 LookupOrProbe 觸發非同步探測(去重)。
type KeyframeService struct {
	repo  keyframeIndexRepo
	probe keyframeProbeFunc
	now   func() time.Time

	mu       sync.Mutex
	inflight map[string]struct{}
	// failedAt remembers when each Video's latest probe failed, so polling
	// does not rerun ffprobe until probeFailureCooldown has passed.
	failedAt map[string]time.Time
}

// probeFailureCooldown is how long a failed probe stands before the next play
// probes again (a disk mounted late, a file replaced).
const probeFailureCooldown = 10 * time.Minute

// NewKeyframeService 建立 KeyframeService(使用真實 ffprobe 探測)。
func NewKeyframeService(repo keyframeIndexRepo) *KeyframeService {
	return &KeyframeService{
		repo:     repo,
		probe:    streaming.ProbeKeyframes,
		now:      time.Now,
		inflight: make(map[string]struct{}),
		failedAt: make(map[string]time.Time),
	}
}

// LookupOrProbe 回傳 Video 的 Segment Boundary 表。Keyframe Index 尚不存在時,
// 以 absPath 觸發背景探測並回 model.ErrStreamPreparing(呼叫端稍後重試);
// 最近一次探測在 probeFailureCooldown 內失敗時,不再重探,回
// model.ErrKeyframeIndexFailed;其他讀取失敗回 wrapped error。
func (s *KeyframeService) LookupOrProbe(ctx context.Context, videoID, absPath string) ([]model.SegmentBoundary, error) {
	idx, err := s.repo.Get(ctx, videoID)
	if errors.Is(err, model.ErrNotFound) {
		if s.failedRecently(videoID) {
			return nil, fmt.Errorf("keyframe index of video %s: %w", videoID, model.ErrKeyframeIndexFailed)
		}
		s.triggerProbe(videoID, absPath)
		return nil, fmt.Errorf("keyframe index of video %s: %w", videoID, model.ErrStreamPreparing)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get keyframe index of video %s: %w", videoID, err)
	}
	return idx.Segments, nil
}

// EnsureIndex builds v's Keyframe Index from absPath when v plays by remux,
// bounded by probeTimeout; other Play Modes never read one, so it is a no-op
// for them. Returns a wrapped probe or store error.
func (s *KeyframeService) EnsureIndex(ctx context.Context, v *model.Video, absPath string) error {
	if !usesKeyframeIndex(v) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	if err := s.indexFile(ctx, v.ID, absPath); err != nil {
		return fmt.Errorf("failed to build keyframe index of video %s: %w", v.ID, err)
	}
	return nil
}

// usesKeyframeIndex reports whether v plays by remux, the only Play Mode that
// reads a Keyframe Index.
func usesKeyframeIndex(v *model.Video) bool {
	return videoPlayMode(v) == model.PlayModeRemux
}

// triggerProbe 非同步探測 absPath 並寫入邊界表;同片探測進行中時重複觸發為 no-op。
func (s *KeyframeService) triggerProbe(videoID, absPath string) {
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
		if err := s.indexFile(ctx, videoID, absPath); err != nil {
			slog.Warn("keyframe probe failed", "video_id", videoID, "error", err)
		}
	}()
}

func (s *KeyframeService) failedRecently(videoID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, failed := s.failedAt[videoID]
	return failed && s.now().Sub(at) < probeFailureCooldown
}

// indexFile builds the Keyframe Index and records the outcome: a failure is
// remembered for probeFailureCooldown, a success forgets an earlier failure.
func (s *KeyframeService) indexFile(ctx context.Context, videoID, absPath string) error {
	err := s.buildIndex(ctx, videoID, absPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.failedAt[videoID] = s.now()
	} else {
		delete(s.failedAt, videoID)
	}
	return err
}

// buildIndex 同步探測 absPath、分組成 Segment Boundary 並寫入 Keyframe Index。
// 失敗回 wrapped error(探測、空表或寫入失敗)。
func (s *KeyframeService) buildIndex(ctx context.Context, videoID, absPath string) error {
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
