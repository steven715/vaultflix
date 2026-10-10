package main

import (
	"net/http"

	"github.com/steven/vaultflix/internal/handler"
	"github.com/steven/vaultflix/internal/middleware"
)

// apiHandlers are the handlers the /api route table dispatches to.
type apiHandlers struct {
	auth              *handler.AuthHandler
	video             *handler.VideoHandler
	hls               *handler.HLSHandler
	tag               *handler.TagHandler
	history           *handler.HistoryHandler
	watchSession      *handler.WatchSessionHandler
	playbackTelemetry *handler.PlaybackTelemetryHandler
	favorite          *handler.FavoriteHandler
	user              *handler.UserHandler
	recommendation    *handler.RecommendationHandler
	mediaSource       *handler.MediaSourceHandler
	backfill          *handler.BackfillHandler
	analytics         *handler.AnalyticsHandler
	enrichment        *handler.EnrichmentHandler
	ws                *handler.WSHandler
}

const (
	get = http.MethodGet
	pst = http.MethodPost
	put = http.MethodPut
	del = http.MethodDelete
)

// apiRoutes is the one table of protected /api routes: path, handler, whether
// the viewer Role may call it (admin may call every route), and whether a
// scope=stream token may (ADR-0013). Paths are relative to /api.
func apiRoutes(h apiHandlers) []middleware.Route {
	type r = middleware.Route
	return []middleware.Route{
		r{Method: get, Path: "/me", Handler: h.auth.Me, Viewer: true},

		// Library
		r{Method: get, Path: "/videos", Handler: h.video.List, Viewer: true},
		r{Method: get, Path: "/videos/:id", Handler: h.video.GetByID, Viewer: true},
		r{Method: put, Path: "/videos/:id", Handler: h.video.Update},
		r{Method: del, Path: "/videos/:id", Handler: h.video.Delete},
		r{Method: pst, Path: "/videos/import", Handler: h.video.Import},
		r{Method: get, Path: "/import-jobs/active", Handler: h.video.GetActiveImportJob},
		r{Method: get, Path: "/import-jobs/:id", Handler: h.video.GetImportJob},
		r{Method: pst, Path: "/videos/:id/tags", Handler: h.tag.AddVideoTag},
		r{Method: del, Path: "/videos/:id/tags/:tagId", Handler: h.tag.RemoveVideoTag},
		r{Method: get, Path: "/tags", Handler: h.tag.List, Viewer: true},
		r{Method: pst, Path: "/tags", Handler: h.tag.Create},

		// Playback
		r{Method: get, Path: "/videos/:id/stream", Handler: h.video.Stream, Viewer: true, StreamToken: true},
		r{Method: get, Path: "/videos/:id/hls/index.m3u8", Handler: h.hls.Playlist, StreamToken: true},
		r{Method: get, Path: "/videos/:id/hls/:segment", Handler: h.hls.Segment, StreamToken: true},
		r{Method: get, Path: "/videos/:id/stream-token", Handler: h.auth.StreamToken, Viewer: true},

		// Watching
		r{Method: pst, Path: "/watch-history", Handler: h.history.SaveProgress, Viewer: true},
		r{Method: get, Path: "/watch-history", Handler: h.history.List, Viewer: true},
		r{Method: del, Path: "/watch-history", Handler: h.history.ClearHistory, Viewer: true},
		r{Method: pst, Path: "/watch-sessions/heartbeat", Handler: h.watchSession.Heartbeat, Viewer: true},
		r{Method: pst, Path: "/playback/telemetry", Handler: h.playbackTelemetry.Record, Viewer: true},
		r{Method: get, Path: "/admin/playback/telemetry", Handler: h.playbackTelemetry.Summary},
		r{Method: get, Path: "/favorites", Handler: h.favorite.List, Viewer: true},
		r{Method: pst, Path: "/favorites", Handler: h.favorite.Add, Viewer: true},
		r{Method: del, Path: "/favorites/:videoId", Handler: h.favorite.Remove, Viewer: true},

		// Users
		r{Method: get, Path: "/users", Handler: h.user.List},
		r{Method: pst, Path: "/users", Handler: h.user.Create},
		r{Method: del, Path: "/users/:id", Handler: h.user.Delete},
		r{Method: put, Path: "/users/:id/enable", Handler: h.user.Enable},
		r{Method: put, Path: "/users/:id/password", Handler: h.user.ResetPassword},

		// Daily Recommendations
		r{Method: get, Path: "/recommendations/today", Handler: h.recommendation.GetToday, Viewer: true},
		r{Method: get, Path: "/recommendations", Handler: h.recommendation.ListByDate},
		r{Method: pst, Path: "/recommendations", Handler: h.recommendation.Create},
		r{Method: put, Path: "/recommendations/:id", Handler: h.recommendation.UpdateSortOrder},
		r{Method: del, Path: "/recommendations/:id", Handler: h.recommendation.Delete},

		// Media Sources
		r{Method: get, Path: "/media-sources", Handler: h.mediaSource.List},
		r{Method: pst, Path: "/media-sources", Handler: h.mediaSource.Create},
		r{Method: put, Path: "/media-sources/:id", Handler: h.mediaSource.Update},
		r{Method: del, Path: "/media-sources/:id", Handler: h.mediaSource.Delete},

		// Backfill Jobs and analytics
		r{Method: pst, Path: "/admin/backfill-jobs", Handler: h.backfill.Start},
		r{Method: get, Path: "/admin/backfill-jobs/active", Handler: h.backfill.GetActive},
		r{Method: pst, Path: "/admin/backfill-jobs/:id/cancel", Handler: h.backfill.Cancel},
		r{Method: get, Path: "/admin/analytics", Handler: h.analytics.Get},

		// Enrichment
		r{Method: pst, Path: "/videos/:id/enrich", Handler: h.enrichment.EnrichVideo},
		r{Method: get, Path: "/videos/:id/suggestions", Handler: h.enrichment.ListSuggestions},
		r{Method: pst, Path: "/videos/:id/suggestions/:sid/accept", Handler: h.enrichment.AcceptSuggestion},
		r{Method: del, Path: "/videos/:id/suggestions/:sid", Handler: h.enrichment.RejectSuggestion},
		r{Method: pst, Path: "/enrich-jobs", Handler: h.enrichment.StartBatch},
		r{Method: get, Path: "/enrich-jobs/active", Handler: h.enrichment.ActiveJob},
		r{Method: del, Path: "/enrich-jobs/:jid", Handler: h.enrichment.CancelBatch},

		r{Method: get, Path: "/ws", Handler: h.ws.HandleWebSocket, Viewer: true},
	}
}
