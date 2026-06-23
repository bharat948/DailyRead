package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"dailyread/internal/domain"
	"github.com/go-shiori/go-readability"
)

type Fetcher interface {
	Fetch(ctx context.Context, url string) (string, error)
}

type defaultFetcher struct {
	client *http.Client
}

func New() Fetcher {
	return &defaultFetcher{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (f *defaultFetcher) Fetch(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36 DailyRead/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	if stats := domain.StatsFromContext(ctx); stats != nil {
		stats.AddWebRequest()
	}

	// Parse the URL for readability
	parsedURL, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Use readability to extract the main content
	article, err := readability.FromReader(resp.Body, parsedURL)
	if err != nil {
		return "", fmt.Errorf("readability extraction failed: %w", err)
	}

	return article.TextContent, nil
}
