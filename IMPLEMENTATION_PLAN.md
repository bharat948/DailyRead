# DailyRead — Implementation Plan

> A Go service that, on a weekly cadence, performs deep web research and curates a
> personalized reading list (case studies, PDFs, articles) for a single user, then
> emails a "what / how / why to read" digest with links and downloads. It learns from
> a compacted history of past suggestions to track the user's evolving interests.

**Status:** Planning · **Target language:** Go 1.26 · **Primary platform:** Windows (dev), Linux (deploy) · **Mode:** single-user, no auth

---

## 1. Goals & Non-Goals

### Goals
- Curate weekly reading material driven by a config of **interests** (multiple, one **primary**) and **intensity** per interest.
- Run **deep web research** on a configurable schedule (e.g., every Saturday morning) using **free web-search providers** (≥2, with fallback).
- Deliver a single **email digest** explaining *what* to read, *how* to read it (depth, time budget, order across the weekend), and *why* it matters to this user — with links and downloadable PDFs.
- Maintain a **compacted history** / long-term reading profile so the system understands the user's reading patterns and latent interests.
- Use a **multi-agent design** that routes small tasks to small models and deep reasoning to the strongest model.
- Be **fault tolerant, reliable, and extensively error-handled** — a missed or partially-failed run must recover, not silently drop the week.

### Non-Goals (for now)
- No authentication / multi-tenant support (single user). The design keeps a `user_id` seam so multi-user is a later, additive change.
- No web UI. Interaction is via a config file + CLI. (A read-only status dashboard is a future enhancement.)
- No paid search APIs as a hard dependency (free tiers only; paid is opt-in via config).
- No real-time / on-demand research (the cadence is weekly; an on-demand `run-now` command exists for testing and manual triggers).

---

## 2. Requirements Traceability

Every requirement maps to a concrete part of the design so nothing is lost.

| Requirement | Where addressed |
|---|---|
| **FR1** Email with summary/description of what/how/why + links + downloads | §6 Pipeline (Curate stage), §10 Email, Appendix C example |
| **FR2** User can change trigger time | §11 Scheduler (config-driven cron, hot reload), §12 Config |
| **FR3** Multiple interests, one primary tag | §12 Config model, §7 Intensity allocation |
| **FR4** Compacted history → learn reading pattern & latent interest | §9 History & Profile, §13 Data model (`profile`, `suggestions`, `seen_resources`) |
| **NFR1** Fault tolerant & reliable | §14 Resilience (checkpoint/resume, catch-up, idempotency) |
| **NFR2** Single user, no auth | §1 Non-Goals, single `user_id=1` seam |
| **NFR3** Extensive error handling | §15 Error handling strategy |
| **NFR4** Multi-agent, smaller models for smaller tasks | §8 Multi-agent & model routing |
| **NFR5** ≥2 free search providers, used extensively | §5 Search providers (Tavily/Brave/DuckDuckGo/SearXNG + fallback) |

---

## 3. High-Level Architecture

```
                       ┌──────────────────────────────────────────────┐
                       │                 dailyread (daemon)           │
                       │                                              │
   config.yaml ──────▶ │  Config loader  ◀── fsnotify hot-reload      │
   (interests,         │       │                                      │
    intensity,         │       ▼                                      │
    schedule)          │   Scheduler (robfig/cron) ── catch-up        │
                       │       │  fires weekly job                    │
                       │       ▼                                      │
                       │   Pipeline Orchestrator                      │
                       │   ┌──────────────────────────────────────┐  │
                       │   │ 1 Load config + compacted profile     │ │
                       │   │ 2 Plan queries      (Agent: Haiku)    │ │
                       │   │ 3 Deep research loop(Agent: Opus +    │ │
                       │   │      web_search/fetch tools)         │ │
                       │   │ 4 Triage/score      (Agent: Haiku)   │ │
                       │   │ 5 Curate digest     (Agent: Opus)    │ │
                       │   │ 6 Download PDFs                       │ │
                       │   │ 7 Render + send email                │ │
                       │   │ 8 Update history + compact profile   │ │
                       │   └──────────────────────────────────────┘  │
                       │       │            │             │           │
                       └───────┼────────────┼─────────────┼──────────┘
                               ▼            ▼             ▼
                     ┌─────────────┐ ┌─────────────┐ ┌──────────────┐
                     │ Search svc  │ │ LLM client  │ │ Email sender │
                     │ Tavily/Brave│ │ Anthropic   │ │ SMTP / API   │
                     │ DDG/SearXNG │ │ (Go SDK)    │ │              │
                     │ +breaker    │ │ +router     │ │              │
                     └─────────────┘ └─────────────┘ └──────────────┘
                               │            │             │
                               ▼            ▼             ▼
                     ┌────────────────────────────────────────────┐
                     │  Store: SQLite (modernc, pure-Go)          │
                     │  runs · suggestions · seen_resources ·     │
                     │  profile · provider_health · downloads     │
                     └────────────────────────────────────────────┘
```

