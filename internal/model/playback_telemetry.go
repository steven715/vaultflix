package model

// PlaybackTelemetryInput is one viewing session's terminal quality summary,
// emitted once when the session ends. RemoteIP is the request source address;
// the service layer fills NetworkScope by classifying it.
type PlaybackTelemetryInput struct {
	SessionID        string
	UserID           string
	VideoID          string
	PlayMode         string
	RemoteIP         string
	NetworkScope     string
	TTFFMs           *int // nil = never reached first frame
	WatchedMs        int  // actual playing time (rebuffer-ratio denominator)
	RebufferCount    int
	RebufferMs       int      // cumulative stall time (excludes initial buffer + seeks)
	AvgDownlinkMbps  *float64 // nil = no throughput samples
	FatalErrorFamily *string  // nil | "starved" | "codec"
}

// TelemetryQuery holds the tunable window and optional scope filter for an
// aggregate read. Scope == "" means all scopes.
type TelemetryQuery struct {
	Days  int
	Scope string
}

// PlayModeStats is one play_mode's aggregated quality metrics within the window.
// Pointer fields are null when no session contributed a value (e.g. no TTFF).
type PlayModeStats struct {
	PlayMode      string   `json:"play_mode"`
	Sessions      int64    `json:"sessions"`
	TTFFP50Ms     *float64 `json:"ttff_p50_ms"`
	TTFFP95Ms     *float64 `json:"ttff_p95_ms"`
	RebufferRatio *float64 `json:"rebuffer_ratio"`
	AvgMbps       *float64 `json:"avg_mbps"`
}

// TelemetrySummary is the payload returned by GET /api/admin/playback/telemetry.
type TelemetrySummary struct {
	RangeDays  int             `json:"range_days"`
	Scope      string          `json:"scope"`
	ByPlayMode []PlayModeStats `json:"by_play_mode"`
}
