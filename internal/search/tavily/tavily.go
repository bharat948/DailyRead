package tavily

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"dailyread/internal/search"
)

const endpoint = "https://api.tavily.com/search"

type Provider struct {
	client *http.Client
	apiKey string
}

func New() *Provider {
	return &Provider{
		client: &http.Client{},
		apiKey: os.Getenv("TAVILY_API_KEY"),
	}
}

func (p *Provider) Name() string {
	return "tavily"
}

func (p *Provider) Healthy() bool {
	return p.apiKey != ""
}

type tavilyRequest struct {
	APIKey            string `json:"api_key"`
	Query             string `json:"query"`
	SearchDepth       string `json:"search_depth"`
	IncludeAnswer     bool   `json:"include_answer"`
	IncludeRawContent bool   `json:"include_raw_content"`
	MaxResults        int    `json:"max_results"`
	Topic             string `json:"topic,omitempty"`
}

type tavilyResponse struct {
	Results []struct {
		Title      string  `json:"title"`
		URL        string  `json:"url"`
		Content    string  `json:"content"`
		RawContent string  `json:"raw_content"`
		Score      float64 `json:"score"`
	} `json:"results"`
}

func (p *Provider) Search(ctx context.Context, q search.Query) ([]search.Result, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("TAVILY_API_KEY is not set")
	}

	reqBody := tavilyRequest{
		APIKey:            p.apiKey,
		Query:             q.Text,
		SearchDepth:       "advanced",
		IncludeAnswer:     false,
		IncludeRawContent: true, // We want the clean content for LLM
		MaxResults:        q.MaxResults,
		Topic:             q.Topic,
	}

	if reqBody.MaxResults <= 0 {
		reqBody.MaxResults = 5
	}

	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily API returned status %d", resp.StatusCode)
	}

	var tr tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}

	var results []search.Result
	for _, r := range tr.Results {
		content := r.RawContent
		if content == "" {
			content = r.Content
		}
		results = append(results, search.Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
			Content: content,
			Source:  p.Name(),
			Score:   r.Score,
		})
	}

	return results, nil
}
