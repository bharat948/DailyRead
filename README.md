# DailyRead

DailyRead is a Go service that performs deep web research and curates a personalized weekly reading list.

## Development Phases

### Phase 0: Scaffold
**Dependencies & APIs:**
- `github.com/spf13/cobra` - CLI framework
- `modernc.org/sqlite` - Pure Go SQLite driver
- `github.com/pressly/goose/v3` - Database migrations
- `gopkg.in/yaml.v3` - Configuration file parsing
- `github.com/fsnotify/fsnotify` - Live config reloading

### Phase 1: Search
**Dependencies & APIs:**
- `github.com/sony/gobreaker` - Circuit breaker for search APIs
- `github.com/cenkalti/backoff/v4` - Exponential backoff for network retries
- **APIs Used:**
  - **Tavily API**: Used for AI-optimized web search (returns clean article content). Requires `TAVILY_API_KEY`.
  - **DuckDuckGo HTML/Instant Answer API**: Keyless fallback.
