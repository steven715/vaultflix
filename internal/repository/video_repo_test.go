package repository

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/steven/vaultflix/internal/model"
)

// seedFullVideo stores a Video with every column populated (after mutate) and
// returns its id. Rows are removed when the test ends, so the shared
// integration database is left as the shell suites expect it.
func seedFullVideo(t *testing.T, repo VideoRepository, sources MediaSourceRepository, mutate func(*model.Video)) string {
	t.Helper()
	ctx := context.Background()
	tag := uuid.NewString()[:8]

	source := &model.MediaSource{Label: "rowtest " + tag, MountPath: "/mnt/host/rowtest-" + tag}
	if err := sources.Create(ctx, source); err != nil {
		t.Fatalf("create media source: %v", err)
	}
	t.Cleanup(func() {
		if err := sources.Delete(context.Background(), source.ID); err != nil {
			t.Errorf("cleanup media source %s: %v", source.ID, err)
		}
	})

	filePath := "dir/" + tag + ".mkv"
	v := &model.Video{
		ID: uuid.NewString(), Title: "vfrowtest" + tag, Description: "desc",
		MinIOObjectKey: "legacy/" + tag, ThumbnailKey: "thumbnails/" + tag + ".jpg", PreviewKey: "previews/" + tag + ".mp4",
		DurationSeconds: 120, Resolution: "1920x1080", FileSizeBytes: 1234, MimeType: "video/x-matroska",
		VideoCodec: "h264", AudioCodec: "aac", OriginalFilename: tag + ".mkv",
		SourceID: &source.ID, FilePath: &filePath, Code: "ABC-123", EnrichmentStatus: "pending",
	}
	if mutate != nil {
		mutate(v)
	}
	if err := repo.Create(ctx, v); err != nil {
		t.Fatalf("create video: %v", err)
	}
	t.Cleanup(func() {
		if err := repo.Delete(context.Background(), v.ID); err != nil {
			t.Errorf("cleanup video %s: %v", v.ID, err)
		}
	})

	release := time.Date(2024, 5, 17, 0, 0, 0, 0, time.UTC)
	if err := repo.UpdateMetadata(ctx, v.ID, model.VideoMetadataUpdate{
		Code: v.Code, Title: v.Title, ReleaseDate: &release, RuntimeMinutes: 118,
		Maker: "Maker", Label: "Label", Series: "Series", CoverKey: "covers/ABC-123-javbus.jpg",
	}); err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	return v.ID
}

func normalizeTimes(v *model.Video) {
	v.CreatedAt, v.UpdatedAt = v.CreatedAt.UTC(), v.UpdatedAt.UTC()
	for _, p := range []**time.Time{&v.ReleaseDate, &v.EnrichedAt} {
		if *p != nil {
			u := (*p).UTC()
			*p = &u
		}
	}
}

