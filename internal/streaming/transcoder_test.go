package streaming

import (
	"strings"
	"testing"
)

func TestBuildRemuxHLSArgs_CopiesCodecsAndEventPlaylist(t *testing.T) {
	args := buildRemuxHLSArgs("/mnt/host/D/movie.mkv", "/cache/sess1")
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"-i /mnt/host/D/movie.mkv",
		"-c copy",
		"-f hls",
		"-hls_playlist_type event",
		"/cache/sess1/index.m3u8",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q; got: %s", want, joined)
		}
	}
}
