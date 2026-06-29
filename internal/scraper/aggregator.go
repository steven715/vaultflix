package scraper

import "github.com/steven/vaultflix/internal/model"

// SourceResult 封裝單一來源的 scrape 結果，用於優先序合併。
type SourceResult struct {
	Source string
	Data   *model.EnrichedMetadata
}

// MergeByPriority 依輸入順序（index 0 最高）逐欄位取第一個非空值。input 空回 nil。
func MergeByPriority(results []SourceResult) *model.EnrichedMetadata {
	if len(results) == 0 {
		return nil
	}
	out := &model.EnrichedMetadata{}
	for _, r := range results {
		d := r.Data
		if d == nil {
			continue
		}
		if out.Code == "" {
			out.Code = d.Code
		}
		if out.Title == "" {
			out.Title = d.Title
		}
		if out.ReleaseDate == nil {
			out.ReleaseDate = d.ReleaseDate
		}
		if out.RuntimeMinutes == 0 {
			out.RuntimeMinutes = d.RuntimeMinutes
		}
		if out.Maker == "" {
			out.Maker = d.Maker
		}
		if out.Label == "" {
			out.Label = d.Label
		}
		if out.Series == "" {
			out.Series = d.Series
		}
		if out.CoverURL == "" {
			out.CoverURL = d.CoverURL
		}
		if len(out.Genres) == 0 {
			out.Genres = d.Genres
		}
		if len(out.Actresses) == 0 {
			out.Actresses = d.Actresses
		}
	}
	return out
}
