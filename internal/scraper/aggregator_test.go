package scraper

import (
	"testing"

	"github.com/steven/vaultflix/internal/model"
)

func TestMergeByPriority_FirstNonEmptyWins(t *testing.T) {
	results := []SourceResult{
		{Source: "javbus", Data: &model.EnrichedMetadata{Code: "DASD-626", Title: "", Maker: "Das!", Genres: nil}},
		{Source: "javlibrary", Data: &model.EnrichedMetadata{Code: "DASD-626", Title: "後輩", Maker: "X", Genres: []string{"巨乳"}}},
	}
	got := MergeByPriority(results)
	if got.Title != "後輩" {
		t.Errorf("title = %q, want 後輩 (fallback to javlibrary)", got.Title)
	}
	if got.Maker != "Das!" {
		t.Errorf("maker = %q, want Das! (javbus wins)", got.Maker)
	}
	if len(got.Genres) != 1 || got.Genres[0] != "巨乳" {
		t.Errorf("genres = %v, want [巨乳]", got.Genres)
	}
}

func TestMergeByPriority_Empty(t *testing.T) {
	if got := MergeByPriority(nil); got != nil {
		t.Errorf("want nil for empty input, got %v", got)
	}
}
