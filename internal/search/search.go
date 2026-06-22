package search

import (
	"context"
	"time"
)

// Query represents a search request.
type Query struct {
	Text       string
	MaxResults int
	TimeRange  string // "week", "month", "year", "" (provider-best-effort)
	Topic      string // optional vertical hint
}

// Result represents a single search result.
type Result struct {
	Title     string
	URL       string
	Snippet   string
	Content   string     // full/partial page text when the provider returns it (e.g. Tavily)
	Source    string     // provider name
	Score     float64    // provider relevance, normalized 0..1 when available
	Published *time.Time // pointer because it might be null
}

// Searcher defines the interface for a web search provider.
type Searcher interface {
	Name() string
	Search(ctx context.Context, q Query) ([]Result, error)
	// Healthy returns true if the provider is currently considered usable (quota ok, etc)
	Healthy() bool
}
