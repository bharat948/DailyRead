package db

import (
	"database/sql"
	"fmt"

	"dailyread/internal/domain"
)

// =====================================================================
// Global research memory (shared corpus + caches)
// =====================================================================

// UpsertResource inserts a resource into the global corpus or, if the URL is
// already known, bumps fetch_count/last_fetched_at and fills in any newly-known
// fields without clobbering existing non-empty values.
func (r *Repository) UpsertResource(res *domain.Resource) error {
	_, err := r.db.Exec(`
		INSERT INTO resources (
			url_hash, url, domain, title, summary, content_text, type,
			word_count, quality_score, lang
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(url_hash) DO UPDATE SET
			fetch_count     = resources.fetch_count + 1,
			last_fetched_at = CURRENT_TIMESTAMP,
			title         = CASE WHEN excluded.title        != '' THEN excluded.title        ELSE resources.title        END,
			summary       = CASE WHEN excluded.summary      != '' THEN excluded.summary      ELSE resources.summary      END,
			content_text  = CASE WHEN excluded.content_text != '' THEN excluded.content_text ELSE resources.content_text END,
			type          = CASE WHEN excluded.type         != '' THEN excluded.type         ELSE resources.type         END,
			word_count    = CASE WHEN excluded.word_count   >  0  THEN excluded.word_count   ELSE resources.word_count   END,
			quality_score = CASE WHEN excluded.quality_score > 0  THEN excluded.quality_score ELSE resources.quality_score END
	`,
		res.URLHash, res.URL, res.Domain, res.Title, res.Summary, res.ContentText,
		nz(res.Type, "article"), res.WordCount, res.QualityScore, nz(res.Lang, "en"),
	)
	if err != nil {
		return fmt.Errorf("upsert resource: %w", err)
	}
	return nil
}

// GetResource returns a corpus entry by URL hash, or (nil, nil) if absent.
func (r *Repository) GetResource(urlHash string) (*domain.Resource, error) {
	res := &domain.Resource{}
	err := r.db.QueryRow(`
		SELECT url_hash, url, domain, title, summary, content_text, type,
		       word_count, quality_score, lang, fetch_count, first_seen_at, last_fetched_at
		FROM resources WHERE url_hash = ?
	`, urlHash).Scan(
		&res.URLHash, &res.URL, &res.Domain, &res.Title, &res.Summary, &res.ContentText,
		&res.Type, &res.WordCount, &res.QualityScore, &res.Lang, &res.FetchCount,
		&res.FirstSeenAt, &res.LastFetchedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get resource: %w", err)
	}
	return res, nil
}

// LinkTopicResource records that a resource is relevant to a topic (warm-start index).
func (r *Repository) LinkTopicResource(topic, urlHash string, relevance float64) error {
	_, err := r.db.Exec(`
		INSERT INTO topic_resources (topic, url_hash, relevance)
		VALUES (?, ?, ?)
		ON CONFLICT(topic, url_hash) DO UPDATE SET
			relevance    = MAX(topic_resources.relevance, excluded.relevance),
			last_seen_at = CURRENT_TIMESTAMP
	`, topic, urlHash, relevance)
	if err != nil {
		return fmt.Errorf("link topic resource: %w", err)
	}
	return nil
}

// GetTopicResources returns the best known resources for a topic, highest
// relevance/quality first — the corpus "warm start" for a new research trigger.
func (r *Repository) GetTopicResources(topic string, limit int) ([]domain.Resource, error) {
	rows, err := r.db.Query(`
		SELECT r.url_hash, r.url, r.domain, r.title, r.summary, r.content_text, r.type,
		       r.word_count, r.quality_score, r.lang, r.fetch_count, r.first_seen_at, r.last_fetched_at
		FROM topic_resources t
		JOIN resources r ON r.url_hash = t.url_hash
		WHERE t.topic = ?
		ORDER BY t.relevance DESC, r.quality_score DESC
		LIMIT ?
	`, topic, limit)
	if err != nil {
		return nil, fmt.Errorf("get topic resources: %w", err)
	}
	defer rows.Close()

	var out []domain.Resource
	for rows.Next() {
		var res domain.Resource
		if err := rows.Scan(
			&res.URLHash, &res.URL, &res.Domain, &res.Title, &res.Summary, &res.ContentText,
			&res.Type, &res.WordCount, &res.QualityScore, &res.Lang, &res.FetchCount,
			&res.FirstSeenAt, &res.LastFetchedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, res)
	}
	return out, rows.Err()
}