**Key idea:** the weekly job is a **checkpointed, resumable pipeline**. Each stage writes its output to the store and advances a `runs.stage` marker, so a crash mid-run resumes from the last completed stage instead of re-doing (and re-paying for) prior LLM/search work.

---

## 4. Technology Choices

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go 1.26 | Single static binary, good concurrency, cross-platform |
| DB | **SQLite via `modernc.org/sqlite`** (pure Go, no CGO) | Reliable embedded transactional store; **no CGO** matters on Windows; trivial backup (single file) |
| Migrations | `pressly/goose` or hand-rolled `embed`-ded SQL | Versioned, embedded in binary |
| Scheduler | `robfig/cron/v3` | Battle-tested cron with timezone support |
| Config | YAML via `gopkg.in/yaml.v3` + `env` overrides for secrets | Human-editable; secrets stay in env, not committed |
| Config reload | `fsnotify/fsnotify` | Hot-reload interest/schedule changes without restart |
| LLM | **`github.com/anthropics/anthropic-sdk-go`** | Official SDK; typed model constants, tool-use loop, streaming, prompt caching |
| HTTP client | stdlib `net/http` + per-request timeouts + retry wrapper | No heavy dependency; full control over timeouts/backoff |
| Article extraction | `go-shiori/go-readability` | Strips boilerplate → clean text for summarization |
| PDF text (optional) | `ledongthuc/pdf` | Extract text from downloaded PDFs for summaries |
| Retry/backoff | `cenkalti/backoff/v4` (or hand-rolled) | Exponential backoff + jitter |
| Circuit breaker | `sony/gobreaker` | Per-provider breaker for search APIs |
| Email | `wneessen/go-mail` (SMTP) + pluggable API sender | Modern SMTP; abstraction allows API providers (Resend/Brevo) later |
| Logging | stdlib `log/slog` (JSON handler) | Structured logs, zero dependency |
| CLI | `spf13/cobra` | Subcommands (`run`, `run-now`, `config`, `history`, `migrate`, `test-email`) |
| Templating | stdlib `html/template` + `text/template` | HTML + plaintext email bodies |

> **Windows note:** Everything above is pure-Go / CGO-free, so `go build` produces a single `dailyread.exe` with no external runtime. The SQLite file and downloaded PDFs live under a configurable data dir (default `%LOCALAPPDATA%\dailyread` on Windows, `$XDG_DATA_HOME/dailyread` on Linux).

---

## 5. Web Search Providers (≥2 free, used extensively)

The system never depends on one provider. A `Searcher` interface is implemented by multiple adapters; a **router** tries them in priority order with **circuit breakers** and **quota awareness**, and an agentic loop can call search **many times per run**.

### 5.1 Interface

```go
// internal/search/search.go
type Query struct {
    Text        string
    MaxResults  int
    TimeRange   string   // "week","month","year","" (provider-best-effort)
    Topic       string   // optional vertical hint
}

type Result struct {
    Title    string
    URL      string
    Snippet  string
    Content  string  // full/partial page text when the provider returns it (e.g. Tavily)
    Source   string  // provider name
    Score    float64 // provider relevance, normalized 0..1 when available
    Published *time.Time
}

type Searcher interface {
    Name() string
    Search(ctx context.Context, q Query) ([]Result, error)
    // Quota/health hints for the router:
    Healthy() bool
}
```

### 5.2 Adapters (free tiers)

| Provider | Key needed | Notes |
|---|---|---|
| **Tavily** | API key (free tier) | AI-search-optimized; returns cleaned **content**, great for LLM consumption. **Primary.** |
| **Brave Search API** | API key (free tier ~2k/mo) | General web index; good breadth. **Secondary.** |
| **DuckDuckGo** | none | Unofficial HTML/Instant-Answer endpoint; **keyless fallback** (lower quality, rate-limited) — always available. |
| **SearXNG** | none (self-host URL) | Self-hosted metasearch; fully free; opt-in via `searxng.base_url`. **Optional fallback.** |

> Adapters are isolated behind the interface, so adding/removing a provider is config + one file. Keys come from env (`TAVILY_API_KEY`, `BRAVE_API_KEY`, …); a provider with no key is automatically skipped, not errored.

### 5.3 Router policy

