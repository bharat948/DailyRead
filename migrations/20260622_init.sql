-- +goose Up
-- runs: one row per scheduled/catch-up/manual pipeline execution
CREATE TABLE runs (
  id              INTEGER PRIMARY KEY,
  scheduled_for   TEXT NOT NULL,           -- the window this run covers
  trigger         TEXT NOT NULL,           -- scheduled | catchup | manual
  status          TEXT NOT NULL,           -- pending|running|succeeded|partial|failed
  stage           TEXT NOT NULL,           -- last completed stage (resume point)
  email_idem_key  TEXT,
  email_sent_at   TEXT,
  error           TEXT,
  tokens_input    INTEGER DEFAULT 0,
  tokens_output   INTEGER DEFAULT 0,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);

-- suggestions: the curated items delivered in a run
CREATE TABLE suggestions (
  id            INTEGER PRIMARY KEY,
  run_id        INTEGER NOT NULL REFERENCES runs(id),
  interest_tag  TEXT NOT NULL,
  title         TEXT NOT NULL,
  url           TEXT NOT NULL,
  type          TEXT,                       -- case_study|article|pdf
  why           TEXT,
  how           TEXT,
  summary       TEXT,
  slot          TEXT,                       -- sat_am|sat_pm|sun
  relevance     REAL,
  novelty       REAL,
  source        TEXT,                       -- provider that surfaced it
  download_path TEXT,
  created_at    TEXT NOT NULL
);

-- seen_resources: dedup + novelty memory across runs
CREATE TABLE seen_resources (
  url_hash         TEXT PRIMARY KEY,        -- sha256(normalized url)
  url              TEXT NOT NULL,
  title            TEXT,
  first_run_id     INTEGER REFERENCES runs(id),
  times_suggested  INTEGER NOT NULL DEFAULT 1,
  last_seen_at     TEXT NOT NULL
);

-- profile: single-row (user_id=1) rolling compacted reading profile
CREATE TABLE profile (
  user_id            INTEGER PRIMARY KEY,   -- always 1 for now
  compacted_summary  TEXT NOT NULL,
  version            INTEGER NOT NULL,
  updated_at         TEXT NOT NULL
);

-- downloads: stored files
CREATE TABLE downloads (
  id          INTEGER PRIMARY KEY,
  run_id      INTEGER NOT NULL REFERENCES runs(id),
  url         TEXT NOT NULL,
  path        TEXT NOT NULL,
  bytes       INTEGER,
  sha256      TEXT,
  created_at  TEXT NOT NULL
);

-- provider_health: circuit-breaker / quota state persisted across restarts
CREATE TABLE provider_health (
  provider        TEXT PRIMARY KEY,
  consecutive_err INTEGER NOT NULL DEFAULT 0,
  breaker_state   TEXT NOT NULL DEFAULT 'closed',
  used_this_month INTEGER NOT NULL DEFAULT 0,
  month           TEXT,                     -- yyyy-mm for cap reset
  last_failure_at TEXT
);

-- +goose Down
DROP TABLE provider_health;
DROP TABLE downloads;
DROP TABLE profile;
DROP TABLE seen_resources;
DROP TABLE suggestions;
DROP TABLE runs;
