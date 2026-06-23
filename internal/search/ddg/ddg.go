package ddg

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"dailyread/internal/search"
)

// A simple regex to parse DuckDuckGo's lite HTML.
// In a production app with heavier scraping needs, we might use goquery.
var (
	resultBlockRegex = regexp.MustCompile(`(?s)<a rel="nofollow" href="([^"]+)" class="result-url">[^<]*</a>.*?<a rel="nofollow" href="[^"]+" class="result-snippet"[^>]*>(.*?)</a>`)
	titleRegex       = regexp.MustCompile(`(?s)<a rel="nofollow" href="[^"]+">(.*?)</a>`)
)

type Provider struct {
	client *http.Client
}

func New() *Provider {
	return &Provider{
		client: &http.Client{},
	}
}

func (p *Provider) Name() string {
	return "ddg"
}

func (p *Provider) Healthy() bool {
	return true // Keyless, always theoretically available
}

func (p *Provider) Search(ctx context.Context, q search.Query) ([]search.Result, error) {
	// DuckDuckGo Lite endpoint
	u := fmt.Sprintf("https://lite.duckduckgo.com/lite/?q=%s", url.QueryEscape(q.Text))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://lite.duckduckgo.com/lite/", strings.NewReader("q="+url.QueryEscape(q.Text)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	_ = u // Unused if using POST, but lite handles POST better.

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("duckduckgo returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	html := string(bodyBytes)

	linkRe := regexp.MustCompile(`(?s)<a rel="nofollow" href="([^"]+)" class='result-link'>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`(?s)<td class='result-snippet'>\s*(.*?)\s*</td>`)
	
	links := linkRe.FindAllStringSubmatch(html, -1)
	snippets := snippetRe.FindAllStringSubmatch(html, -1)
	
	var results []search.Result
	for i := 0; i < len(links) && i < len(snippets); i++ {
		if q.MaxResults > 0 && i >= q.MaxResults {
			break
		}
		
		results = append(results, search.Result{
			Title:   stripTags(links[i][2]),
			URL:     links[i][1],
			Snippet: stripTags(snippets[i][1]),
			Content: stripTags(snippets[i][1]), // DDG doesn't provide full content
			Source:  p.Name(),
			Score:   0.5, // Fallback score
		})
	}

	return results, nil
}

func stripTags(html string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(html, "")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#x27;", "'")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	return strings.TrimSpace(text)
}
