package service

import (
	"context"
	"fmt"
	"net"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
)

// isValidPlayMode reports whether m is one of the closed set the ingest
// endpoint accepts.
func isValidPlayMode(m string) bool {
	switch model.PlayMode(m) {
	case model.PlayModeDirect, model.PlayModeRemux, model.PlayModeTranscode:
		return true
	default:
		return false
	}
}

type PlaybackTelemetryService struct {
	repo repository.PlaybackTelemetryRepository
}

func NewPlaybackTelemetryService(repo repository.PlaybackTelemetryRepository) *PlaybackTelemetryService {
	return &PlaybackTelemetryService{repo: repo}
}

// Record validates and clamps one session summary, derives NetworkScope from
// RemoteIP, then upserts it. Returns model.ErrInvalidInput for a missing id or
// unknown play_mode, and propagates model.ErrNotFound when the video is gone.
func (s *PlaybackTelemetryService) Record(ctx context.Context, in model.PlaybackTelemetryInput) error {
	if in.SessionID == "" || in.UserID == "" || in.VideoID == "" {
		return fmt.Errorf("telemetry requires session/user/video: %w", model.ErrInvalidInput)
	}
	if !isValidPlayMode(in.PlayMode) {
		return fmt.Errorf("invalid play_mode %q: %w", in.PlayMode, model.ErrInvalidInput)
	}
	in.NetworkScope = classifyNetworkScope(in.RemoteIP)
	clampTelemetry(&in)
	if err := s.repo.Insert(ctx, in); err != nil {
		return fmt.Errorf("failed to record telemetry: %w", err)
	}
	return nil
}

// Summary returns per-play_mode aggregates for the (handler-clamped) window.
func (s *PlaybackTelemetryService) Summary(ctx context.Context, q model.TelemetryQuery) (*model.TelemetrySummary, error) {
	stats, err := s.repo.Aggregate(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate telemetry: %w", err)
	}
	return &model.TelemetrySummary{RangeDays: q.Days, Scope: q.Scope, ByPlayMode: stats}, nil
}

// clampTelemetry floors negative counters/durations to 0, defending against a
// buggy or hostile client. Nil optional fields pass through untouched.
func clampTelemetry(in *model.PlaybackTelemetryInput) {
	if in.WatchedMs < 0 {
		in.WatchedMs = 0
	}
	if in.RebufferCount < 0 {
		in.RebufferCount = 0
	}
	if in.RebufferMs < 0 {
		in.RebufferMs = 0
	}
	if in.TTFFMs != nil && *in.TTFFMs < 0 {
		*in.TTFFMs = 0
	}
	if in.AvgDownlinkMbps != nil && *in.AvgDownlinkMbps < 0 {
		*in.AvgDownlinkMbps = 0
	}
}

// classifyNetworkScope maps a request source IP to a coarse network location.
// Loopback, private (RFC1918), unique-local (fc00::/7) and link-local addresses
// are "lan"; any other routable address is "external"; an empty or unparseable
// value is "unknown".
func classifyNetworkScope(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "unknown"
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
		return "lan"
	}
	return "external"
}