```
router.Search(q):
  for provider in priority_order (config-driven, default: tavily, brave, searxng, ddg):
     if breaker[provider].open: continue
     try provider.Search(q) with timeout + retry(2, backoff+jitter)
        on success: record health OK; return results (optionally merge from N providers)
        on error:   record failure; trip breaker after K consecutive failures; continue
  if all failed: return AggregateError (pipeline degrades gracefully — see §15)
```

- **Fan-out mode (config `search.fanout: true`):** query the top-2 healthy providers in parallel and **dedup+merge** results by normalized URL. Improves recall (each provider is blind to the others). Default for the deep-research loop.
- **Quota guard:** a token-bucket per provider (configurable monthly cap) avoids burning a free tier in one run. When a provider's bucket is empty it's treated as temporarily unhealthy.
- **Provider health** is persisted (`provider_health` table) so breaker state survives restarts within a run window.

---

## 6. The Weekly Pipeline (deep research flow)

The orchestrator runs eight stages. Each stage is **idempotent**, writes a checkpoint, and can be resumed.

```
Stage 0  ACQUIRE LOCK   — single-flight per run_id (no concurrent duplicate run)
Stage 1  LOAD           — config snapshot + compacted profile + recent suggestions (dedup set)
Stage 2  PLAN           — Query Planner agent → list of search queries per interest
Stage 3  RESEARCH       — Deep-research agent loop: search → fetch → assess → search again
Stage 4  TRIAGE         — Triage agent scores/filters candidates (relevance, novelty, intensity fit)
Stage 5  CURATE         — Curator agent produces the structured reading plan (what/how/why, ordering)
Stage 6  DOWNLOAD       — fetch PDFs / archive links for offline reading; store paths
Stage 7  DELIVER        — render HTML+text email, send (idempotent: one email per run)
Stage 8  LEARN          — persist suggestions; compact new info into long-term profile
```

### Stage 2 — Query Planning
The **Query Planner** (small model, Haiku) takes interests + intensity + the compacted profile + recently-seen topics and emits a structured set of queries:
- Distribute query budget across interests **weighted by intensity**, with a floor/boost for the **primary** interest (§7).
- Bias toward **novelty** (avoid topics already covered, surfaced from `seen_resources`) while reinforcing the user's demonstrated reading pattern.
- Output is **structured JSON** (validated via structured outputs / strict tool) → `[]PlannedQuery{interest, query, rationale}`.

### Stage 3 — Deep Research (agentic loop)
This is the core "deep web research." The **Deep-Research agent** (Opus 4.8, adaptive thinking, effort `high`) runs a bounded **tool-use loop**:

- Tools exposed to the model (client-side, backed by our services):
  - `web_search(query, time_range, max_results)` → calls the **search router** (§5).
  - `fetch_url(url)` → fetches + extracts readable text (`go-readability`) or PDF metadata.
- Loop: the agent plans → searches → reads the most promising results → identifies gaps → searches again, until it has enough strong candidates or hits the **budget** (max rounds, max tool calls, max tokens — see §14 budgets).
- The agent accumulates a candidate set with provenance (which query/provider surfaced it).
- Guardrails: hard caps on rounds and on total search/fetch calls; per-call timeouts; dedup of fetched URLs.

> Why agentic here (and only here): query → search → read → re-search is genuinely open-ended and benefits from the model deciding its trajectory. Every other stage is a single, well-specified call (cheaper, on smaller models).

### Stage 4 — Triage / Scoring
The **Triage agent** (Haiku for volume; escalate borderline items to Sonnet) scores each candidate on:
- **Relevance** to the matched interest.
- **Novelty** vs `seen_resources` (penalize near-duplicates; cosine/heuristic title+URL match, optionally lightweight embeddings later).
- **Intensity fit** (a "deep" interest tolerates long/dense material; a "light" interest prefers digestible reads).
- **Type fit** (case study / PDF / article preference per interest, if configured).
Output: ranked, filtered candidate list sized to the **per-interest budget** (§7).

### Stage 5 — Curation
The **Curator agent** (Opus 4.8, adaptive thinking, effort `high`/`xhigh`) produces the final reading plan as **structured output**:
- Per item: `title, url, type, why_it_matters (ties to interest + history), how_to_read (skim/deep, time budget, focus points), summary/abstract, suggested_slot (Sat AM / Sat PM / Sun)`.
- A short **intro** tying the week's themes to the user's evolving interests.
- A **weekend reading schedule** ordered by intensity and dependency.

### Stage 6 — Download
For PDFs and downloadable case studies: stream to the data dir (`downloads/<run_id>/`), checksum, record path + bytes in `downloads`. Failures here are **non-fatal** — the email still links to the source.

### Stage 7 — Deliver
Render the curated plan into HTML + plaintext (multipart). Send via the configured email channel. **Idempotency:** the run carries an `email_idempotency_key`; if the run resumes after a crash *after* send, it does not re-send (checked via `runs.email_sent_at`).