// GetSearchCache returns cached results for a query hash if present and newer
// than maxAgeSeconds; otherwise (nil, nil) so the caller does a fresh search.
func (r *Repository) GetSearchCache(queryHash string, maxAgeSeconds int) (*domain.SearchCacheEntry, error) {
	e := &domain.SearchCacheEntry{}
	cutoff := fmt.Sprintf("-%d seconds", maxAgeSeconds)
	err := r.db.QueryRow(`
		SELECT query_hash, query, results_json, provider, created_at
		FROM search_cache
		WHERE query_hash = ? AND created_at > datetime('now', ?)
	`, queryHash, cutoff).Scan(&e.QueryHash, &e.Query, &e.ResultsJSON, &e.Provider, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get search cache: %w", err)
	}
	return e, nil
}

// PutSearchCache stores (or refreshes) distilled results for a query.
func (r *Repository) PutSearchCache(queryHash, query, resultsJSON, provider string) error {
	_, err := r.db.Exec(`
		INSERT INTO search_cache (query_hash, query, results_json, provider)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(query_hash) DO UPDATE SET
			results_json = excluded.results_json,
			provider     = excluded.provider,
			created_at   = CURRENT_TIMESTAMP
	`, queryHash, query, resultsJSON, provider)
	if err != nil {
		return fmt.Errorf("put search cache: %w", err)
	}
	return nil
}

// =====================================================================
// User-specific memory (runs, digests, seen, profile)
// =====================================================================

// CreateRun inserts a new pipeline run row (status defaults to "running").
func (r *Repository) CreateRun(run *domain.Run) error {
	_, err := r.db.Exec(`
		INSERT INTO runs (id, user_id, trigger, status, stage)
		VALUES (?, ?, ?, ?, ?)
	`, run.ID, run.UserID, nz(run.Trigger, "manual"), nz(run.Status, "running"), run.Stage)
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	return nil
}

