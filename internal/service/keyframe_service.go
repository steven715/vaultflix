package service

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
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

// keyframeVideoRepo 是 KeyframeService 所需的 video 查詢子集。
type keyframeVideoRepo interface {
	ListKeyframeCandidates(ctx context.Context, limit int) ([]model.Video, error)
}

// keyframeSourceRepo 是 KeyframeService 所需的 media source 查詢子集。
type keyframeSourceRepo interface {
	FindByID(ctx context.Context, id string) (*model.MediaSource, error)
}

// KeyframeService 提供邊界表查詢、非同步探測(去重)與 backfill。
type KeyframeService struct {
	repo       keyframeIndexRepo
	videoRepo  keyframeVideoRepo
	sourceRepo keyframeSourceRepo
	probe      keyframeProbeFunc

	mu       sync.Mutex
	inflight map[string]struct{}
}

// NewKeyframeService 建立 KeyframeService(使用真實 ffprobe 探測)。
func NewKeyframeService(repo keyframeIndexRepo, videoRepo keyframeVideoRepo, sourceRepo keyframeSourceRepo) *KeyframeService {
	return &KeyframeService{
		repo:       repo,
		videoRepo:  videoRepo,
		sourceRepo: sourceRepo,
		probe:      streaming.ProbeKeyframes,
		inflight:   make(map[string]struct{}),
	}
}

// GetSegments 回傳邊界表;無資料回 model.ErrNotFound(呼叫端可觸發 TriggerProbe)。
func (s *KeyframeService) GetSegments(ctx context.Context, videoID string) ([]model.SegmentBoundary, error) {
	idx, err := s.repo.Get(ctx, videoID)
	if err != nil {
		return nil, err
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
		if err := s.probeAndStore(ctx, videoID, absPath); err != nil {
			slog.Warn("keyframe probe failed", "video_id", videoID, "error", err)
		}
	}()
}

// probeAndStore 探測、分組並寫入邊界表。
func (s *KeyframeService) probeAndStore(ctx context.Context, videoID, absPath string) error {
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

// RunBackfill 掃描缺邊界表的 remux 影片並循序探測,回傳 (processed, failed)。
func (s *KeyframeService) RunBackfill(ctx context.Context) (int, int, error) {
	videos, err := s.videoRepo.ListKeyframeCandidates(ctx, 10000)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list keyframe candidates: %w", err)
	}
	processed, failed := 0, 0
	for _, v := range videos {
		container := strings.TrimPrefix(filepath.Ext(v.OriginalFilename), ".")
		if ClassifyPlayMode(container, v.VideoCodec, v.AudioCodec) != model.PlayModeRemux {
			continue
		}
		source, err := s.sourceRepo.FindByID(ctx, *v.SourceID)
		if err != nil {
			slog.Warn("keyframe backfill: source lookup failed", "video_id", v.ID, "error", err)
			failed++
			continue
		}
		abs := filepath.Clean(filepath.Join(source.MountPath, *v.FilePath))
		if err := s.probeAndStore(ctx, v.ID, abs); err != nil {
			slog.Warn("keyframe backfill: probe failed", "video_id", v.ID, "error", err)
			failed++
			continue
		}
		processed++
	}
	slog.Info("keyframe backfill complete", "processed", processed, "failed", failed)
	return processed, failed, nil
}
