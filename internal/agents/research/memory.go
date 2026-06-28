package research

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"dailyread/internal/domain"
	"dailyread/internal/search"
)

// searchCacheTTLSeconds bounds how long a cached web search is reused before we
// re-query providers. 3 days keeps the corpus fresh without re-paying every run.
const searchCacheTTLSeconds = 3 * 24 * 60 * 60

// fetchSnippetLimit caps how much fetched text we hand back to the model per call.
const fetchSnippetLimit = 4000

// Memory is the slice of the global research store the researcher needs. It is
// satisfied by *db.Repository; declared here so the agent stays decoupled and
// testable with a fake.
type Memory interface {
	GetSearchCache(queryHash string, maxAgeSeconds int) (*domain.SearchCacheEntry, error)
	PutSearchCache(queryHash, query, resultsJSON, provider string) error
	GetResource(urlHash string) (*domain.Resource, error)
	UpsertResource(res *domain.Resource) error
	LinkTopicResource(topic, urlHash string, relevance float64) error
	GetTopicResources(topic string, limit int) ([]domain.Resource, error)
}

// SearchProvider is the search surface the researcher uses; *search.Router satisfies it.
type SearchProvider interface {
	Search(ctx context.Context, q search.Query) ([]search.Result, error)
}

// memSearch boils a web search down into the global corpus: it serves a fresh
// cached result when available, otherwise queries providers, then persists both
// the distilled results (as resources + topic links) and the raw result set
// (as a search-cache entry) so the same query is never re-paid within the TTL.
func (r *Researcher) memSearch(ctx context.Context, topic, query string) (string, error) {
	qhash := domain.HashQuery(query)

	if r.mem != nil {
		if hit, err := r.mem.GetSearchCache(qhash, searchCacheTTLSeconds); err == nil && hit != nil {
			slog.Info("search memory hit", "topic", topic, "query", query)
			return hit.ResultsJSON, nil
		}
	}

	results, err := r.searcher.Search(ctx, search.Query{Text: query, MaxResults: 5})
	if err != nil {
		return "", err
	}
	resBytes, _ := json.Marshal(results)

	if r.mem != nil {
		for _, res := range results {
			if res.URL == "" {
				continue
			}
			h := domain.HashURL(res.URL)
			_ = r.mem.UpsertResource(&domain.Resource{
				URLHash:     h,
				URL:         res.URL,
				Domain:      domain.DomainOf(res.URL),
				Title:       res.Title,
				Summary:     res.Snippet,
				ContentText: res.Content,
				WordCount:   wordCount(res.Content),
			})
			_ = r.mem.LinkTopicResource(topic, h, weakRelevance(res.Score))
		}
		_ = r.mem.PutSearchCache(qhash, query, string(resBytes), providerOf(results))
	}

	return string(resBytes), nil
}

// memFetch serves a page from the corpus when we already have its text, else
// fetches it live and writes the distilled content back for future reuse.
func (r *Researcher) memFetch(ctx context.Context, rawURL string) (string, error) {
	h := domain.HashURL(rawURL)

	if r.mem != nil {
		if res, err := r.mem.GetResource(h); err == nil && res != nil && res.ContentText != "" {
			slog.Info("fetch memory hit", "url", rawURL)
			return truncate(res.ContentText, fetchSnippetLimit), nil
		}
	}

	content, err := r.fetcher.Fetch(ctx, rawURL)
	if err != nil {
		return "", err
	}

	if r.mem != nil {
		_ = r.mem.UpsertResource(&domain.Resource{
			URLHash:     h,
			URL:         rawURL,
			Domain:      domain.DomainOf(rawURL),
			ContentText: content,
			WordCount:   wordCount(content),
		})
	}

	return truncate(content, fetchSnippetLimit), nil
}

// warmStartContext returns a prompt fragment listing resources the corpus
// already knows for this topic, nudging the agent to reuse or surpass them.
func (r *Researcher) warmStartContext(topic string) string {
	if r.mem == nil {
		return ""
	}
	known, err := r.mem.GetTopicResources(topic, 5)
	if err != nil || len(known) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nOur research memory already holds these resources for this topic from prior runs. ")
	b.WriteString("Reuse one only if it is still excellent; otherwise prioritise finding NEW, higher-quality material:\n")
	for _, k := range known {
		title := k.Title
		if title == "" {
			title = k.URL
		}
		b.WriteString(fmt.Sprintf("- %s (%s)\n", title, k.URL))
	}
	return b.String()
}

// recordCandidates promotes the agent's final picks into the corpus with a
// stronger topic relevance and a quality score derived from the agent's rating.
func (r *Researcher) recordCandidates(topic string, candidates []domain.Candidate) {
	if r.mem == nil {
		return
	}
	for _, c := range candidates {
		if c.URL == "" {
			continue
		}
		h := domain.HashURL(c.URL)
		q := float64(c.Relevance) / 10
		_ = r.mem.UpsertResource(&domain.Resource{
			URLHash:      h,
			URL:          c.URL,
			Domain:       domain.DomainOf(c.URL),
			Title:        c.Title,
			Summary:      c.Summary,
			WordCount:    c.WordCount,
			QualityScore: q,
		})
		_ = r.mem.LinkTopicResource(topic, h, q)
	}
}

func wordCount(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Fields(s))
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "\n...[TRUNCATED]"
	}
	return s
}

func providerOf(results []search.Result) string {
	if len(results) > 0 && results[0].Source != "" {
		return results[0].Source
	}
	return "router"
}

// weakRelevance clamps a provider score into 0..1, defaulting low when unknown.
func weakRelevance(score float64) float64 {
	switch {
	case score <= 0:
		return 0.2
	case score > 1:
		return 1
	default:
		return score
	}
}
