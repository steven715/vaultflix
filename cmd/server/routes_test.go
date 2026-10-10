package main

import (
	"net/http"
	"sort"
	"strings"
	"testing"
)

func routeKeys(filter func(viewer, stream bool) bool) []string {
	var keys []string
	for _, r := range apiRoutes(apiHandlers{}) {
		if filter(r.Viewer, r.StreamToken) {
			keys = append(keys, r.Method+" "+r.Path)
		}
	}
	sort.Strings(keys)
	return keys
}

// Exactly what a viewer may call. Changing this list is a permission change:
// it must be deliberate. (Snapshot of casbin/policy.csv when the table replaced
// it, minus its rule for a PUT /watch-history route that never existed.)
func TestAPIRoutes_ViewerPermissions(t *testing.T) {
	want := []string{
		"DELETE /favorites/:videoId",
		"DELETE /watch-history",
		"GET /favorites",
		"GET /me",
		"GET /recommendations/today",
		"GET /tags",
		"GET /videos",
		"GET /videos/:id",
		"GET /videos/:id/stream",
		"GET /videos/:id/stream-token",
		"GET /watch-history",
		"GET /ws",
		"POST /favorites",
		"POST /playback/telemetry",
		"POST /watch-history",
		"POST /watch-sessions/heartbeat",
	}
	got := routeKeys(func(viewer, _ bool) bool { return viewer })
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("viewer routes changed:\n got: %v\nwant: %v", got, want)
	}
}

// Exactly where a Stream Token works; each must bind :id, because the guard
// matches the token's Video against it.
func TestAPIRoutes_StreamTokenRoutes(t *testing.T) {
	want := []string{
		"GET /videos/:id/hls/:segment",
		"GET /videos/:id/hls/index.m3u8",
		"GET /videos/:id/stream",
	}
	got := routeKeys(func(_, stream bool) bool { return stream })
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("stream-token routes changed:\n got: %v\nwant: %v", got, want)
	}
	for _, k := range got {
		if !strings.Contains(k, "/:id/") && !strings.HasSuffix(k, "/:id") {
			t.Errorf("stream-token route %q does not bind :id", k)
		}
	}
}

func TestAPIRoutes_Unique(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range apiRoutes(apiHandlers{}) {
		key := r.Method + " " + r.Path
		if seen[key] {
			t.Errorf("route %s registered twice", key)
		}
		seen[key] = true
		switch r.Method {
		case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
		default:
			t.Errorf("route %s: unexpected method", key)
		}
	}
}
