package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestMediaProcessor_ProbeMediaInfo(t *testing.T) {
	cases := []struct {
		name string
		file string
		raw  []byte
		want model.MediaInfo
	}{
		{
			name: "mkv h264+aac is remux, MIME from extension",
			file: "a.mkv", raw: mock.ProbeJSONFor(120.5, "h264", "aac"),
			want: model.MediaInfo{DurationSeconds: 120, Resolution: "1920x1080", MimeType: "video/x-matroska", VideoCodec: "h264", AudioCodec: "aac"},
		},
		{
			name: "mp4 h264+aac is direct play, video/mp4",
			file: "b.mp4", raw: mock.ProbeJSONFor(60, "h264", "aac"),
			want: model.MediaInfo{DurationSeconds: 60, Resolution: "1920x1080", MimeType: "video/mp4", VideoCodec: "h264", AudioCodec: "aac"},
		},
		{
			name: "upper-case extension still classified",
			file: "c.MOV", raw: mock.ProbeJSONFor(10, "h264", "mp3"),
			want: model.MediaInfo{DurationSeconds: 10, Resolution: "1920x1080", MimeType: "video/mp4", VideoCodec: "h264", AudioCodec: "mp3"},
		},
		{
			name: "avi mpeg4+mp3 is transcode, MIME from extension",
			file: "e.avi", raw: mock.ProbeJSONFor(30, "mpeg4", "mp3"),
			want: model.MediaInfo{DurationSeconds: 30, Resolution: "1920x1080", MimeType: "video/x-msvideo", VideoCodec: "mpeg4", AudioCodec: "mp3"},
		},
		{
			name: "no streams leaves codecs empty",
			file: "d.wmv", raw: []byte(`{"format":{"duration":"5"},"streams":[]}`),
			want: model.MediaInfo{DurationSeconds: 5, MimeType: "video/x-ms-wmv"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := &mock.MediaTool{ProbeOutput: map[string][]byte{tc.file: tc.raw}}
			got, err := NewMediaProcessor(tool, &mock.MinIOClient{}).ProbeMediaInfo(context.Background(), "/mnt/host/D/"+tc.file)
			if err != nil {
				t.Fatalf("ProbeMediaInfo: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestMediaProcessor_ProbeMediaInfo_Errors(t *testing.T) {
	tests := []struct {
		name string
		tool *mock.MediaTool
	}{
		{"ffprobe fails", &mock.MediaTool{ProbeErr: errors.New("exit 1")}},
		{"malformed output", &mock.MediaTool{ProbeOutput: map[string][]byte{"a.mkv": []byte("not json")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewMediaProcessor(tt.tool, &mock.MinIOClient{}).ProbeMediaInfo(context.Background(), "/x/a.mkv"); err == nil {
				t.Error("ProbeMediaInfo succeeded")
			}
		})
	}
}

func TestMediaProcessor_MakeThumbnailAndPreview_UploadUnderVideoKeys(t *testing.T) {
	uploaded := map[string]string{}
	minio := &mock.MinIOClient{
		UploadThumbnailFunc: func(_ context.Context, key, _ string) error { uploaded["thumbnail"] = key; return nil },
		UploadPreviewFunc:   func(_ context.Context, key, _ string) error { uploaded["preview"] = key; return nil },
	}
	tool := &mock.MediaTool{Dir: t.TempDir()}
	p := NewMediaProcessor(tool, minio)
	f := model.MediaFile{VideoID: "v1", Path: "/mnt/host/D/a.mkv", DurationSeconds: 120}

	thumb, err := p.MakeThumbnail(context.Background(), f)
	if err != nil {
		t.Fatalf("MakeThumbnail: %v", err)
	}
	preview, err := p.MakePreview(context.Background(), f)
	if err != nil {
		t.Fatalf("MakePreview: %v", err)
	}

	if thumb != "thumbnails/v1.jpg" || uploaded["thumbnail"] != thumb {
		t.Errorf("thumbnail key %q (uploaded %q), want thumbnails/v1.jpg", thumb, uploaded["thumbnail"])
	}
	if preview != "previews/v1.mp4" || uploaded["preview"] != preview {
		t.Errorf("preview key %q (uploaded %q), want previews/v1.mp4", preview, uploaded["preview"])
	}
	for _, tmp := range tool.Created() {
		if _, err := os.Stat(tmp); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("temp file %s left behind", tmp)
		}
	}
}

func TestMediaProcessor_Make_Errors(t *testing.T) {
	f := model.MediaFile{VideoID: "v1", Path: "/x/a.mkv", DurationSeconds: 120}
	failingUpload := &mock.MinIOClient{
		UploadThumbnailFunc: func(context.Context, string, string) error { return errors.New("minio down") },
		UploadPreviewFunc:   func(context.Context, string, string) error { return errors.New("minio down") },
	}
	tests := []struct {
		name  string
		tool  *mock.MediaTool
		minio *mock.MinIOClient
	}{
		{"ffmpeg fails", &mock.MediaTool{FrameErr: errors.New("boom"), PreviewErr: errors.New("boom")}, &mock.MinIOClient{}},
		{"upload fails", &mock.MediaTool{Dir: t.TempDir()}, failingUpload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewMediaProcessor(tt.tool, tt.minio)
			if _, err := p.MakeThumbnail(context.Background(), f); err == nil {
				t.Error("MakeThumbnail succeeded")
			}
			if _, err := p.MakePreview(context.Background(), f); err == nil {
				t.Error("MakePreview succeeded")
			}
		})
	}
}