### Stage 8 — Learn
- Insert each suggestion into `suggestions`, add URLs to `seen_resources` (for future dedup).
- The **Compactor agent** (Haiku) folds this week's themes/outcomes into the **long-term `profile`** (a rolling summary, versioned) — see §9.

---

## 7. Interests, Intensity & the Primary Tag

```yaml
interests:
  - tag: "distributed-systems"
    primary: true
    intensity: high        # high | medium | light  (or 1..5)
    types: [case_study, article, pdf]
  - tag: "product-management"
    intensity: medium
  - tag: "ai-evals"
    intensity: light
```

**Allocation algorithm** (turns intensity into a per-interest resource budget for the week):
1. Map intensity → weight (`high=3, medium=2, light=1`, configurable).
2. Give the **primary** interest a boost (e.g. `+1` weight or a guaranteed floor of N items).
3. Total weekly item budget `B` (config `weekly.max_items`, default ~6) is distributed proportionally to weights, primary first, remainder by largest fractional share.
4. Intensity also drives **reading depth** signals passed to Curator: a `high` interest gets longer/deeper material and more time budget; `light` gets short digestible reads.

Constraints validated at config load: exactly **one** `primary: true`; at least one interest; intensities within allowed set.

---

## 8. Multi-Agent Design & Model Routing

A small set of single-purpose agents, each pinned to the cheapest model that does the job well. This directly satisfies NFR4 ("use smaller models for smaller tasks").

| Agent | Task | Model | Thinking / effort | Why this tier |
|---|---|---|---|---|
| **Query Planner** | interests+history → search queries (structured) | `claude-haiku-4-5` | thinking disabled | cheap, structured, high volume |
| **Deep-Research** | agentic search→read→re-search loop | `claude-opus-4-8` | adaptive, effort `high` | open-ended reasoning + tool use |
| **Triage** | score/filter candidates (structured) | `claude-haiku-4-5` (escalate borderline → `claude-sonnet-4-6`) | Haiku: disabled; Sonnet: adaptive | high volume, simple decisions |
| **Per-doc Summarizer** | summarize a fetched article/PDF | `claude-haiku-4-5` | disabled | one doc per call, cheap |
| **Curator** | final what/how/why plan (structured) | `claude-opus-4-8` | adaptive, effort `high`/`xhigh` | deep synthesis, user-facing quality |
| **Compactor** | fold week into long-term profile | `claude-haiku-4-5` | disabled | summarization |

### Model facts that constrain the implementation (verified against the Anthropic Go SDK)

- **Model IDs / constants:** `anthropic.ModelClaudeOpus4_8` (`"claude-opus-4-8"`), `anthropic.ModelClaudeSonnet4_6` (`"claude-sonnet-4-6"`), `anthropic.ModelClaudeHaiku4_5_20251001` (`"claude-haiku-4-5"`).
- **Pricing / context (for budget math, §14):**
  | Model | Input $/1M | Output $/1M | Context | Max output |
  |---|---|---|---|---|
  | Opus 4.8 | $5.00 | $25.00 | 1M | 128K |
  | Sonnet 4.6 | $3.00 | $15.00 | 1M | 64K |
  | Haiku 4.5 | $1.00 | $5.00 | 200K | 64K |
- **Thinking:** Opus 4.8 / Sonnet 4.6 use **adaptive** thinking (`thinking: {type: "adaptive"}`); `budget_tokens` is **rejected (400)** on Opus 4.8. **Haiku 4.5 does *not* support adaptive thinking or the `effort` parameter** — leave thinking unset/`disabled` for Haiku agents and do **not** pass `output_config.effort` to Haiku (it errors). This is encapsulated in the model router so call sites can't get it wrong.
- **Effort:** `output_config.effort` (`low|medium|high|xhigh|max`) on Opus 4.8 / Sonnet 4.6 only. Use `high` for the research/curation agents; lower for cheap stages.
- **Structured outputs** (Opus 4.8 / Sonnet 4.6 / Haiku 4.5): use `OutputConfig.Format` (json_schema) or `strict: true` tools so Planner/Triage/Curator return validated JSON — no brittle string parsing.
- **Prompt caching:** put each agent's stable system prompt + tool list first with `CacheControl: anthropic.NewCacheControlEphemeralParam()`; keep volatile content (the week's data) after the breakpoint. Min cacheable prefix is 4096 tokens on Opus/Haiku, 2048 on Sonnet — only the large research/curation system prompts will actually cache.
- **Streaming:** the Curator can emit up to 128K tokens; use `client.Messages.NewStreaming(...)` and `message.Accumulate(...)` for any call with large `max_tokens` to avoid HTTP timeouts.

