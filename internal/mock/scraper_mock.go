package mock

import (
	"context"
	"fmt"

	"github.com/steven/vaultflix/internal/model"
)

// Scraper is a hand-written mock for scraper.MetadataScraper.
// Set ScrapeByCodeFunc to override the default error behaviour.
// SourceValue is returned verbatim by Source().
type Scraper struct {
	SourceValue      string
	ScrapeByCodeFunc func(ctx context.Context, code string) (*model.EnrichedMetadata, error)
}

// Source returns the configured SourceValue.
func (m *Scraper) Source() string {
	return m.SourceValue
}

// ScrapeByCode delegates to ScrapeByCodeFunc if set; otherwise returns an error.
func (m *Scraper) ScrapeByCode(ctx context.Context, code string) (*model.EnrichedMetadata, error) {
	if m.ScrapeByCodeFunc == nil {
		return nil, fmt.Errorf("mock: ScrapeByCodeFunc not set")
	}
	return m.ScrapeByCodeFunc(ctx, code)
}
