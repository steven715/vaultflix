package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestRecordHeartbeat_ClampsDeltaToMax(t *testing.T) {
	var got model.HeartbeatInput
	repo := &mock.WatchSessionRepository{
		UpsertFunc: func(_ context.Context, in model.HeartbeatInput) error { got = in; return nil },
	}
	svc := NewWatchSessionService(repo)

	err := svc.RecordHeartbeat(context.Background(), model.HeartbeatInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1", PlayedDelta: 999, PositionSeconds: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PlayedDelta != MaxHeartbeatDelta {
		t.Fatalf("expected delta clamped to %d, got %d", MaxHeartbeatDelta, got.PlayedDelta)
	}
}

func TestRecordHeartbeat_RejectsNegativeDelta(t *testing.T) {
	repo := &mock.WatchSessionRepository{
		UpsertFunc: func(_ context.Context, _ model.HeartbeatInput) error { return nil },
	}
	svc := NewWatchSessionService(repo)
	err := svc.RecordHeartbeat(context.Background(), model.HeartbeatInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1", PlayedDelta: -1, PositionSeconds: 0,
	})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestRecordHeartbeat_MissingVideo(t *testing.T) {
	repo := &mock.WatchSessionRepository{
		UpsertFunc: func(_ context.Context, _ model.HeartbeatInput) error { return model.ErrNotFound },
	}
	svc := NewWatchSessionService(repo)
	err := svc.RecordHeartbeat(context.Background(), model.HeartbeatInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1", PlayedDelta: 5, PositionSeconds: 5,
	})
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
