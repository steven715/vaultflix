package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func ptrInt(v int) *int { return &v }

func TestClassifyNetworkScope(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want string
	}{
		{"loopback v4", "127.0.0.1", "lan"},
		{"loopback v6", "::1", "lan"},
		{"rfc1918 10", "10.1.2.3", "lan"},
		{"rfc1918 172.16", "172.16.5.5", "lan"},
		{"rfc1918 172.31", "172.31.255.1", "lan"},
		{"rfc1918 192.168", "192.168.0.7", "lan"},
		{"ula v6", "fc00::1", "lan"},
		{"link-local v4", "169.254.1.1", "lan"},
		{"link-local v6", "fe80::1", "lan"},
		{"public v4", "8.8.8.8", "external"},
		{"public v6", "2606:4700:4700::1111", "external"},
		{"empty", "", "unknown"},
		{"garbage", "not-an-ip", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyNetworkScope(tc.ip); got != tc.want {
				t.Fatalf("classifyNetworkScope(%q) = %q, want %q", tc.ip, got, tc.want)
			}
		})
	}
}

func TestRecord_ValidUpsertsWithDerivedScope(t *testing.T) {
	var captured model.PlaybackTelemetryInput
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, in model.PlaybackTelemetryInput) error {
			captured = in
			return nil
		},
	}
	svc := NewPlaybackTelemetryService(repo)
	err := svc.Record(context.Background(), model.PlaybackTelemetryInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1",
		PlayMode: "remux", RemoteIP: "8.8.8.8", TTFFMs: ptrInt(1200), WatchedMs: 60000,
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if captured.NetworkScope != "external" {
		t.Fatalf("NetworkScope = %q, want external", captured.NetworkScope)
	}
}

func TestRecord_InvalidPlayMode(t *testing.T) {
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, _ model.PlaybackTelemetryInput) error {
			t.Fatal("Insert must not be called for invalid input")
			return nil
		},
	}
	svc := NewPlaybackTelemetryService(repo)
	err := svc.Record(context.Background(), model.PlaybackTelemetryInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1", PlayMode: "bogus", RemoteIP: "127.0.0.1",
	})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestRecord_MissingIDs(t *testing.T) {
	svc := NewPlaybackTelemetryService(&mock.PlaybackTelemetryRepository{})
	err := svc.Record(context.Background(), model.PlaybackTelemetryInput{PlayMode: "direct"})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
}

func TestRecord_ClampsNegatives(t *testing.T) {
	var captured model.PlaybackTelemetryInput
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, in model.PlaybackTelemetryInput) error {
			captured = in
			return nil
		},
	}
	svc := NewPlaybackTelemetryService(repo)
	neg := -50
	negMbps := -3.5
	err := svc.Record(context.Background(), model.PlaybackTelemetryInput{
		SessionID: "s1", UserID: "u1", VideoID: "v1", PlayMode: "direct", RemoteIP: "10.0.0.1",
		WatchedMs: -10, RebufferCount: -1, RebufferMs: -5, TTFFMs: &neg, AvgDownlinkMbps: &negMbps,
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}
	if captured.WatchedMs != 0 || captured.RebufferCount != 0 || captured.RebufferMs != 0 || *captured.TTFFMs != 0 {
		t.Fatalf("negatives not clamped: %+v", captured)
	}
	if captured.AvgDownlinkMbps == nil || *captured.AvgDownlinkMbps != 0 {
		t.Fatalf("AvgDownlinkMbps not clamped: %+v", captured.AvgDownlinkMbps)
	}
}

func TestRecord_PropagatesNotFound(t *testing.T) {
	repo := &mock.PlaybackTelemetryRepository{
		InsertFunc: func(_ context.Context, _ model.PlaybackTelemetryInput) error {
			return model.ErrNotFound
		},
	}
	svc := NewPlaybackTelemetryService(repo)
	err := svc.Record(context.Background(), model.PlaybackTelemetryInput{
		SessionID: "s1", UserID: "u1", VideoID: "ghost", PlayMode: "direct", RemoteIP: "10.0.0.1",
	})
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestSummary_ShapesQueryResult(t *testing.T) {
	ratio := 0.05
	repo := &mock.PlaybackTelemetryRepository{
		AggregateFunc: func(_ context.Context, q model.TelemetryQuery) ([]model.PlayModeStats, error) {
			if q.Days != 7 || q.Scope != "external" {
				t.Fatalf("query not passed through: %+v", q)
			}
			return []model.PlayModeStats{{PlayMode: "remux", Sessions: 3, RebufferRatio: &ratio}}, nil
		},
	}
	svc := NewPlaybackTelemetryService(repo)
	out, err := svc.Summary(context.Background(), model.TelemetryQuery{Days: 7, Scope: "external"})
	if err != nil {
		t.Fatalf("Summary error: %v", err)
	}
	if out.RangeDays != 7 || out.Scope != "external" || len(out.ByPlayMode) != 1 || out.ByPlayMode[0].PlayMode != "remux" {
		t.Fatalf("unexpected summary: %+v", out)
	}
}