### Model router (single seam)

```go
// internal/llm/router.go
type Role int
const ( RolePlanner Role = iota; RoleResearch; RoleTriage; RoleSummarize; RoleCurate; RoleCompact )

type ModelSpec struct {
    Model   anthropic.Model
    Effort  string // "" when unsupported (Haiku)
    Adaptive bool  // false for Haiku
    MaxTokens int64
    Stream  bool
}

func (r *Router) Spec(role Role) ModelSpec { /* table-driven, config-overridable */ }
```

All agents call through `Router` + a thin `llm.Client` wrapper that applies retries, prompt caching, the correct thinking/effort per model, and structured-output decoding. **Models are config-overridable** so the operator can, e.g., promote Triage to Sonnet without code changes.

### Tool-use loop (Go, manual loop for control)

The Deep-Research agent uses a manual loop so we can log, gate, and budget each tool call:

```go
messages := []anthropic.MessageParam{ anthropic.NewUserMessage(...) }
for round := 0; round < cfg.MaxRounds; round++ {
    resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
        Model: spec.Model, MaxTokens: spec.MaxTokens,
        Thinking: adaptive(spec),                 // OfAdaptive for Opus; omit for Haiku
        OutputConfig: effortCfg(spec),            // effort:high for Opus
        Messages: messages, Tools: researchTools, // web_search, fetch_url
    })
    // handle err with typed-error classification + retry (see §15)
    messages = append(messages, resp.ToParam())
    if resp.StopReason != anthropic.StopReasonToolUse { break }
    results := executeTools(ctx, resp.Content)    // calls search router / fetcher; budget-guarded
    messages = append(messages, anthropic.NewUserMessage(results...))
}
```

---

## 9. History & Long-Term Profile (compacted)

Two layers satisfy FR4 ("compacted history → understand reading pattern & latent interest"):

1. **Raw recent history** — the last N weeks of `suggestions` (title, url, interest, scores, whether downloaded). Used for dedup (`seen_resources`) and as recent context for the Planner.
2. **Compacted long-term profile** — a single rolling, versioned summary (`profile.compacted_summary`) that captures:
   - demonstrated topic affinities and how they're drifting,
   - preferred depth/length/type per interest,
   - recurring sub-themes the user keeps getting (latent interests not explicitly configured).

After each run, the **Compactor agent** updates the profile: it takes `(current profile, this week's themes + suggestions)` and returns an updated summary, bounded in length so it never grows unbounded. Each update bumps `profile.version` and keeps the prior version for rollback/debug.

> This is deliberately a **summary, not embeddings**, to stay free-tier-friendly and dependency-light. A future enhancement (§19) adds embeddings for similarity-based dedup and "more like this."

`seen_resources` (URL hash + title + first-suggested run) prevents re-recommending the same material and feeds novelty scoring.

---

## 10. Email Delivery

- **Composition:** `html/template` (rich) + `text/template` (plaintext) rendered from the Curator's structured plan → multipart/alternative message. Templates live in `internal/email/templates/` and are unit-tested with golden files.
- **Content** (FR1): intro/themes → weekend schedule → per-item card (title, type, **why**, **how**, summary, **link**, **download** link/attachment).
- **Channels (abstracted):**
  ```go
  type Sender interface { Send(ctx, Message) error }
  ```
  - `SMTPSender` (`go-mail`) — default; works with Gmail app password or any SMTP relay.
  - `APISender` — pluggable transactional API (Resend/Brevo/Mailgun free tiers) — later/optional.
- **Attachments vs links:** PDFs may be attached if small (config `email.attach_max_mb`), else linked to the stored download or original source.
- **Idempotency & reliability:** send is the last externally-visible action; guarded by `runs.email_sent_at` so a resumed run never double-sends. Send failures retry with backoff; persistent failure marks the run `partial` and (optionally) sends an **admin alert** email on the next healthy attempt.
- **Secrets:** SMTP creds / API keys from env only.

---

## 11. Scheduling, Trigger Time & Catch-Up

- **Config-driven schedule** (FR2): `schedule.cron` + `schedule.timezone` (e.g. `"0 7 * * SAT"`, `"Asia/Kolkata"`). Changing the time is a config edit; **fsnotify** hot-reloads and re-registers the cron entry without restart.
- **Catch-up / missed runs (NFR1):** on startup and on each tick, the scheduler computes the **expected last fire time**; if no `runs` row exists for that window (machine was off, crash), it enqueues a **catch-up run**. This prevents losing a week because the box was asleep at 7am Saturday.
- **Single-flight:** a run lock (DB row / advisory lock) ensures only one pipeline runs at a time; a second trigger is coalesced.
- **Manual trigger:** `dailyread run-now [--dry-run] [--no-email]` for testing — `--dry-run` executes the full pipeline but skips send and history writes.

