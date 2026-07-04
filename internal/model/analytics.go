package model

// AnalyticsQuery holds the tunable window/limit for an analytics request.
type AnalyticsQuery struct {
	Days  int
	Limit int
}

// DailyPoint is one calendar day of the trend (zero-filled for empty days).
type DailyPoint struct {
	Date       string  `json:"date"` // YYYY-MM-DD
	Views      int     `json:"views"`
	WatchHours float64 `json:"watch_hours"`
}

// TopVideo is one row of the most-watched leaderboard within the window.
type TopVideo struct {
	VideoID      string  `json:"video_id"`
	Title        string  `json:"title"`
	ThumbnailKey string  `json:"thumbnail_key"`
	Views        int     `json:"views"`
	WatchHours   float64 `json:"watch_hours"`
}

// TopTag is one tag's view count within the window. Tag IDs are integers.
type TopTag struct {
	TagID int    `json:"tag_id"`
	Name  string `json:"name"`
	Views int    `json:"views"`
}

// AnalyticsSummary is the full payload returned by GET /api/admin/analytics.
type AnalyticsSummary struct {
	RangeDays         int          `json:"range_days"`
	TotalViews        int          `json:"total_views"`
	TotalWatchHours   float64      `json:"total_watch_hours"`
	AvgCompletionRate float64      `json:"avg_completion_rate"`
	ActiveUsers       int          `json:"active_users"`
	DailyTrend        []DailyPoint `json:"daily_trend"`
	TopVideos         []TopVideo   `json:"top_videos"`
	TopTags           []TopTag     `json:"top_tags"`
}

// DailyRawRow is one present day from the DB before zero-fill (seconds, not hours).
type DailyRawRow struct {
	Views          int
	WatchedSeconds int64
}
