package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestAddVideoTag_Outcomes(t *testing.T) {
	boom := errors.New("db down")
	tests := []struct {
		name        string
		videoErr    error
		tagErr      error
		addErr      error
		wantCreated bool
		wantMissing string // which resource a NotFoundError names
		wantErr     error
	}{
		{name: "adds the tag", wantCreated: true},
		{name: "already tagged is a success, not created", addErr: model.ErrConflict},
		{name: "video missing", videoErr: model.ErrNotFound, wantMissing: "video", wantErr: model.ErrNotFound},
		{name: "tag missing", tagErr: model.ErrNotFound, wantMissing: "tag", wantErr: model.ErrNotFound},
		{name: "video lookup fails", videoErr: boom, wantErr: boom},
		{name: "repository failure", addErr: boom, wantErr: boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added := false
			videos := &mock.VideoRepository{GetByIDFunc: func(context.Context, string) (*model.Video, error) {
				return &model.Video{ID: "v1"}, tt.videoErr
			}}
			tags := &mock.TagRepository{
				GetByIDFunc: func(context.Context, int) (*model.Tag, error) { return &model.Tag{ID: 3}, tt.tagErr },
				AddVideoTagFunc: func(context.Context, string, int) error {
					added = true
					return tt.addErr
				},
			}
			created, err := NewTagService(tags, videos).AddVideoTag(context.Background(), "v1", 3)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if created != tt.wantCreated {
				t.Errorf("created = %v, want %v", created, tt.wantCreated)
			}
			var missing *model.NotFoundError
			gotMissing := errors.As(err, &missing)
			if tt.wantMissing != "" && (!gotMissing || missing.What != tt.wantMissing) {
				t.Errorf("err = %v, want a NotFoundError naming %q", err, tt.wantMissing)
			}
			if tt.wantMissing == "" && gotMissing {
				t.Errorf("err = %v, a failed lookup must not read as not found", err)
			}
			if tt.wantMissing != "" && added {
				t.Error("tagged a missing video or with a missing tag")
			}
		})
	}
}

func TestRemoveVideoTag_NotFound(t *testing.T) {
	tags := &mock.TagRepository{RemoveVideoTagFunc: func(context.Context, string, int) error { return model.ErrNotFound }}
	err := NewTagService(tags, &mock.VideoRepository{}).RemoveVideoTag(context.Background(), "v1", 3)
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateTag_Category(t *testing.T) {
	tests := []struct {
		name     string
		category string
		want     string // category stored; "" when refused
	}{
		{"defaults to custom", "", "custom"},
		{"keeps an allowed category", "genre", "genre"},
		{"refuses an unknown category", "mood", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stored *model.Tag
			tags := &mock.TagRepository{CreateFunc: func(_ context.Context, tag *model.Tag) error {
				stored = tag
				return nil
			}}
			err := NewTagService(tags, &mock.VideoRepository{}).Create(context.Background(), &model.Tag{Name: "x", Category: tt.category})

			if tt.want == "" {
				if !errors.Is(err, model.ErrInvalidInput) || stored != nil {
					t.Errorf("err = %v, stored = %v; want ErrInvalidInput and nothing stored", err, stored)
				}
				return
			}
			if err != nil || stored.Category != tt.want {
				t.Errorf("err = %v, stored = %+v; want category %q", err, stored, tt.want)
			}
		})
	}
}

func TestListTags_UnknownCategory(t *testing.T) {
	_, err := NewTagService(&mock.TagRepository{}, &mock.VideoRepository{}).List(context.Background(), "mood")
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}