---

## 12. Configuration

Source of truth is `config.yaml` (user-editable, hand or via CLI), secrets via env. Validated on load; invalid config fails fast with actionable messages.

```yaml
# config.yaml
user:
  email: "bharatpatidar002@gmail.com"
  name: "Bharat"

schedule:
  cron: "0 7 * * SAT"      # Saturday 07:00
  timezone: "Asia/Kolkata"
  catch_up: true

weekly:
  max_items: 6             # total reading-list size budget
  primary_floor: 2         # min items for the primary interest

interests:
  - tag: "distributed-systems"
    primary: true
    intensity: high
    types: [case_study, article, pdf]
  - tag: "ai-evals"
    intensity: medium
  - tag: "engineering-leadership"
    intensity: light

search:
  priority: [tavily, brave, searxng, ddg]
  fanout: true
  monthly_caps: { tavily: 900, brave: 1800 }   # quota guard for free tiers
  searxng_base_url: ""                         # set to enable

models:                     # optional overrides; defaults from router table
  triage: "claude-haiku-4-5"
  research: "claude-opus-4-8"

budgets:
  research_max_rounds: 8
  research_max_tool_calls: 40
  per_run_token_cap: 1500000

email:
  channel: smtp            # smtp | api
  attach_max_mb: 8

paths:
  data_dir: ""             # default per-OS

# secrets are NEVER here — env only:
#   ANTHROPIC_API_KEY, TAVILY_API_KEY, BRAVE_API_KEY,
#   SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS
```

**CLI for config edits** (FR2/FR3 without hand-editing): `dailyread config set-schedule "0 8 * * SUN"`, `config add-interest --tag x --intensity high`, `config set-primary x`, `config show` — these mutate `config.yaml` safely (atomic write) and trigger reload.

---

## 13. Data Model (SQLite)

```sql
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
```

All writes that advance a stage happen in a transaction with the `runs.stage`/`status` update, so checkpoints are atomic.

---

## 14. Resilience & Fault Tolerance (NFR1)

| Mechanism | Detail |
|---|---|
| **Checkpoint + resume** | Each stage commits its output and advances `runs.stage`. On startup, any `running` run is resumed from its last completed stage (not restarted). |
| **Catch-up runs** | Missed windows (machine off) are detected and executed (§11). |
| **Idempotency** | Email send guarded by `email_sent_at`; history writes guarded by run/stage; downloads keyed by url+run. |
| **Retries w/ backoff+jitter** | All external calls (LLM, search, fetch, SMTP) retry transient failures (429/5xx/network) via `cenkalti/backoff`. The Anthropic SDK also auto-retries 408/409/429/5xx (`max_retries`). |
| **Circuit breakers** | Per search provider (`gobreaker`); a tripped provider is skipped and recovers via half-open probes. |
| **Timeouts + ctx cancellation** | Every network call has a context deadline; the whole run has an overall deadline. |
| **Budgets** | Hard caps: research rounds, tool calls, and a per-run **token cap** (abort + mark `partial` if exceeded) to bound cost. SDK **task budgets** (beta) optionally given to the research agent so it self-paces. |
| **Graceful degradation** | If search providers all fail → run on cached/prior candidates or send a "couldn't research this week" notice rather than crashing. If PDF download fails → link instead. If a sub-agent fails after retries → degrade that item, keep the rest. |
| **Single-flight lock** | Prevents concurrent duplicate runs. |
| **Backups** | The SQLite file + downloads dir are the entire state; a `dailyread backup` command copies them (online backup via SQLite `VACUUM INTO`). |

**Run state machine:**
```
pending → running → (succeeded | partial | failed)
            ↑__________ resume from runs.stage on restart
```

---

## 15. Error Handling Strategy (NFR3)

- **Typed errors & wrapping:** sentinel errors (`ErrAllProvidersDown`, `ErrBudgetExceeded`, `ErrLLMRefusal`) + `fmt.Errorf("...: %w", err)` everywhere; `errors.Is/As` at decision points.
- **LLM errors:** classify via the SDK's `*anthropic.Error` (`errors.As`) and branch on `StatusCode` — 429/5xx retry with backoff; 400 (e.g., bad params) fail fast and log the request id; **`StopReason == "refusal"`** handled explicitly (don't read `content[0]` blindly) — log category from `stop_details`, skip the item, continue.
- **Search errors:** per-provider failures are expected and non-fatal (router moves on); only a total wipeout degrades the run.
- **Structured output:** Planner/Triage/Curator use json_schema/strict tools; on schema mismatch the SDK forces a model retry. A final parse guard still validates before use.
- **Per-stage isolation:** a stage failure marks the run `partial`/`failed` with the stage + error persisted; the next startup or run can resume or retry. One bad item never aborts the whole digest.
- **Observability of errors:** every error is logged with `run_id`, `stage`, `provider/agent`, and the Anthropic `request_id` when present (for support).
- **Admin alerting:** on `failed`/`partial`, optionally email the operator a concise failure summary (best-effort, never blocks).

