package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

// importFixture is an Import run against a temp Media Source with a real
// MediaProcessor; only ffprobe/ffmpeg (mock.MediaTool) and the stores are fake.
type importFixture struct {
	svc       *ImportService
	notifier  *mock.Notifier
	tool      *mock.MediaTool
	keyframes *KeyframeService
	indexes   *fakeKeyframeRepo
	mu        sync.Mutex
	created   []model.Video
}

func newImportFixture(t *testing.T, files map[string][]byte, alreadyImported ...string) (*importFixture, *model.MediaSource) {
	t.Helper()
	mount := t.TempDir()
	for name := range files {
		if err := os.WriteFile(filepath.Join(mount, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := &importFixture{tool: &mock.MediaTool{Dir: t.TempDir(), ProbeOutput: files}, indexes: newFakeKeyframeRepo()}
	f.keyframes = NewKeyframeService(f.indexes)
	f.keyframes.probe = func(context.Context, string) ([]float64, float64, error) { return []float64{0, 8}, 16, nil }
	repo := &mock.VideoRepository{
		FindBySourceAndPathFunc: func(_ context.Context, _, path string) (*model.Video, error) {
			for _, dup := range alreadyImported {
				if dup == path {
					return &model.Video{ID: "existing"}, nil
				}
			}
			return nil, model.ErrNotFound
		},
		CreateFunc: func(_ context.Context, v *model.Video) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.created = append(f.created, *v)
			return nil
		},
	}
	minio := &mock.MinIOClient{
		UploadThumbnailFunc: func(context.Context, string, string) error { return nil },
		UploadPreviewFunc:   func(context.Context, string, string) error { return nil },
	}
	f.notifier = &mock.Notifier{}
	f.svc = NewImportService(ImportServiceDeps{
		Videos: repo, Media: NewMediaProcessor(f.tool, minio), Notifier: f.notifier, Lane: NewJobRunner(), Keyframes: f.keyframes,
	})
	return f, &model.MediaSource{ID: "src-1", Label: "D", MountPath: mount}
}

func (f *importFixture) run(t *testing.T, source *model.MediaSource) *model.ImportJob {
	t.Helper()
	job, err := f.svc.StartAsync(context.Background(), source, "user-1")
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got, _ := f.svc.GetJob(job.ID); got.Status != "running" {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("import never finished")
	return nil
}

func (f *importFixture) video(t *testing.T, filename string) model.Video {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.created {
		if v.OriginalFilename == filename {
			return v
		}
	}
	t.Fatalf("no Video created for %s", filename)
	return model.Video{}
}

func TestImportStartAsync_RegistersEveryNewFile(t *testing.T) {
	f, source := newImportFixture(t, map[string][]byte{
		"DASD-626.mkv": mock.ProbeJSONFor(120, "h264", "aac"),
		"home.mp4":     mock.ProbeJSONFor(60, "h264", "aac"),
	})

	job := f.run(t, source)

	if job.Status != "completed" || job.Imported != 2 || job.Failed != 0 {
		t.Fatalf("job = %+v, want completed with 2 imported", job)
	}
	mkv := f.video(t, "DASD-626.mkv")
	if mkv.Title != "DASD-626" || *mkv.FilePath != "DASD-626.mkv" || *mkv.SourceID != "src-1" {
		t.Errorf("identity fields wrong: %+v", mkv)
	}
	if mkv.DurationSeconds != 120 || mkv.VideoCodec != "h264" || mkv.MimeType != "video/x-matroska" {
		t.Errorf("Media Info not stored: %+v", mkv)
	}
	if mkv.ThumbnailKey != "thumbnails/"+mkv.ID+".jpg" || mkv.PreviewKey != "previews/"+mkv.ID+".mp4" {
		t.Errorf("asset keys = %q, %q", mkv.ThumbnailKey, mkv.PreviewKey)
	}
	if mkv.Code != "DASD-626" || mkv.EnrichmentStatus != model.EnrichmentPending {
		t.Errorf("Code seeding = %q/%q", mkv.Code, mkv.EnrichmentStatus)
	}
	if home := f.video(t, "home.mp4"); home.EnrichmentStatus != model.EnrichmentNoCode || home.MimeType != "video/mp4" {
		t.Errorf("home.mp4 = %+v", home)
	}
	if _, ok := f.indexes.stored[mkv.ID]; !ok || len(f.indexes.stored) != 1 {
		t.Errorf("Keyframe Indexes = %v, want only the remux DASD-626.mkv's", f.indexes.stored)
	}
}

// A Video whose Keyframe Index can't be built is still imported: the first
// play probes it again.
func TestImportStartAsync_KeyframeIndexFailureStillImports(t *testing.T) {
	f, source := newImportFixture(t, map[string][]byte{"a.mkv": mock.ProbeJSONFor(10, "h264", "aac")})
	f.keyframes.probe = func(context.Context, string) ([]float64, float64, error) { return nil, 0, errors.New("ffprobe exit 1") }

	job := f.run(t, source)

	if job.Imported != 1 || job.Failed != 0 || len(f.indexes.stored) != 0 {
		t.Errorf("job = %+v, indexes %v, want imported without a Keyframe Index", job, f.indexes.stored)
	}
}

func TestImportStartAsync_SkipsAlreadyImportedFile(t *testing.T) {
	f, source := newImportFixture(t, map[string][]byte{"a.mkv": mock.ProbeJSONFor(10, "h264", "aac")}, "a.mkv")

	job := f.run(t, source)

	if job.Skipped != 1 || job.Imported != 0 || len(f.created) != 0 {
		t.Errorf("job = %+v, created %d, want one skipped and nothing created", job, len(f.created))
	}
}

func TestImportStartAsync_PerFileFailures(t *testing.T) {
	tests := []struct {
		name         string
		breakTool    func(*mock.MediaTool)
		wantImported int
		wantFailed   int
	}{
		{"probe fails", func(m *mock.MediaTool) { m.ProbeErr = errors.New("ffprobe exit 1") }, 0, 1},
		{"thumbnail fails", func(m *mock.MediaTool) { m.FrameErr = errors.New("ffmpeg exit 1") }, 0, 1},
		{"preview fails, still imported", func(m *mock.MediaTool) { m.PreviewErr = errors.New("ffmpeg exit 1") }, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, source := newImportFixture(t, map[string][]byte{"a.mkv": mock.ProbeJSONFor(10, "h264", "aac")})
			tt.breakTool(f.tool)

			job := f.run(t, source)

			if job.Imported != tt.wantImported || job.Failed != tt.wantFailed {
				t.Fatalf("imported=%d failed=%d, want %d/%d (errors %+v)", job.Imported, job.Failed, tt.wantImported, tt.wantFailed, job.Errors)
			}
			if tt.wantImported == 1 {
				if v := f.video(t, "a.mkv"); v.PreviewKey != "" || v.ThumbnailKey == "" {
					t.Errorf("degraded import: preview %q thumbnail %q, want no preview but a thumbnail", v.PreviewKey, v.ThumbnailKey)
				}
			}
		})
	}
}

// Each per-file result carries the job's counts so far: the admin UI shows
// them as-is instead of adding up messages it may have missed.
func TestImportStartAsync_ProgressCarriesJobCounts(t *testing.T) {
	f, source := newImportFixture(t, map[string][]byte{"a.mkv": mock.ProbeJSONFor(10, "h264", "aac"), "b.mkv": mock.ProbeJSONFor(10, "h264", "aac")}, "b.mkv")

	f.run(t, source)

	var last model.ImportProgress
	for _, m := range f.notifier.GetMessages() {
		if p, ok := m.Payload.(model.ImportProgress); ok && p.Status != "processing" {
			last = p
		}
	}
	if last.Current != 2 || last.Total != 2 || last.Imported != 1 || last.Skipped != 1 || last.Failed != 0 {
		t.Errorf("last progress = %+v, want 2/2 with imported 1, skipped 1, failed 0", last)
	}
}
