package db

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func runMigrations(db *sql.DB) error {
	goose.SetBaseFS(nil)
	
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}

	// We'll define up migrations explicitly to avoid embedding files
	return up(db)
}

func up(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS user_configs (
			user_id TEXT PRIMARY KEY,
			schedule_enabled BOOLEAN DEFAULT 0,
			schedule_cron TEXT DEFAULT '0 9 * * 6',
			schedule_timezone TEXT DEFAULT 'UTC',
			smtp_host TEXT DEFAULT '',
			smtp_port INTEGER DEFAULT 587,
			smtp_user TEXT DEFAULT '',
			smtp_pass_encrypted TEXT DEFAULT '',
			openai_key_encrypted TEXT DEFAULT '',
			models_provider TEXT DEFAULT 'openai',
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS interests (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			tag TEXT NOT NULL,
			intensity TEXT DEFAULT 'medium',
			is_primary BOOLEAN DEFAULT 0,
			types TEXT DEFAULT '',
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,

		// ---- Global research memory (user-agnostic, shared across runs/users) ----

		// resources: the distilled research corpus + fetch cache. Every URL we ever
		// fetch is stored once and reused, so research compounds instead of restarting.
		`CREATE TABLE IF NOT EXISTS resources (
			url_hash        TEXT PRIMARY KEY,
			url             TEXT NOT NULL,
			domain          TEXT DEFAULT '',
			title           TEXT DEFAULT '',
			summary         TEXT DEFAULT '',
			content_text    TEXT DEFAULT '',
			type            TEXT DEFAULT 'article',
			word_count      INTEGER DEFAULT 0,
			quality_score   REAL DEFAULT 0,
			lang            TEXT DEFAULT 'en',
			fetch_count     INTEGER DEFAULT 1,
			first_seen_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,

		// topic_resources: which resources are relevant to which topic, so a
		// previously-researched topic gets a warm start from the corpus.
		`CREATE TABLE IF NOT EXISTS topic_resources (
			topic        TEXT NOT NULL,
			url_hash     TEXT NOT NULL,
			relevance    REAL DEFAULT 0,
			last_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (topic, url_hash),
			FOREIGN KEY (url_hash) REFERENCES resources(url_hash) ON DELETE CASCADE
		);`,

		// search_cache: distilled web-search results keyed by normalized query, to
		// avoid re-hitting providers for the same query within a freshness window.
		`CREATE TABLE IF NOT EXISTS search_cache (
			query_hash   TEXT PRIMARY KEY,
			query        TEXT NOT NULL,
			results_json TEXT NOT NULL,
			provider     TEXT DEFAULT '',
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,

		// ---- User-specific memory (everything scoped by user_id) ----

		// runs: one row per pipeline execution for a user.
		`CREATE TABLE IF NOT EXISTS runs (
			id            TEXT PRIMARY KEY,
			user_id       TEXT NOT NULL,
			trigger       TEXT NOT NULL DEFAULT 'manual',
			status        TEXT NOT NULL DEFAULT 'running',
			stage         TEXT DEFAULT '',
			item_count    INTEGER DEFAULT 0,
			tokens_input  INTEGER DEFAULT 0,
			tokens_output INTEGER DEFAULT 0,
			error         TEXT DEFAULT '',
			started_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			finished_at   DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,

		// digest_items: what was actually delivered to a user in a run (past-digest memory).
		`CREATE TABLE IF NOT EXISTS digest_items (
			id           TEXT PRIMARY KEY,
			run_id       TEXT NOT NULL,
			user_id      TEXT NOT NULL,
			url_hash     TEXT NOT NULL,
			interest_tag TEXT DEFAULT '',
			title        TEXT DEFAULT '',
			url          TEXT DEFAULT '',
			summary      TEXT DEFAULT '',
			why          TEXT DEFAULT '',
			how          TEXT DEFAULT '',
			relevance    REAL DEFAULT 0,
			novelty      REAL DEFAULT 0,
			created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,

		// user_seen: per-user record of every resource already shown -> dedup + novelty.
		`CREATE TABLE IF NOT EXISTS user_seen (
			user_id      TEXT NOT NULL,
			url_hash     TEXT NOT NULL,
			first_run_id TEXT DEFAULT '',
			times_shown  INTEGER DEFAULT 1,
			last_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, url_hash),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,

		// user_profile: compacted, versioned long-term interest summary per user.
		`CREATE TABLE IF NOT EXISTS user_profile (
			user_id           TEXT PRIMARY KEY,
			compacted_summary TEXT NOT NULL DEFAULT '',
			version           INTEGER NOT NULL DEFAULT 0,
			updated_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,

		// Indexes for the common access paths.
		`CREATE INDEX IF NOT EXISTS idx_topic_resources_topic ON topic_resources(topic);`,
		`CREATE INDEX IF NOT EXISTS idx_resources_domain ON resources(domain);`,
		`CREATE INDEX IF NOT EXISTS idx_runs_user ON runs(user_id, started_at);`,
		`CREATE INDEX IF NOT EXISTS idx_digest_items_user ON digest_items(user_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_digest_items_run ON digest_items(run_id);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("failed to execute migration: %s, err: %w", q, err)
		}
	}
	
	// Add new column to existing table. Ignore error if it already exists.
	db.Exec(`ALTER TABLE user_configs ADD COLUMN schedule_enabled BOOLEAN DEFAULT 0;`)
	
	return nil
}
