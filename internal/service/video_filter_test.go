package service

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func queryOf(params map[string]string) func(string) string {
	return func(key string) string { return params[key] }
}

func TestParseVideoFilter_Defaults(t *testing.T) {
	got, err := ParseVideoFilter(queryOf(nil))
	if err != nil {
		t.Fatalf("ParseVideoFilter: %v", err)
	}
	want := model.VideoFilter{Page: 1, PageSize: 20, SortBy: model.SortByCreatedAt, SortOrder: "desc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filter = %+v, want %+v", got, want)
	}
}

func TestParseVideoFilter_ReadsEveryParameter(t *testing.T) {
	got, err := ParseVideoFilter(queryOf(map[string]string{
		"page": "3", "page_size": "100", "sort_by": "title", "sort_order": "ASC", "q": "abc", "tag_ids": "1, 2",
	}))
	if err != nil {
		t.Fatalf("ParseVideoFilter: %v", err)
	}
	want := model.VideoFilter{Page: 3, PageSize: 100, SortBy: model.SortByTitle, SortOrder: "asc", Query: "abc", TagIDs: []int{1, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("filter = %+v, want %+v", got, want)
	}
}

// random is the player's up-next column: a shuffled sample.
func TestParseVideoFilter_AcceptsRandom(t *testing.T) {
	got, err := ParseVideoFilter(queryOf(map[string]string{"sort_by": "random"}))
	if err != nil || got.SortBy != model.SortRandom {
		t.Errorf("filter = %+v, err = %v, want sort_by random", got, err)
	}
}

func TestParseVideoFilter_InvalidParameters(t *testing.T) {
	tests := []struct {
		name  string
		param string
		value string
	}{
		{"page not a number", "page", "x"},
		{"page zero", "page", "0"},
		{"page_size zero", "page_size", "0"},
		{"page_size over 100", "page_size", "101"},
		{"unknown sort_by", "sort_by", "xxx"},
		{"unknown sort_order", "sort_order", "up"},
		{"tag_ids not numbers", "tag_ids", "1,a"},
		{"tag_ids zero", "tag_ids", "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseVideoFilter(queryOf(map[string]string{tt.param: tt.value}))
			if !errors.Is(err, model.ErrInvalidInput) || !strings.HasPrefix(err.Error(), tt.param+" must") {
				t.Errorf("%s=%q: err = %v, want an invalid-input error naming %s", tt.param, tt.value, err, tt.param)
			}
		})
	}
}
