package serpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"dailyread/internal/search"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func New() *Client {
	return &Client{
		apiKey:     os.Getenv("SERPAPI_API_KEY"),
		httpClient: &http.Client{},
	}
}

func (c *Client) Name() string {
	return "serpapi"
}

func (c *Client) Healthy() bool {
	return c.apiKey != ""
}

type serpApiResponse struct {
	OrganicResults []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
	} `json:"organic_results"`
}

func (c *Client) Search(ctx context.Context, q search.Query) ([]search.Result, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("SERPAPI_API_KEY not set")
	}

	endpoint := "https://serpapi.com/search"
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Add("engine", "google")
	params.Add("q", q.Text)
	params.Add("api_key", c.apiKey)
	params.Add("num", fmt.Sprintf("%d", q.MaxResults))

	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("serpapi returned status: %s", resp.Status)
	}

	var parsed serpApiResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var results []search.Result
	for _, item := range parsed.OrganicResults {
		if item.Link != "" {
			results = append(results, search.Result{
				Title:   item.Title,
				URL:     item.Link,
				Snippet: item.Snippet,
				Source:  "serpapi",
			})
		}
	}

	return results, nil
}
