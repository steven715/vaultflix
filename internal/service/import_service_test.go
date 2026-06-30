package service

import (
	"context"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestGetJob_Found(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	job := &model.ImportJob{
		ID:     "job-123",
		Status: "running",
	}
	svc.activeJobs.Store(job.ID, job)

	got, err := svc.GetJob("job-123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.ID != "job-123" {
		t.Errorf("expected job ID job-123, got %s", got.ID)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	_, err := svc.GetJob("nonexistent")
	if err != model.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetActiveJob_Running(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	job := &model.ImportJob{ID: "job-active", Status: "running"}
	svc.activeJobs.Store(job.ID, job)

	completed := &model.ImportJob{ID: "job-done", Status: "completed"}
	svc.activeJobs.Store(completed.ID, completed)

	got := svc.GetActiveJob()
	if got == nil {
		t.Fatal("expected active job, got nil")
	}
	if got.ID != "job-active" {
		t.Errorf("expected job-active, got %s", got.ID)
	}
}

func TestGetActiveJob_None(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	completed := &model.ImportJob{ID: "job-done", Status: "completed"}
	svc.activeJobs.Store(completed.ID, completed)

	got := svc.GetActiveJob()
	if got != nil {
		t.Errorf("expected nil, got job %s", got.ID)
	}
}

func TestStartAsync_Conflict(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	// Manually lock to simulate a running job
	svc.LockForTest()

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Test Source",
		MountPath: t.TempDir(),
	}

	_, err := svc.StartAsync(nil, source, "user-1")
	if err != model.ErrConflict {
		t.Errorf("expected ErrConflict, got %v", err)
	}

	svc.UnlockForTest()
}

func TestStartAsync_EmptyDirectory(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Empty Source",
		MountPath: t.TempDir(),
	}

	job, err := svc.StartAsync(nil, source, "user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	// Wait for background goroutine to complete
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job to complete")
		default:
			got, _ := svc.GetJob(job.ID)
			if got.Status != "running" {
				if got.Status != "completed" {
					t.Errorf("expected completed, got %s", got.Status)
				}
				if got.Total != 0 {
					t.Errorf("expected total 0, got %d", got.Total)
				}
				if got.FinishedAt == nil {
					t.Error("expected FinishedAt to be set")
				}

				msgs := notifier.GetMessages()
				if len(msgs) == 0 {
					t.Fatal("expected at least 1 notifier message")
				}
				lastMsg := msgs[len(msgs)-1]
				if lastMsg.Type != "import_complete" {
					t.Errorf("expected import_complete, got %s", lastMsg.Type)
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestStartAsync_ScanError(t *testing.T) {
	notifier := &mock.Notifier{}
	svc := NewImportService(&mock.VideoRepository{}, &mock.MinIOClient{}, notifier)

	source := &model.MediaSource{
		ID:        "src-1",
		Label:     "Bad Source",
		MountPath: "/nonexistent/path/that/does/not/exist",
	}

	job, err := svc.StartAsync(nil, source, "user-1")
	if err != nil {
		t.Fatalf("StartAsync failed: %v", err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job to complete")
		default:
			got, _ := svc.GetJob(job.ID)
			if got.Status != "running" {
				if got.Status != "failed" {
					t.Errorf("expected failed, got %s", got.Status)
				}
				if len(got.Errors) == 0 {
					t.Error("expected at least 1 error")
				}

				msgs := notifier.GetMessages()
				hasImportError := false
				for _, msg := range msgs {
					if msg.Type == "import_error" {
						hasImportError = true
					}
				}
				if !hasImportError {
					t.Error("expected import_error message")
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

// TestSeedEnrichment verifies the seedEnrichment helper sets Code and
// EnrichmentStatus correctly for filenames with and without a JAV code.
func TestSeedEnrichment(t *testing.T) {
	cases := []struct {
		filename             string
		wantCode             string
		wantEnrichmentStatus string
	}{
		{
			filename:             "DASD-626.mp4",
			wantCode:             "DASD-626",
			wantEnrichmentStatus: model.EnrichmentPending,
		},
		{
			filename:             "家庭聚會.mp4",
			wantCode:             "",
			wantEnrichmentStatus: model.EnrichmentNoCode,
		},
		{
			filename:             "FC2-PPV-1234567.mkv",
			wantCode:             "FC2-PPV-1234567",
			wantEnrichmentStatus: model.EnrichmentPending,
		},
		{
			filename:             "random_home_video.mp4",
			wantCode:             "",
			wantEnrichmentStatus: model.EnrichmentNoCode,
		},
	}

	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			v := &model.Video{}
			seedEnrichment(v, tc.filename)
			if v.Code != tc.wantCode {
				t.Errorf("Code: got %q, want %q", v.Code, tc.wantCode)
			}
			if v.EnrichmentStatus != tc.wantEnrichmentStatus {
				t.Errorf("EnrichmentStatus: got %q, want %q", v.EnrichmentStatus, tc.wantEnrichmentStatus)
			}
		})
	}
}

func TestParseProbeOutput(t *testing.T) {
	cases := []struct {
		name           string
		raw            []byte
		ext            string
		wantVideoCodec string
		wantAudioCodec string
		wantMimeType   string
		wantDuration   int
		wantResolution string
		wantErr        bool
	}{
		{
			// Original flat test: codec/resolution/duration extraction for mkv.
			// .mkv h264+aac is PlayModeRemux (not direct), so mime falls back to
			// extensionToMIME(".mkv") = "video/x-matroska".
			name: "mkv h264+aac extracts codecs and resolution",
			raw: []byte(`{
				"format": {"duration": "120.5", "size": "1048576"},
				"streams": [
					{"codec_type": "video", "codec_name": "h264", "width": 1920, "height": 1080},
					{"codec_type": "audio", "codec_name": "aac"}
				]
			}`),
			ext:            ".mkv",
			wantVideoCodec: "h264",
			wantAudioCodec: "aac",
			wantMimeType:   "video/x-matroska",
			wantDuration:   120,
			wantResolution: "1920x1080",
		},
		{
			// Original flat test: .mp4 h264+aac is PlayModeDirect → mime = "video/mp4".
			name: "mp4 h264+aac direct play gets video/mp4 mime",
			raw: []byte(`{"format":{"duration":"10"},"streams":[
				{"codec_type":"video","codec_name":"h264","width":640,"height":480},
				{"codec_type":"audio","codec_name":"aac"}]}`),
			ext:            ".mp4",
			wantVideoCodec: "h264",
			wantAudioCodec: "aac",
			wantMimeType:   "video/mp4",
		},
		{
			// Remux case: .mkv h264+aac → ClassifyPlayMode returns PlayModeRemux,
			// so mimeTypeFor falls back to extensionToMIME(".mkv") = "video/x-matroska".
			name: "mkv remux h264+aac gets extensionToMIME fallback video/x-matroska",
			raw: []byte(`{"format":{"duration":"60"},"streams":[
				{"codec_type":"video","codec_name":"h264","width":1280,"height":720},
				{"codec_type":"audio","codec_name":"aac"}]}`),
			ext:            ".mkv",
			wantVideoCodec: "h264",
			wantAudioCodec: "aac",
			wantMimeType:   "video/x-matroska",
			wantDuration:   60,
			wantResolution: "1280x720",
		},
		{
			// Transcode case: .avi mpeg4+mp3 → ClassifyPlayMode returns PlayModeTranscode,
			// so mimeTypeFor falls back to extensionToMIME(".avi") = "video/x-msvideo".
			name: "avi mpeg4+mp3 gets extensionToMIME fallback video/x-msvideo",
			raw: []byte(`{"format":{"duration":"90"},"streams":[
				{"codec_type":"video","codec_name":"mpeg4","width":720,"height":480},
				{"codec_type":"audio","codec_name":"mp3"}]}`),
			ext:            ".avi",
			wantVideoCodec: "mpeg4",
			wantAudioCodec: "mp3",
			wantMimeType:   "video/x-msvideo",
			wantDuration:   90,
			wantResolution: "720x480",
		},
		{
			name:    "malformed JSON returns error",
			raw:     []byte(`{not valid json`),
			ext:     ".mp4",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md, err := parseProbeOutput(tc.raw, tc.ext)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if md.videoCodec != tc.wantVideoCodec {
				t.Errorf("videoCodec = %q, want %q", md.videoCodec, tc.wantVideoCodec)
			}
			if md.audioCodec != tc.wantAudioCodec {
				t.Errorf("audioCodec = %q, want %q", md.audioCodec, tc.wantAudioCodec)
			}
			if md.mimeType != tc.wantMimeType {
				t.Errorf("mimeType = %q, want %q", md.mimeType, tc.wantMimeType)
			}
			if tc.wantDuration != 0 && md.durationSeconds != tc.wantDuration {
				t.Errorf("durationSeconds = %d, want %d", md.durationSeconds, tc.wantDuration)
			}
			if tc.wantResolution != "" && md.resolution != tc.wantResolution {
				t.Errorf("resolution = %q, want %q", md.resolution, tc.wantResolution)
			}
		})
	}
}

// TestSeedEnrichment_CreateCapture verifies that a Video passed through
// seedEnrichment before Create carries the expected Code and EnrichmentStatus.
// This mirrors the exact call sequence in processOneFile.
func TestSeedEnrichment_CreateCapture(t *testing.T) {
	cases := []struct {
		name                 string
		filename             string
		wantCode             string
		wantEnrichmentStatus string
	}{
		{
			name:                 "JAV code extracted → pending",
			filename:             "DASD-626.mp4",
			wantCode:             "DASD-626",
			wantEnrichmentStatus: model.EnrichmentPending,
		},
		{
			name:                 "no JAV code → no_code",
			filename:             "家庭聚會.mp4",
			wantCode:             "",
			wantEnrichmentStatus: model.EnrichmentNoCode,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured model.Video
			videoRepo := &mock.VideoRepository{
				CreateFunc: func(_ context.Context, v *model.Video) error {
					captured = *v
					return nil
				},
			}

			v := &model.Video{OriginalFilename: tc.filename}
			seedEnrichment(v, tc.filename)
			if err := videoRepo.CreateFunc(context.Background(), v); err != nil {
				t.Fatalf("CreateFunc: %v", err)
			}

			if captured.Code != tc.wantCode {
				t.Errorf("Code: got %q, want %q", captured.Code, tc.wantCode)
			}
			if captured.EnrichmentStatus != tc.wantEnrichmentStatus {
				t.Errorf("EnrichmentStatus: got %q, want %q", captured.EnrichmentStatus, tc.wantEnrichmentStatus)
			}
		})
	}
}
