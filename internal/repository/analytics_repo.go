package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// AnalyticsRepository runs read-only aggregations over watch_sessions.
// The window is the trailing `days` on started_at; a "view" is a session with
// watched_seconds >= 10.
type AnalyticsRepository interface {
	KPIs(ctx context.Context, days int) (totalViews int, totalWatchedSeconds int64, avgCompletion float64, activeUsers int, err error)
	DailyRaw(ctx context.Context, days int) (map[string]model.DailyRawRow, error)
	TopVideos(ctx context.Context, days, limit int) ([]model.TopVideo, error)
	TopTags(ctx context.Context, days, limit int) ([]model.TopTag, error)
}

const queryAnalyticsKPIs = `
    SELECT
        COUNT(*) FILTER (WHERE watched_seconds >= 10) AS total_views,
        COALESCE(SUM(watched_seconds), 0) AS total_watched_seconds,
        COALESCE(AVG(LEAST(max_progress_seconds::float / video_duration_seconds, 1.0))
            FILTER (WHERE video_duration_seconds > 0 AND watched_seconds >= 10), 0) AS avg_completion,
        COUNT(DISTINCT user_id) FILTER (WHERE watched_seconds >= 10) AS active_users
    FROM watch_sessions
    WHERE started_at >= NOW() - make_interval(days => $1)
`

const queryAnalyticsDaily = `
    SELECT to_char(started_at::date, 'YYYY-MM-DD') AS day,
           COUNT(*) FILTER (WHERE watched_seconds >= 10) AS views,
           COALESCE(SUM(watched_seconds), 0) AS watched_seconds
    FROM watch_sessions
    WHERE started_at >= NOW() - make_interval(days => $1)
    GROUP BY started_at::date
`

const queryAnalyticsTopVideos = `
    SELECT v.id, v.title, v.thumbnail_key,
           COUNT(*) FILTER (WHERE ws.watched_seconds >= 10) AS views,
           COALESCE(SUM(ws.watched_seconds), 0) AS watched_seconds
    FROM watch_sessions ws
    JOIN videos v ON v.id = ws.video_id
    WHERE ws.started_at >= NOW() - make_interval(days => $1)
    GROUP BY v.id, v.title, v.thumbnail_key
    HAVING SUM(ws.watched_seconds) > 0
    ORDER BY SUM(ws.watched_seconds) DESC
    LIMIT $2
`

const queryAnalyticsTopTags = `
    SELECT t.id, t.name,
           COUNT(*) FILTER (WHERE ws.watched_seconds >= 10) AS views
    FROM watch_sessions ws
    JOIN video_tags vt ON vt.video_id = ws.video_id
    JOIN tags t ON t.id = vt.tag_id
    WHERE ws.started_at >= NOW() - make_interval(days => $1)
    GROUP BY t.id, t.name
    HAVING COUNT(*) FILTER (WHERE ws.watched_seconds >= 10) > 0
    ORDER BY views DESC
    LIMIT $2
`

type analyticsRepository struct {
	pool *pgxpool.Pool
}

func NewAnalyticsRepository(pool *pgxpool.Pool) AnalyticsRepository {
	return &analyticsRepository{pool: pool}
}

func (r *analyticsRepository) KPIs(ctx context.Context, days int) (int, int64, float64, int, error) {
	var views, active int
	var watched int64
	var avg float64
	err := r.pool.QueryRow(ctx, queryAnalyticsKPIs, days).Scan(&views, &watched, &avg, &active)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to query analytics kpis: %w", err)
	}
	return views, watched, avg, active, nil
}

func (r *analyticsRepository) DailyRaw(ctx context.Context, days int) (map[string]model.DailyRawRow, error) {
	rows, err := r.pool.Query(ctx, queryAnalyticsDaily, days)
	if err != nil {
		return nil, fmt.Errorf("failed to query analytics daily: %w", err)
	}
	defer rows.Close()

	out := make(map[string]model.DailyRawRow)
	for rows.Next() {
		var day string
		var row model.DailyRawRow
		if err := rows.Scan(&day, &row.Views, &row.WatchedSeconds); err != nil {
			return nil, fmt.Errorf("failed to scan daily row: %w", err)
		}
		out[day] = row
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate daily rows: %w", err)
	}
	return out, nil
}

func (r *analyticsRepository) TopVideos(ctx context.Context, days, limit int) ([]model.TopVideo, error) {
	rows, err := r.pool.Query(ctx, queryAnalyticsTopVideos, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top videos: %w", err)
	}
	defer rows.Close()

	out := []model.TopVideo{}
	for rows.Next() {
		var v model.TopVideo
		var watched int64
		if err := rows.Scan(&v.VideoID, &v.Title, &v.ThumbnailKey, &v.Views, &watched); err != nil {
			return nil, fmt.Errorf("failed to scan top video: %w", err)
		}
		v.WatchHours = float64(watched) / 3600.0
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate top video rows: %w", err)
	}
	return out, nil
}

func (r *analyticsRepository) TopTags(ctx context.Context, days, limit int) ([]model.TopTag, error) {
	rows, err := r.pool.Query(ctx, queryAnalyticsTopTags, days, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top tags: %w", err)
	}
	defer rows.Close()

	out := []model.TopTag{}
	for rows.Next() {
		var t model.TopTag
		if err := rows.Scan(&t.TagID, &t.Name, &t.Views); err != nil {
			return nil, fmt.Errorf("failed to scan top tag: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate top tag rows: %w", err)
	}
	return out, nil
}
