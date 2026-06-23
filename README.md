# DailyRead

## What is the system about
DailyRead is a pure-Go background daemon that performs deep web research on a weekly cadence to curate a personalized reading list (case studies, PDFs, articles) for its users. It learns from a compacted history of past suggestions to track the user's evolving interests and emails a "what / how / why to read" digest containing direct links and downloads.

Designed for reliability and minimal dependencies, the system uses a multi-agent LLM architecture where small, specialized agents (like Claude Haiku or GPT-4o-mini) handle routing and triage, while a stronger agent (Claude Opus or o3-mini) handles the deep research loop.

## Architecture & Design Decisions

### Multi-Tenant SaaS Transformation
Initially built as a single-user CLI application configured via `config.yaml`, the system has been architected into a Multi-Tenant SaaS platform:
- **Storage:** We transitioned from YAML files to an embedded **SQLite** database (`modernc.org/sqlite` which is CGO-free, ensuring easy deployment). SQLite is sufficient for the early stages of a SaaS and keeps operational complexity low. Future scale out can easily adapt to Postgres.
- **Authentication:** We use `gorilla/sessions` with `HttpOnly`, `Secure` cookies to manage user logins. Passwords are hashed via `bcrypt`. 
- **Bring Your Own Credentials (BYOC):** Because AI and Search APIs can be expensive, the architecture shifts API costs to the user. Users can input their own OpenAI/Anthropic keys and SMTP credentials via the web dashboard.
- **Dynamic Scheduling:** The background scheduler dynamically manages individual `cron` jobs for each user's unique timezone and schedule preference.
- **Web UI:** A sleek, minimal web interface provides users with control over their scheduling, interests, and API credentials.

### Pipeline execution
1. **Load:** Reads user's interests, intensity preferences, and API credentials from the DB.
2. **Plan:** A Query Planner agent generates search queries.
3. **Research:** A Deep-Research agent runs a tool-use loop (search -> fetch -> read -> re-search) using providers like Tavily and DuckDuckGo to accumulate candidate articles.
4. **Triage:** Candidates are scored on relevance, novelty, and intensity fit.
5. **Curate:** A Curator agent produces the structured reading plan.
6. **Deliver:** The digest is rendered into an HTML email and sent via the user's SMTP account.

## How to setup

### Prerequisites
- **Go 1.26+** (No CGO required)

### Installation

1. **Clone and build:**
   ```bash
   git clone <your-repo>/DailyRead
   cd DailyRead
   go build -o dailyread.exe ./cmd/dailyread
   ```

2. **Start the Platform:**
   Start the DailyRead web server and background daemon. This command will automatically initialize the database in the `data/` directory and start listening for HTTP connections.
   ```bash
   ./dailyread.exe start --port 8080
   ```

3. **Access Dashboard & Register:**
   Navigate to `http://localhost:8080/register` in your web browser. Create an account, and log in.

4. **Bring Your Own Credentials (BYOC):**
   In the Settings dashboard, you will need to provide:
   - **OpenAI API Key** (or Anthropic API Key based on model provider)
   - **SMTP Credentials** (e.g. your Gmail address and an App Password) to allow the system to send emails on your behalf.
   - **Tavily API Key** (optional, fallback to free search engines if omitted)

5. **Run the Daemon:**
   Once configured, the background daemon will automatically trigger your personal research pipeline based on your cron schedule. You can also manually trigger a run via the Web UI "Trigger Run Now" button.

*(You can also force a run via CLI for a specific user ID for testing purposes by running `./dailyread.exe run-now --user <uuid>`)*
