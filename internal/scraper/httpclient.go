package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/steven/vaultflix/internal/model"
)

type ClientOptions struct {
	Timeout     time.Duration
	UserAgent   string
	MinInterval time.Duration
	MaxRetries  int
	Cookies     map[string]string
}

type Client struct {
	hc      *http.Client
	opts    ClientOptions
	lastReq time.Time
}

func NewClient(opts ClientOptions) *Client {
	if opts.Timeout == 0 {
		opts.Timeout = 15 * time.Second
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "Vaultflix/1.0"
	}
	return &Client{hc: &http.Client{Timeout: opts.Timeout}, opts: opts}
}

// Get 抓取 url，含速率間隔與退避重試。429/503 重試；耗盡回 ErrSourceUnavailable。
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastStatus int
	for attempt := 0; attempt <= c.opts.MaxRetries; attempt++ {
		if c.opts.MinInterval > 0 {
			if wait := time.Until(c.lastReq.Add(c.opts.MinInterval)); wait > 0 {
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		}
		c.lastReq = time.Now()

		body, status, err := c.do(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("http get %s: %w", url, err)
		}
		lastStatus = status
		switch {
		case status == http.StatusOK:
			return body, nil
		case status == http.StatusForbidden:
			return nil, fmt.Errorf("status 403 for %s: %w", url, model.ErrScrapeBlocked)
		case status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable:
			backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		default:
			return nil, fmt.Errorf("status %d for %s: %w", status, url, model.ErrSourceUnavailable)
		}
	}
	return nil, fmt.Errorf("exhausted retries (last status %d) for %s: %w", lastStatus, url, model.ErrSourceUnavailable)
}

func (c *Client) do(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.opts.UserAgent)
	for k, v := range c.opts.Cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}
