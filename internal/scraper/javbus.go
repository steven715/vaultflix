package scraper

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/steven/vaultflix/internal/model"
)

// JavBusScraper 實作 MetadataScraper，抓取 JavBus 影片頁面。
type JavBusScraper struct {
	client  *Client
	baseURL string
}

// NewJavBusScraper 建立 JavBusScraper。baseURL 預設為 "https://www.javbus.com"。
func NewJavBusScraper(c *Client, baseURL string) *JavBusScraper {
	if baseURL == "" {
		baseURL = "https://www.javbus.com"
	}
	return &JavBusScraper{client: c, baseURL: baseURL}
}

// Source 回傳來源站識別字串。
func (s *JavBusScraper) Source() string { return "javbus" }

// ScrapeByCode 依番號組 URL 抓取頁面並解析 metadata。
// 找不到番號回 model.ErrCodeNotFound；被擋回 model.ErrScrapeBlocked；
// 連線/解析失敗回 model.ErrSourceUnavailable。
func (s *JavBusScraper) ScrapeByCode(ctx context.Context, code string) (*model.EnrichedMetadata, error) {
	url := fmt.Sprintf("%s/%s", strings.TrimRight(s.baseURL, "/"), code)
	body, err := s.client.Get(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("javbus get %s: %w", code, err)
	}
	return parseJavBus(body, code)
}

// parseJavBus 解析 JavBus 影片頁 HTML，回傳 EnrichedMetadata。
// html 為頁面內容；code 為番號。
// 偵測到年齡驗證頁時回 model.ErrScrapeBlocked；
// 解析出的 title 為空時，表示頁面不存在，回 model.ErrCodeNotFound。
func parseJavBus(html []byte, code string) (*model.EnrichedMetadata, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("parse javbus html: %w", model.ErrSourceUnavailable)
	}

	// 年齡驗證頁偵測：title 含 "Age Verification" 或 body 含 driver-verify marker
	pageTitle := strings.TrimSpace(doc.Find("title").First().Text())
	if strings.Contains(pageTitle, "Age Verification") || doc.Find("[name=driver-verify]").Length() > 0 {
		return nil, fmt.Errorf("javbus age-gate for %s: %w", code, model.ErrScrapeBlocked)
	}

	m := &model.EnrichedMetadata{Code: code}

	// 標題：頁面 <h3> 第一個
	m.Title = strings.TrimSpace(doc.Find("h3").First().Text())
	if m.Title == "" {
		return nil, fmt.Errorf("javbus empty title for %s: %w", code, model.ErrCodeNotFound)
	}

	// 基本資訊區塊（製作商、發行商、系列、片長）
	parseInfoBlock(doc, m)

	// 類別：只收 href 含 /genre/ 的連結，跳過 /star/ 等非類別連結
	doc.Find(".genre a").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		if !strings.Contains(href, "/genre/") {
			return
		}
		if g := strings.TrimSpace(a.Text()); g != "" {
			m.Genres = append(m.Genres, g)
		}
	})

	// 女優（名稱 + 頭像）
	parseActresses(doc, m)

	// 封面圖
	if src, ok := doc.Find(".bigImage img").Attr("src"); ok && src != "" {
		m.CoverURL = absoluteURL(src)
	}

	return m, nil
}

// parseInfoBlock 從 .movie .info p 區塊解析製作商、發行商、系列、片長與發行日期。
func parseInfoBlock(doc *goquery.Document, m *model.EnrichedMetadata) {
	doc.Find(".movie .info p").Each(func(_ int, p *goquery.Selection) {
		header := strings.TrimSpace(p.Find("span.header").First().Text())
		switch {
		case strings.Contains(header, "製作商") || strings.Contains(header, "Studio"):
			m.Maker = strings.TrimSpace(p.Find("a").First().Text())
			if m.Maker == "" {
				m.Maker = cleanLabel(p.Text(), header)
			}
		case strings.Contains(header, "發行商") || strings.Contains(header, "Label"):
			m.Label = strings.TrimSpace(p.Find("a").First().Text())
			if m.Label == "" {
				m.Label = cleanLabel(p.Text(), header)
			}
		case strings.Contains(header, "系列") || strings.Contains(header, "Series"):
			m.Series = strings.TrimSpace(p.Find("a").First().Text())
			if m.Series == "" {
				m.Series = cleanLabel(p.Text(), header)
			}
		case strings.Contains(header, "長度") || strings.Contains(header, "Length"):
			m.RuntimeMinutes = parseMinutes(p.Text())
		case strings.Contains(header, "發行日期") || strings.Contains(header, "Release Date"):
			if t := parseDate(cleanLabel(p.Text(), header)); t != nil {
				m.ReleaseDate = t
			}
		}
	})
}

// parseActresses 從 .star-name a（女優名）與父層 .star li 的 img（頭像）解析女優資訊。
func parseActresses(doc *goquery.Document, m *model.EnrichedMetadata) {
	doc.Find(".star-name a").Each(func(_ int, a *goquery.Selection) {
		name := strings.TrimSpace(a.Text())
		if name == "" {
			return
		}
		// 頭像：向上找最近的 li，再找 img
		avatar := ""
		li := a.Closest("li")
		if li.Length() > 0 {
			if src, ok := li.Find("img").First().Attr("src"); ok {
				avatar = absoluteURL(src)
			}
		}
		m.Actresses = append(m.Actresses, model.ActressMeta{
			NameJa:    name,
			AvatarURL: avatar,
		})
	})
}

// cleanLabel 移除段落文字前綴的 header 標題，回傳剩餘內容。
func cleanLabel(full, header string) string {
	trimmed := strings.TrimSpace(full)
	after := strings.TrimPrefix(trimmed, header)
	return strings.TrimSpace(after)
}

// parseMinutes 從含數字的字串中提取分鐘數（如 "120分鐘"）。
func parseMinutes(s string) int {
	var digits strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	n, _ := strconv.Atoi(digits.String())
	return n
}

// parseDate 嘗試解析 "YYYY-MM-DD" 格式日期；失敗回 nil。
func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

// absoluteURL 將 protocol-relative（//...）或相對路徑補全為 https:// 開頭的絕對 URL。
func absoluteURL(src string) string {
	if strings.HasPrefix(src, "//") {
		return "https:" + src
	}
	if strings.HasPrefix(src, "/") {
		return "https://www.javbus.com" + src
	}
	return src
}
