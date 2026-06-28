# DailyRead

> **Your personal AI news anchor.** At a scheduled time, DailyRead researches what matters to you, writes your briefing, and delivers it — remembering everything it told you before.

---

## What it does

DailyRead is a Go daemon that runs a multi-agent research pipeline on a schedule. For each of your configured interests it:

1. **Researches** — a Deep-Research agent searches the web, reads full articles, and distills them into a growing **global research corpus** (no URL is ever fetched twice)
2. **Filters** — a novelty filter drops anything it has already delivered to you (cross-run dedup via a per-user `user_seen` index)
3. **Triages** — a Triage agent (Haiku-class) scores and selects the best candidates by relevance, depth, and diversity
4. **Curates** — a Curator agent (Opus-class) frames each item with a personalized **Why it matters**, **How to read it**, and **When to read it** — tuned to your long-term profile
5. **Delivers** — renders a rich HTML + plaintext briefing email and sends it
6. **Compacts** — a Compactor agent (Haiku-class) folds the run's themes into your evolving **user profile** so the anchor remembers and improves

---

## Architecture

```
schedule / HTTP API / CLI
        │
        ▼
  pipeline.Service
  ├── Stage: research  (Researcher agent + global memory corpus)
  ├── Stage: triage    (Triage agent — Haiku)
  ├── Stage: curate    (Curator agent — Opus)
  ├── Stage: persist   (digest_items + user_seen → SQLite)
  ├── Stage: deliver   (HTML+text email via SMTP)
  └── Stage: compact   (Compactor agent — Haiku → user_profile)

Two memory layers (SQLite, pure-Go, no CGO):
  Global  → resources, topic_resources, search_cache
  User    → runs, digest_items, user_seen, user_profile
```

**Provider-agnostic:** swap between Anthropic (Claude) and OpenAI by setting `models_provider` in your config or the user's settings. The LLM router assigns the right model tier per role automatically.

---

## Quick start

### Prerequisites

- Go 1.22+
- An Anthropic **or** OpenAI API key
- (Optional) Tavily API key for higher-quality search

### Build

```bash
git clone https://github.com/bharat948/DailyRead.git
cd DailyRead
go build -o dailyread.exe ./cmd/dailyread
```

### Configure

Create a `.env` file in the project root:

```env
# LLM — choose one
ANTHROPIC_API_KEY=sk-ant-...
# OPENAI_API_KEY=sk-...

# Search (optional but recommended)
TAVILY_API_KEY=tvly-...

# Email delivery (optional)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=you@gmail.com
SMTP_PASS=your-app-password      # Gmail: generate an App Password

# Default user shortcut for CLI
DEFAULT_USER_EMAIL=you@gmail.com
```

### Run

```bash
# Start the server (web dashboard + JSON API + background scheduler)
./dailyread.exe start --port 8080

# Register at http://localhost:8080/register
# Log in, add interests, set a schedule — or use the API below
```

---

## JSON API

The server exposes a REST API at `/api/`. No auth required (single-user local deployment).
User is resolved automatically when there is exactly one account, or pass `?user=<id>` / `X-User-ID` header.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/healthz` | Liveness check |
| `POST` | `/api/runs` | Trigger a pipeline run (async) |
| `GET` | `/api/runs` | List runs (`?limit=N`) |
| `GET` | `/api/runs/{id}` | Run detail + delivered items |
| `GET` | `/api/digests` | Recently delivered items — past-digest memory |
| `GET` | `/api/profile` | User's compacted long-term profile |
| `GET` | `/api/topics/{topic}` | Global research corpus for a topic |
| `GET` | `/api/interests` | List interests |
| `POST` | `/api/interests` | Add interest `{"tag","intensity","types","primary"}` |
| `DELETE` | `/api/interests/{id}` | Remove interest |

### Example flow

```bash
# Trigger a run (returns immediately; pipeline runs in background)
curl -X POST http://localhost:8080/api/runs
# → {"id":"<run-id>","status":"running",...}

# Poll until finished
curl http://localhost:8080/api/runs/<run-id>
# → {"run":{...,"status":"succeeded"},"items":[...]}

# Read your delivered digest with Why/How/Slot framing
curl http://localhost:8080/api/digests

# Inspect the global research corpus for a topic
curl http://localhost:8080/api/topics/distributed-systems

# See your evolving profile (grows after every run)
curl http://localhost:8080/api/profile

# Run manually from CLI
./dailyread.exe run-now
```

---

## Project structure

```
cmd/dailyread/         CLI (cobra) — start, run-now, migrate
internal/
  agents/
    research/          Deep-Research agent — agentic web search + fetch loop
    triage/            Triage agent — score and select candidates
    curator/           Curator agent — personalized why/how/slot framing
    compact/           Compactor agent — fold run into user profile
  db/                  SQLite init, migrations, repository (global + user memory)
  domain/              Core types (Candidate, Resource, Run, DigestItem, UserProfile…)
  delivery/            HTML + plaintext email rendering + SMTP sender
  fetch/               HTTP fetcher with go-readability extraction
  llm/                 Provider-agnostic client (Anthropic + OpenAI) + model router
  pipeline/            Pipeline orchestrator — all stages wired together
  schedule/            Per-user cron scheduler (robfig/cron)
  search/              Search router + circuit breakers + Tavily / SerpAPI / DDG adapters
  web/                 HTTP server — dashboard (HTML) + JSON API
```

---

## Technology

| Concern | Choice |
|---------|--------|
| Language | Go 1.22+ (single static binary, no CGO) |
| Database | SQLite via `modernc.org/sqlite` (pure Go, no CGO) |
| LLM | `anthropic-sdk-go` + `openai-go`, provider-agnostic interface |
| Search | Tavily / SerpAPI / DuckDuckGo with circuit breakers (`sony/gobreaker`) |
| Scheduler | `robfig/cron/v3` |
| HTTP | stdlib `net/http` |
| Article extraction | `go-shiori/go-readability` |
| Retry/backoff | `cenkalti/backoff/v4` |
| CLI | `spf13/cobra` |

---

## Roadmap

- [ ] Text-to-speech (OpenAI TTS) — spoken audio briefings
- [ ] Audio playback scheduling — the anchor "goes on air" at your chosen time
- [ ] Scheduled-run catch-up (missed windows)
- [ ] Curator follow-up awareness (reference prior weeks in framing)
- [ ] Private podcast RSS feed for mobile listening
- [ ] Feedback loop (mark items read/liked → reinforce profile)
