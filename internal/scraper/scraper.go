package scraper

import (
	"context"

	"github.com/steven/vaultflix/internal/model"
)

// MetadataScraper 對單一來源站依番號抓 metadata。
// ScrapeByCode 找不到番號頁回 model.ErrCodeNotFound；
// 被 Cloudflare / JS challenge 擋下回 model.ErrScrapeBlocked；
// 連線/解析失敗回 model.ErrSourceUnavailable。
type MetadataScraper interface {
	// Source 回傳來源站識別字串（如 "javbus"）。
	Source() string

	// ScrapeByCode 依番號抓取並回傳 EnrichedMetadata。
	// 找不到番號：回 model.ErrCodeNotFound。
	// 被封鎖：回 model.ErrScrapeBlocked。
	// 網路/解析失敗：回 model.ErrSourceUnavailable。
	ScrapeByCode(ctx context.Context, code string) (*model.EnrichedMetadata, error)
}