// Every model.Video field must come back from the database: a column added to
// the struct but missing from the shared column list fails here. The fixture
// therefore gives every field a non-zero value; a future field whose only
// valid value can be zero needs an explicit exemption below.
func TestVideoRepository_GetByID_ReadsEveryColumn(t *testing.T) {
	pool := openTestPool(t)
	repo, sources := NewVideoRepository(pool), NewMediaSourceRepository(pool)
	id := seedFullVideo(t, repo, sources, nil)

	got, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	rv := reflect.ValueOf(*got)
	for i := 0; i < rv.NumField(); i++ {
		if rv.Field(i).IsZero() {
			t.Errorf("model.Video.%s came back zero", rv.Type().Field(i).Name)
		}
	}
	if got.Maker != "Maker" || got.EnrichmentStatus != "enriched" || got.RuntimeMinutes != 118 {
		t.Errorf("enriched Metadata not read back: %+v", got)
	}
	if got.ReleaseDate == nil || !got.ReleaseDate.Equal(time.Date(2024, 5, 17, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("release_date = %v, want 2024-05-17", got.ReleaseDate)
	}
}

// Every reader of Videos returns the same complete row GetByID does.
func TestVideoReaders_ReturnCompleteRows(t *testing.T) {
	ctx := context.Background()
	pool := openTestPool(t)
	repo, sources := NewVideoRepository(pool), NewMediaSourceRepository(pool)
	recs, tags := NewRecommendationRepository(pool), NewTagRepository(pool)
	tagged := func(t *testing.T, videoID string) int {
		t.Helper()
		tag := &model.Tag{Name: "rowtest-" + uuid.NewString()[:8], Category: "custom"}
		if err := tags.Create(ctx, tag); err != nil {
			t.Fatalf("create tag: %v", err)
		}
		t.Cleanup(func() {
			if _, err := pool.Exec(context.Background(), "DELETE FROM tags WHERE id = $1", tag.ID); err != nil {
				t.Errorf("cleanup tag %d: %v", tag.ID, err)
			}
		})
		if err := tags.AddVideoTag(ctx, videoID, tag.ID); err != nil {
			t.Fatalf("tag video: %v", err)
		}
		return tag.ID
	}
	findIn := func(videos []model.Video, id string) *model.Video {
		for i := range videos {
			if videos[i].ID == id {
				return &videos[i]
			}
		}
		return nil
	}
	tests := []struct {
		name   string
		mutate func(*model.Video)
		read   func(t *testing.T, repo VideoRepository, want *model.Video) (*model.Video, error)
	}{
		{"FindBySourceAndPath", nil, func(t *testing.T, r VideoRepository, w *model.Video) (*model.Video, error) {
			return r.FindBySourceAndPath(ctx, *w.SourceID, *w.FilePath)
		}},
		{"List", nil, func(t *testing.T, r VideoRepository, w *model.Video) (*model.Video, error) {
			vs, _, err := r.List(ctx, model.VideoFilter{Page: 1, PageSize: 50, Query: w.Title})
			return findIn(vs, w.ID), err
		}},
		{"List by tag", nil, func(t *testing.T, r VideoRepository, w *model.Video) (*model.Video, error) {
			vs, _, err := r.List(ctx, model.VideoFilter{Page: 1, PageSize: 50, TagIDs: []int{tagged(t, w.ID)}})
			return findIn(vs, w.ID), err
		}},
		{"List random", nil, func(t *testing.T, r VideoRepository, w *model.Video) (*model.Video, error) {
			vs, _, err := r.List(ctx, model.VideoFilter{Page: 1, PageSize: 50, Query: w.Title, SortBy: "random"})
			return findIn(vs, w.ID), err
		}},
		{"ListMissingPreviews", func(v *model.Video) { v.PreviewKey = "" }, func(t *testing.T, r VideoRepository, w *model.Video) (*model.Video, error) {
			vs, err := r.ListMissingPreviews(ctx)
			return findIn(vs, w.ID), err
		}},
		{"ListByEnrichmentStatus", nil, func(t *testing.T, r VideoRepository, w *model.Video) (*model.Video, error) {
			vs, err := r.ListByEnrichmentStatus(ctx, "enriched")
			return findIn(vs, w.ID), err
		}},
		{"ListMissingCodecs", func(v *model.Video) { v.VideoCodec, v.AudioCodec = "", "" }, func(t *testing.T, r VideoRepository, w *model.Video) (*model.Video, error) {
			vs, err := r.ListMissingCodecs(ctx, 100000)
			return findIn(vs, w.ID), err
		}},
		{"RecommendationRepository.GetRandomUnwatched", nil, func(t *testing.T, _ VideoRepository, w *model.Video) (*model.Video, error) {
			vs, err := recs.GetRandomUnwatched(ctx, uuid.NewString(), 100000)
			return findIn(vs, w.ID), err
		}},
		{"ListKeyframeCandidates", nil, func(t *testing.T, r VideoRepository, w *model.Video) (*model.Video, error) {
			vs, err := r.ListKeyframeCandidates(ctx, 100000)
			return findIn(vs, w.ID), err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := seedFullVideo(t, repo, sources, tt.mutate)
			want, err := repo.GetByID(ctx, id)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			got, err := tt.read(t, repo, want)
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if got == nil {
				t.Fatalf("%s did not return the seeded video", tt.name)
			}
			normalizeTimes(want)
			normalizeTimes(got)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s returned a partial row\n got: %+v\nwant: %+v", tt.name, *got, *want)
			}
		})
	}
}
