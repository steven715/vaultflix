package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/model"
)

// applyFixture seeds a complete Video plus one pending Suggestion for it, and
// removes the Tags and Performers the test names when it ends.
type applyFixture struct {
	pool        *pgxpool.Pool
	videos      VideoRepository
	suggestions SuggestionRepository
	tags        TagRepository
	performers  ActressRepository
	videoID     string
	suggestion  string
	tag         string // unique prefix for names this test creates
}

func newApplyFixture(t *testing.T) *applyFixture {
	t.Helper()
	pool := openTestPool(t)
	f := &applyFixture{
		pool: pool, videos: NewVideoRepository(pool), suggestions: NewSuggestionRepository(pool),
		tags: NewTagRepository(pool), performers: NewActressRepository(pool), tag: "applytest-" + uuid.NewString()[:8],
	}
	f.videoID = seedFullVideo(t, f.videos, NewMediaSourceRepository(pool), nil)
	sug := &model.MetadataSuggestion{VideoID: f.videoID, Source: "javbus", Code: "ABC-123", Status: "pending"}
	if err := f.suggestions.Create(context.Background(), sug); err != nil {
		t.Fatalf("create suggestion: %v", err)
	}
	f.suggestion = sug.ID
	t.Cleanup(func() {
		for _, q := range []string{
			"DELETE FROM tags WHERE name LIKE $1 || '%'",
			"DELETE FROM actresses WHERE name_ja LIKE $1 || '%'",
		} {
			if _, err := pool.Exec(context.Background(), q, f.tag); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})
	return f
}

func (f *applyFixture) application(genres []string, performerNames ...string) model.SuggestionApplication {
	app := model.SuggestionApplication{
		SuggestionID: f.suggestion, VideoID: f.videoID, Genres: genres,
		Metadata: model.VideoMetadataUpdate{Code: "ABC-123", Title: "Applied", Maker: "NewMaker"},
	}
	for _, n := range performerNames {
		app.Performers = append(app.Performers, model.Actress{NameJa: n})
	}
	return app
}

func (f *applyFixture) tagNames(t *testing.T) []string {
	t.Helper()
	tags, err := f.tags.GetByVideoID(context.Background(), f.videoID)
	if err != nil {
		t.Fatalf("tags of video: %v", err)
	}
	var names []string
	for _, tg := range tags {
		names = append(names, tg.Name)
	}
	return names
}

// Accepting a Suggestion whose genre the Video already has must still succeed
// (it used to fail with a duplicate video_tags row after writing half of it).
func TestSuggestionRepository_Apply_GenreAlreadyOnVideo(t *testing.T) {
	f := newApplyFixture(t)
	ctx := context.Background()
	existing, err := f.tags.GetOrCreateByName(ctx, f.tag+"-existing", "genre")
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := f.tags.AddVideoTag(ctx, f.videoID, existing.ID); err != nil {
		t.Fatalf("tag video: %v", err)
	}

	err = f.suggestions.Apply(ctx, f.application([]string{f.tag + "-existing", f.tag + "-new"}, f.tag+"-performer"))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	v, err := f.videos.GetByID(ctx, f.videoID)
	if err != nil || v.Maker != "NewMaker" || v.Title != "Applied" {
		t.Errorf("Metadata not applied: %+v, %v", v, err)
	}
	if got := strings.Join(f.tagNames(t), ","); !strings.Contains(got, f.tag+"-existing") || !strings.Contains(got, f.tag+"-new") {
		t.Errorf("tags = %s, want both genres", got)
	}
	performers, err := f.performers.GetByVideoID(ctx, f.videoID)
	if err != nil || len(performers) != 1 || performers[0].NameJa != f.tag+"-performer" {
		t.Errorf("performers = %+v, %v", performers, err)
	}
	if _, err := f.suggestions.GetByID(ctx, f.suggestion); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("Suggestion still there after Apply: %v", err)
	}
}

// A failure at the very last write leaves nothing applied and the Suggestion
// pending: Metadata, the Performer and the first genre are all rolled back.
func TestSuggestionRepository_Apply_RollsBackOnFailure(t *testing.T) {
	f := newApplyFixture(t)
	ctx := context.Background()
	tooLong := f.tag + strings.Repeat("長", 120) // tags.name is VARCHAR(100)

	if err := f.suggestions.Apply(ctx, f.application([]string{f.tag + "-genre", tooLong}, f.tag+"-performer")); err == nil {
		t.Fatal("Apply succeeded with an over-long genre name")
	}

	if v, _ := f.videos.GetByID(ctx, f.videoID); v.Maker != "Maker" {
		t.Errorf("Maker = %q, want the pre-Apply value (rolled back)", v.Maker)
	}
	if names := f.tagNames(t); len(names) != 0 {
		t.Errorf("tags linked despite rollback: %v", names)
	}
	if performers, _ := f.performers.GetByVideoID(ctx, f.videoID); len(performers) != 0 {
		t.Errorf("performers linked despite rollback: %+v", performers)
	}
	if _, err := f.suggestions.GetByID(ctx, f.suggestion); err != nil {
		t.Errorf("Suggestion gone despite rollback: %v", err)
	}
}

func TestSuggestionRepository_Apply_SuggestionMissing(t *testing.T) {
	f := newApplyFixture(t)
	app := f.application(nil)
	app.SuggestionID = uuid.NewString()

	if err := f.suggestions.Apply(context.Background(), app); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if v, _ := f.videos.GetByID(context.Background(), f.videoID); v.Maker != "Maker" {
		t.Errorf("Metadata applied for a missing Suggestion: %+v", v)
	}
}

// A Suggestion without a staged image must not erase the image a Video or
// Performer already has; a staged one replaces it.
func TestSuggestionRepository_Apply_ImageKeys(t *testing.T) {
	tests := []struct {
		name                  string
		coverKey, avatarKey   string
		wantCover, wantAvatar string
	}{
		{"nothing staged keeps existing images", "", "", "covers/ABC-123-javbus.jpg", "actresses/old.jpg"},
		{"staged images replace them", "covers/new.jpg", "actresses/new.jpg", "covers/new.jpg", "actresses/new.jpg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newApplyFixture(t)
			ctx := context.Background()
			name := f.tag + "-performer"
			if err := f.performers.Upsert(ctx, &model.Actress{NameJa: name, AvatarKey: "actresses/old.jpg"}); err != nil {
				t.Fatalf("seed performer: %v", err)
			}
			app := f.application(nil)
			app.Metadata.CoverKey = tt.coverKey
			app.Performers = []model.Actress{{NameJa: name, AvatarKey: tt.avatarKey}}

			if err := f.suggestions.Apply(ctx, app); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if v, _ := f.videos.GetByID(ctx, f.videoID); v.CoverKey != tt.wantCover {
				t.Errorf("cover_key = %q, want %q", v.CoverKey, tt.wantCover)
			}
			if ps, _ := f.performers.GetByVideoID(ctx, f.videoID); len(ps) != 1 || ps[0].AvatarKey != tt.wantAvatar {
				t.Errorf("performers = %+v, want avatar %q", ps, tt.wantAvatar)
			}
		})
	}
}
