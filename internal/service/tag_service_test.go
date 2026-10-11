package service

import (
	"context"
	"errors"
	"testing"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func TestTagServiceAddVideoTag(t *testing.T) {
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

			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if created != tt.wantCreated {
				t.Errorf("created = %v, want %v", created, tt.wantCreated)
			}
			var missing *model.NotFoundError
			if tt.wantMissing != "" && (!errors.As(err, &missing) || missing.What != tt.wantMissing) {
				t.Errorf("err = %v, want a NotFoundError naming %q", err, tt.wantMissing)
			}
			if tt.wantMissing != "" && added {
				t.Error("tagged a missing video or with a missing tag")
			}
		})
	}
}

func TestTagServiceRemoveVideoTag_NotFound(t *testing.T) {
	tags := &mock.TagRepository{RemoveVideoTagFunc: func(context.Context, string, int) error { return model.ErrNotFound }}
	err := NewTagService(tags, &mock.VideoRepository{}).RemoveVideoTag(context.Background(), "v1", 3)
	if !errors.Is(err, model.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