// FinishRun updates a run's terminal state, counts, token usage and stage.
func (r *Repository) FinishRun(run *domain.Run) error {
	_, err := r.db.Exec(`
		UPDATE runs SET
			status = ?, stage = ?, item_count = ?, tokens_input = ?, tokens_output = ?,
			error = ?, finished_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, run.Status, run.Stage, run.ItemCount, run.TokensInput, run.TokensOutput, run.Error, run.ID)
	if err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	return nil
}

// CreateDigestItem records a delivered item for a run.
func (r *Repository) CreateDigestItem(it *domain.DigestItem) error {
	_, err := r.db.Exec(`
		INSERT INTO digest_items (
			id, run_id, user_id, url_hash, interest_tag, title, url, summary, why, how, relevance, novelty
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, it.ID, it.RunID, it.UserID, it.URLHash, it.InterestTag, it.Title, it.URL,
		it.Summary, it.Why, it.How, it.Relevance, it.Novelty)
	if err != nil {
		return fmt.Errorf("create digest item: %w", err)
	}
	return nil
}

// GetRunsByUser returns a user's runs, most recent first.
func (r *Repository) GetRunsByUser(userID string, limit int) ([]domain.Run, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, trigger, status, stage, item_count, tokens_input, tokens_output, error, started_at, finished_at
		FROM runs WHERE user_id = ?
		ORDER BY started_at DESC LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get runs by user: %w", err)
	}
	defer rows.Close()

	var out []domain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

// GetRun returns a single run by id, or (nil, nil) if absent.
func (r *Repository) GetRun(id string) (*domain.Run, error) {
	run, err := scanRun(r.db.QueryRow(`
		SELECT id, user_id, trigger, status, stage, item_count, tokens_input, tokens_output, error, started_at, finished_at
		FROM runs WHERE id = ?
	`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get run: %w", err)
	}
	return run, nil
}

// GetDigestItemsByRun returns the items delivered in a specific run.
func (r *Repository) GetDigestItemsByRun(runID string) ([]domain.DigestItem, error) {
	rows, err := r.db.Query(`
		SELECT id, run_id, user_id, url_hash, interest_tag, title, url, summary, why, how, relevance, novelty, created_at
		FROM digest_items WHERE run_id = ?
		ORDER BY relevance DESC
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("get digest items by run: %w", err)
	}
	defer rows.Close()

	var out []domain.DigestItem
	for rows.Next() {
		var it domain.DigestItem
		if err := rows.Scan(
			&it.ID, &it.RunID, &it.UserID, &it.URLHash, &it.InterestTag, &it.Title, &it.URL,
			&it.Summary, &it.Why, &it.How, &it.Relevance, &it.Novelty, &it.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// scanRow is the subset of *sql.Row / *sql.Rows used by scanRun.
type scanRow interface {
	Scan(dest ...any) error
}

func scanRun(row scanRow) (*domain.Run, error) {
	var run domain.Run
	var finished sql.NullTime
	if err := row.Scan(
		&run.ID, &run.UserID, &run.Trigger, &run.Status, &run.Stage,
		&run.ItemCount, &run.TokensInput, &run.TokensOutput, &run.Error,
		&run.StartedAt, &finished,
	); err != nil {
		return nil, err
	}
	if finished.Valid {
		run.FinishedAt = &finished.Time
	}
	return &run, nil
}

// GetRecentDigestItems returns a user's most recently delivered items (newest first).
func (r *Repository) GetRecentDigestItems(userID string, limit int) ([]domain.DigestItem, error) {
	rows, err := r.db.Query(`
		SELECT id, run_id, user_id, url_hash, interest_tag, title, url, summary, why, how, relevance, novelty, created_at
		FROM digest_items WHERE user_id = ?
		ORDER BY created_at DESC LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get recent digest items: %w", err)
	}
	defer rows.Close()

	var out []domain.DigestItem
	for rows.Next() {
		var it domain.DigestItem
		if err := rows.Scan(
			&it.ID, &it.RunID, &it.UserID, &it.URLHash, &it.InterestTag, &it.Title, &it.URL,
			&it.Summary, &it.Why, &it.How, &it.Relevance, &it.Novelty, &it.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// MarkSeen records (or bumps) that a user has been shown a resource.
func (r *Repository) MarkSeen(userID, urlHash, runID string) error {
	_, err := r.db.Exec(`
		INSERT INTO user_seen (user_id, url_hash, first_run_id)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, url_hash) DO UPDATE SET
			times_shown  = user_seen.times_shown + 1,
			last_seen_at = CURRENT_TIMESTAMP
	`, userID, urlHash, runID)
	if err != nil {
		return fmt.Errorf("mark seen: %w", err)
	}
	return nil
}

// GetSeenHashes returns the set of resource hashes a user has already been shown,
// for per-user dedup and novelty scoring.
func (r *Repository) GetSeenHashes(userID string) (map[string]bool, error) {
	rows, err := r.db.Query(`SELECT url_hash FROM user_seen WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("get seen hashes: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		seen[h] = true
	}
	return seen, rows.Err()
}

// GetUserProfile returns a user's compacted profile, or an empty version-0
// profile if none exists yet.
func (r *Repository) GetUserProfile(userID string) (*domain.UserProfile, error) {
	p := &domain.UserProfile{}
	err := r.db.QueryRow(`
		SELECT user_id, compacted_summary, version, updated_at
		FROM user_profile WHERE user_id = ?
	`, userID).Scan(&p.UserID, &p.CompactedSummary, &p.Version, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return &domain.UserProfile{UserID: userID, Version: 0}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user profile: %w", err)
	}
	return p, nil
}

// UpsertUserProfile stores a new compacted summary, bumping the version.
func (r *Repository) UpsertUserProfile(userID, summary string) error {
	_, err := r.db.Exec(`
		INSERT INTO user_profile (user_id, compacted_summary, version, updated_at)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET
			compacted_summary = excluded.compacted_summary,
			version           = user_profile.version + 1,
			updated_at        = CURRENT_TIMESTAMP
	`, userID, summary)
	if err != nil {
		return fmt.Errorf("upsert user profile: %w", err)
	}
	return nil
}

// nz returns def when s is empty.
func nz(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
