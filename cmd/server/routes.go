package main

import (
	"net/http"

	"github.com/gin-gonic/gin"

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

// apiRoutes is the one table of protected /api routes: path, handler, whether
// the viewer Role may call it (admin may call every route), and whether a
// Stream Token may (ADR-0013). Paths are relative to /api.
func apiRoutes(h apiHandlers) []middleware.Route {
	var routes []middleware.Route
	for _, group := range [][]middleware.Route{
		libraryRoutes(h), playbackRoutes(h), watchingRoutes(h), adminRoutes(h), enrichmentRoutes(h),
	} {
		routes = append(routes, group...)
	}
	return routes
}

// route builds one table row; the zero access is admin-only.
func route(method, path string, handler gin.HandlerFunc, access ...access) middleware.Route {
	r := middleware.Route{Method: method, Path: path, Handler: handler}
	for _, a := range access {
		a(&r)
	}
	return r
}

type access func(*middleware.Route)

// viewer lets the viewer Role call the route.
func viewer(r *middleware.Route) { r.Viewer = true }

// streamToken lets a Stream Token call the route for its own Video.
func streamToken(r *middleware.Route) { r.StreamToken = true }

func libraryRoutes(h apiHandlers) []middleware.Route {
	return []middleware.Route{
		route(http.MethodGet, "/me", h.auth.Me, viewer),
		route(http.MethodGet, "/videos", h.video.List, viewer),
		route(http.MethodGet, "/videos/:id", h.video.GetByID, viewer),
		route(http.MethodPut, "/videos/:id", h.video.Update),
		route(http.MethodDelete, "/videos/:id", h.video.Delete),
		route(http.MethodPost, "/videos/import", h.video.Import),
		route(http.MethodGet, "/import-jobs/active", h.video.GetActiveImportJob),
		route(http.MethodGet, "/import-jobs/:id", h.video.GetImportJob),
		route(http.MethodPost, "/videos/:id/tags", h.tag.AddVideoTag),
		route(http.MethodDelete, "/videos/:id/tags/:tagId", h.tag.RemoveVideoTag),
		route(http.MethodGet, "/tags", h.tag.List, viewer),
		route(http.MethodPost, "/tags", h.tag.Create),
	}
}

func playbackRoutes(h apiHandlers) []middleware.Route {
	return []middleware.Route{
		route(http.MethodGet, "/videos/:id/stream", h.video.Stream, viewer, streamToken),
		route(http.MethodGet, "/videos/:id/hls/index.m3u8", h.hls.Playlist, streamToken),
		route(http.MethodGet, "/videos/:id/hls/:segment", h.hls.Segment, streamToken),
		route(http.MethodGet, "/videos/:id/stream-token", h.auth.StreamToken, viewer),
	}
}

func watchingRoutes(h apiHandlers) []middleware.Route {
	return []middleware.Route{
		route(http.MethodPost, "/watch-history", h.history.SaveProgress, viewer),
		route(http.MethodGet, "/watch-history", h.history.List, viewer),
		route(http.MethodDelete, "/watch-history", h.history.ClearHistory, viewer),
		route(http.MethodPost, "/watch-sessions/heartbeat", h.watchSession.Heartbeat, viewer),
		route(http.MethodPost, "/playback/telemetry", h.playbackTelemetry.Record, viewer),
		route(http.MethodGet, "/admin/playback/telemetry", h.playbackTelemetry.Summary),
		route(http.MethodGet, "/favorites", h.favorite.List, viewer),
		route(http.MethodPost, "/favorites", h.favorite.Add, viewer),
		route(http.MethodDelete, "/favorites/:videoId", h.favorite.Remove, viewer),
		route(http.MethodGet, "/ws", h.ws.HandleWebSocket, viewer),
	}
}

// adminRoutes: Users, Daily Recommendations, Media Sources, Backfill Jobs,
// analytics.
func adminRoutes(h apiHandlers) []middleware.Route {
	return []middleware.Route{
		route(http.MethodGet, "/users", h.user.List),
		route(http.MethodPost, "/users", h.user.Create),
		route(http.MethodDelete, "/users/:id", h.user.Delete),
		route(http.MethodPut, "/users/:id/enable", h.user.Enable),
		route(http.MethodPut, "/users/:id/password", h.user.ResetPassword),

		route(http.MethodGet, "/recommendations/today", h.recommendation.GetToday, viewer),
		route(http.MethodGet, "/recommendations", h.recommendation.ListByDate),
		route(http.MethodPost, "/recommendations", h.recommendation.Create),
		route(http.MethodPut, "/recommendations/:id", h.recommendation.UpdateSortOrder),
		route(http.MethodDelete, "/recommendations/:id", h.recommendation.Delete),

		route(http.MethodGet, "/media-sources", h.mediaSource.List),
		route(http.MethodPost, "/media-sources", h.mediaSource.Create),
		route(http.MethodPut, "/media-sources/:id", h.mediaSource.Update),
		route(http.MethodDelete, "/media-sources/:id", h.mediaSource.Delete),

		route(http.MethodPost, "/admin/backfill-jobs", h.backfill.Start),
		route(http.MethodGet, "/admin/backfill-jobs/active", h.backfill.GetActive),
		route(http.MethodPost, "/admin/backfill-jobs/:id/cancel", h.backfill.Cancel),
		route(http.MethodGet, "/admin/analytics", h.analytics.Get),
	}
}

func enrichmentRoutes(h apiHandlers) []middleware.Route {
	return []middleware.Route{
		route(http.MethodPost, "/videos/:id/enrich", h.enrichment.EnrichVideo),
		route(http.MethodGet, "/videos/:id/suggestions", h.enrichment.ListSuggestions),
		route(http.MethodPost, "/videos/:id/suggestions/:sid/accept", h.enrichment.AcceptSuggestion),
		route(http.MethodDelete, "/videos/:id/suggestions/:sid", h.enrichment.RejectSuggestion),
		route(http.MethodPost, "/enrich-jobs", h.enrichment.StartBatch),
		route(http.MethodGet, "/enrich-jobs/active", h.enrichment.ActiveJob),
		route(http.MethodDelete, "/enrich-jobs/:jid", h.enrichment.CancelBatch),
	}
}
