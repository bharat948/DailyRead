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
