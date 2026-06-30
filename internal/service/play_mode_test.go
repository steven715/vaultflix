package service

import (
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func TestClassifyPlayMode(t *testing.T) {
	tests := []struct {
		name      string
		container string
		vcodec    string
		acodec    string
		want      model.PlayMode
	}{
		{"mp4 h264 aac is direct", "mp4", "h264", "aac", model.PlayModeDirect},
		{"mp4 h264 mp3 is direct", "mp4", "h264", "mp3", model.PlayModeDirect},
		{"mov h264 aac is direct", "mov", "h264", "aac", model.PlayModeDirect},
		{"avi h264 aac is remux", "avi", "h264", "aac", model.PlayModeRemux},
		{"avi h264 mp3 is remux", "avi", "h264", "mp3", model.PlayModeRemux},
		{"mkv h264 aac is remux", "mkv", "h264", "aac", model.PlayModeRemux},
		{"avi mpeg4 mp3 is transcode", "avi", "mpeg4", "mp3", model.PlayModeTranscode},
		{"mp4 mpeg4 aac is transcode", "mp4", "mpeg4", "aac", model.PlayModeTranscode},
		{"wmv wmv2 wmav2 is transcode", "wmv", "wmv2", "wmav2", model.PlayModeTranscode},
		{"mp4 hevc aac is transcode", "mp4", "hevc", "aac", model.PlayModeTranscode},
		{"mkv mpeg2video ac3 is transcode", "mkv", "mpeg2video", "ac3", model.PlayModeTranscode},
		{"uppercase normalized", "MP4", "H264", "AAC", model.PlayModeDirect},
		{"unknown video codec is transcode", "mp4", "", "aac", model.PlayModeTranscode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPlayMode(tt.container, tt.vcodec, tt.acodec)
			if got != tt.want {
				t.Errorf("ClassifyPlayMode(%q,%q,%q) = %q, want %q",
					tt.container, tt.vcodec, tt.acodec, got, tt.want)
			}
		})
	}
}