---

## 16. Observability

- **Structured logging** (`slog`, JSON) with consistent fields: `run_id`, `stage`, `agent`, `provider`, `tokens_in/out`, `latency_ms`, `request_id`.
- **Per-run metrics** persisted on `runs`: tokens in/out (→ cost estimate using §8 pricing), stage timings, provider usage, item counts.
- **`dailyread status`** CLI prints recent runs, their status/stage, token spend, and last email time.
- (Future) optional Prometheus endpoint for run counts, durations, provider error rates.

---

## 17. Security & Secrets

- **Secrets only via env / `.env`** (gitignored): `ANTHROPIC_API_KEY`, `TAVILY_API_KEY`, `BRAVE_API_KEY`, SMTP creds. Never in `config.yaml`, never logged.
- Even though single-user/no-auth, the API keys are real money/risk — the data dir and `.env` are documented as sensitive.
- Outbound fetch hardening: cap fetch size, restrict to http/https, timeout, and don't execute/parse anything beyond text/PDF.
- SSRF note: `fetch_url` only follows model-provided URLs that originated from search results; we validate scheme and (optionally) disallow private IP ranges.

---

## 18. Project Structure

```
dailyread/
  cmd/dailyread/
    main.go                 # wires config, store, scheduler, daemon + cobra root
    cmd_run.go              # run (daemon), run-now, status, config, migrate, backup, test-email
  internal/
    config/                 # load, validate, hot-reload (fsnotify), CLI mutations
    scheduler/              # cron registration, catch-up, single-flight
    pipeline/               # orchestrator + the 8 stages + checkpointing
    agents/
      planner/ research/ triage/ summarize/ curate/ compact/
    llm/                    # Anthropic client wrapper + model Router + retries + caching
    search/                 # Searcher interface + router + breakers + quota
      tavily/ brave/ ddg/ searxng/
    fetch/                  # http fetcher, readability extraction, PDF download/text
    rank/                   # scoring, dedup, novelty, intensity allocation
    email/                  # composer (templates) + Sender (smtp/api) + golden tests
    store/                  # SQLite repositories + migrations (embedded)
    history/                # profile compaction logic
    resilience/             # retry, backoff, breaker, budgets, timeouts helpers
    domain/                 # core types (Interest, Candidate, Suggestion, Run, ...)
    observability/          # slog setup, run metrics
  migrations/               # *.sql (embedded via embed.FS)
  configs/config.example.yaml
  testdata/                 # fixtures: fake search results, sample HTML/PDF, golden emails
  go.mod
  README.md
  IMPLEMENTATION_PLAN.md
```

---

## 19. Testing Strategy

- **Unit tests** for each package with **fakes**: `fakeSearcher`, `fakeLLM` (canned tool-use + structured responses), `fakeSender`. No network in unit tests.
- **Router/breaker tests:** force provider failures, assert fallback order, breaker trip/recover, quota guard.
- **Pipeline resume tests:** kill the pipeline at each stage; assert resume continues correctly and never double-sends email / double-writes history.
- **Email golden tests:** render templates from a fixed plan → compare to `testdata/golden/*.html|.txt`.
- **Allocation tests:** intensity/primary → per-interest budgets across edge cases.
- **Contract tests (opt-in, tagged):** real calls to each search provider + a tiny real Anthropic call, run manually / in a gated CI job with secrets — kept out of the default `go test ./...`.
- **`run-now --dry-run`** as an end-to-end smoke test against fakes.

---

## 20. Phased Delivery Roadmap

Each phase is independently runnable/testable.

