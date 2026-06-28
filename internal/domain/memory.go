package domain

import "time"

// ---- Global research memory ----

// Resource is one entry in the global, user-agnostic research corpus. It is the
// distilled, deduplicated record of a URL we have fetched and boiled down, reused
// across runs and users so research compounds instead of starting cold.
type Resource struct {
	URLHash       string    `json:"url_hash"`
	URL           string    `json:"url"`
	Domain        string    `json:"domain"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`       // boiled-down distillation
	ContentText   string    `json:"content_text"`  // extracted readable text (may be truncated)
	Type          string    `json:"type"`          // article | pdf | case_study | other
	WordCount     int       `json:"word_count"`
	QualityScore  float64   `json:"quality_score"` // 0..1
	Lang          string    `json:"lang"`
	FetchCount    int       `json:"fetch_count"`
	FirstSeenAt   time.Time `json:"first_seen_at"`
	LastFetchedAt time.Time `json:"last_fetched_at"`
}

// TopicResource links a topic/tag to a resource in the corpus with a relevance
// score, giving previously-researched topics a warm start.
type TopicResource struct {
	Topic      string    `json:"topic"`
	URLHash    string    `json:"url_hash"`
	Relevance  float64   `json:"relevance"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// SearchCacheEntry stores distilled results for a normalized query so the same
// search isn't re-run against providers within a freshness window.
type SearchCacheEntry struct {
	QueryHash   string    `json:"query_hash"`
	Query       string    `json:"query"`
	ResultsJSON string    `json:"results_json"`
	Provider    string    `json:"provider"`
	CreatedAt   time.Time `json:"created_at"`
}

// ---- User-specific memory ----

// Run is a single pipeline execution for a user.
type Run struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Trigger      string     `json:"trigger"` // scheduled | manual | catchup
	Status       string     `json:"status"`  // running | succeeded | partial | failed
	Stage        string     `json:"stage"`
	ItemCount    int        `json:"item_count"`
	TokensInput  int        `json:"tokens_input"`
	TokensOutput int        `json:"tokens_output"`
	Error        string     `json:"error"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

// DigestItem is one curated item delivered to a user in a run — the past-digest memory.
type DigestItem struct {
	ID          string    `json:"id"`
	RunID       string    `json:"run_id"`
	UserID      string    `json:"user_id"`
	URLHash     string    `json:"url_hash"`
	InterestTag string    `json:"interest_tag"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Summary     string    `json:"summary"`
	Why         string    `json:"why"` // per-user framing
	How         string    `json:"how"`
	Relevance   float64   `json:"relevance"`
	Novelty     float64   `json:"novelty"`
	CreatedAt   time.Time `json:"created_at"`
}

// UserProfile is the compacted, versioned long-term interest summary for a user.
type UserProfile struct {
	UserID           string    `json:"user_id"`
	CompactedSummary string    `json:"compacted_summary"`
	Version          int       `json:"version"`
	UpdatedAt        time.Time `json:"updated_at"`
}
