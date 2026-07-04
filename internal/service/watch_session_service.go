package service

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// MaxHeartbeatDelta caps one heartbeat's counted play time (15s cadence x1.5)
// so seeks and background tabs cannot inflate accumulated watch time.
const MaxHeartbeatDelta = 22

type WatchSessionService struct {
	repo repository.WatchSessionRepository
}

func NewWatchSessionService(repo repository.WatchSessionRepository) *WatchSessionService {
	return &WatchSessionService{repo: repo}
}

// RecordHeartbeat validates and clamps a heartbeat, then upserts its session.
func (s *WatchSessionService) RecordHeartbeat(ctx context.Context, in model.HeartbeatInput) error {
	if in.SessionID == "" || in.UserID == "" || in.VideoID == "" || in.PlayedDelta < 0 {
		return fmt.Errorf("heartbeat requires session/user/video and non-negative delta: %w", model.ErrInvalidInput)
	}
	if in.PlayedDelta > MaxHeartbeatDelta {
		in.PlayedDelta = MaxHeartbeatDelta
	}
	if in.PositionSeconds < 0 {
		in.PositionSeconds = 0
	}
	if err := s.repo.Upsert(ctx, in); err != nil {
		return fmt.Errorf("failed to record heartbeat: %w", err)
	}
	return nil
}