| Phase | Deliverable | Exit criteria |
|---|---|---|
| **0 — Scaffold** | Go module, config load+validate, SQLite + migrations, slog, cobra skeleton, `status`/`migrate` | `dailyread migrate` creates DB; `config show` validates sample |
| **1 — Search** | `Searcher` interface + Tavily + DDG (keyless) + router + breakers + quota | `dailyread search "<q>"` returns merged results; provider failure falls back |
| **2 — LLM core** | `llm.Client` + model `Router` (correct thinking/effort per model) + retries + prompt caching + structured-output decode | one structured call per role works against a tiny live prompt |
| **3 — Research loop** | Deep-Research agent with `web_search`/`fetch_url` tools + budgets + fetch/readability | `run-now --dry-run` produces a candidate set within budget |
| **4 — Rank + Curate** | Triage scoring, intensity allocation, Curator structured plan | dry-run yields an ordered reading plan JSON |
| **5 — Email** | Templates (HTML+text) + SMTP sender + `test-email` + golden tests | `test-email` delivers a real digest |
| **6 — Schedule + resilience** | Cron + catch-up + single-flight + checkpoint/resume + idempotent send | kill-and-resume test passes; missed-window catch-up fires |
| **7 — History** | `suggestions`/`seen_resources` writes + Compactor profile updates + novelty in scoring | week N avoids week N−1 dupes; profile evolves across runs |
| **8 — Hardening** | admin alerts, backup command, metrics on `runs`, docs, contract tests | full week runs unattended end-to-end |

A **walking skeleton** (config → one search → one Haiku call → console output) lands at the end of Phase 2 so the riskiest integrations (search + LLM) are proven early.

---

## 21. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Free search tiers throttle / change | ≥2 providers + keyless DDG fallback + SearXNG self-host option + quota guard; provider is one file behind an interface |
| LLM cost runs away in the agentic loop | hard round/tool-call caps + per-run token cap + task budgets + smallest-viable model per task |
| Low-quality / paywalled sources | readability extraction + triage relevance/novelty scoring; prefer providers that return content (Tavily) |
| Machine offline at trigger time | catch-up runs on startup |
| Partial failure mid-run | checkpoint/resume + per-stage isolation + idempotent send |
| Refusals / safety stops on niche topics | explicit `refusal` handling; skip item, continue; (optional) Opus fallback for borderline |
| Profile/summary drift or bloat | bounded, versioned compaction with rollback |
| Windows/CGO build pain | pure-Go SQLite (`modernc.org/sqlite`), no CGO anywhere |

---

## 22. Future Enhancements (explicitly out of scope now)
- Embeddings for similarity dedup, clustering, and "more like this."
- Read-only status web dashboard / served history.
- Feedback loop: user marks items read/liked → reinforce profile.
- Multi-user (the `user_id` seam already exists).
- Paid providers as opt-in higher-quality search.
- ICS calendar attachment for the weekend reading schedule.

---

## Appendix A — Environment Variables

```
ANTHROPIC_API_KEY=...     # required
TAVILY_API_KEY=...        # optional (provider skipped if absent)
BRAVE_API_KEY=...         # optional
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=...
SMTP_PASS=...             # app password
DAILYREAD_DATA_DIR=...    # optional override
```

## Appendix B — Go Dependencies (initial)

```
github.com/anthropics/anthropic-sdk-go
modernc.org/sqlite
github.com/robfig/cron/v3
github.com/spf13/cobra
gopkg.in/yaml.v3
github.com/fsnotify/fsnotify
github.com/go-shiori/go-readability
github.com/ledongthuc/pdf
github.com/cenkalti/backoff/v4
github.com/sony/gobreaker
github.com/wneessen/go-mail
github.com/pressly/goose/v3        // migrations (or embedded SQL)
```

## Appendix C — Example Email (shape)

```
Subject: Your weekend reading — 3 themes, 6 picks (Sat 22 Jun)

Hi Bharat,

This week leans into your primary interest (distributed systems) with two
deep case studies, plus lighter picks in AI evals and eng leadership. I noticed
you keep gravitating toward consensus/replication write-ups, so I prioritized a
real-world Raft post-mortem.

Suggested schedule
  Sat AM (deep):  1. <title>  — ~45 min
  Sat PM (deep):  2. <title>  — ~40 min
  Sun  (light):   3–6 …

1. <Title>   [case_study]  · source · [Open] [Download PDF]
   Why: ties to your distributed-systems focus and last week's interest in failure modes.
   How: deep read; focus on the section on quorum loss; ~45 min.
   Summary: <2–3 lines>

... (items 2–6) ...

— DailyRead
```

## Appendix D — LLM Call Conventions (enforced by `internal/llm`)

- Opus 4.8 / Sonnet 4.6: `Thinking = adaptive`, `OutputConfig.Effort = "high"` (research/curate).
- Haiku 4.5: **no** `effort`, **no** adaptive thinking (would 400) — thinking unset/disabled.
- Large outputs (Curator): **stream** + `Accumulate`.
- Structured agents: `OutputConfig.Format` (json_schema) or `strict: true` tools; validate after decode.
- Stable system prompt + tools first, `CacheControl: ephemeral` on the last system block.
- Wrap every call: classify `*anthropic.Error` via `errors.As`, retry transient, handle `StopReason == refusal`, record `tokens_in/out` + `request_id`.
```
```
