# DailyRead

## What is the system about
DailyRead is a pure-Go background daemon that performs deep web research on a weekly cadence to curate a personalized reading list (case studies, PDFs, articles) for a single user. It learns from a compacted history of past suggestions to track the user's evolving interests and emails a "what / how / why to read" digest containing direct links and downloads.

Designed for reliability and minimal dependencies, the system uses a multi-agent LLM architecture where small, specialized agents (like Claude Haiku) handle routing and triage, while a stronger agent (Claude Opus) handles the deep research loop.

## How it works
DailyRead operates as a checkpointed, resumable pipeline that fires on a schedule (e.g., every Saturday morning). The pipeline consists of the following core stages:

1. **Load:** Reads your interests, intensity preferences, and compacted reading history.
2. **Plan:** A Query Planner agent distributes your weekly budget across interests and generates search queries.
3. **Research:** A Deep-Research agent runs a tool-use loop (search -> fetch -> read -> re-search) using providers like Tavily and DuckDuckGo to accumulate candidate articles.
4. **Triage:** Candidates are scored on relevance, novelty, and intensity fit.
5. **Curate:** A Curator agent produces the structured reading plan, explaining *why* an item matters to you and *how* to read it.
6. **Download:** PDFs and case studies are downloaded to your local data directory.
7. **Deliver:** The digest is rendered into an HTML/text email and sent via SMTP.
8. **Learn:** The system compacts the week's themes into a rolling, long-term profile to improve future recommendations.

Every stage writes its output to an embedded SQLite database. If the daemon crashes mid-run, it resumes precisely from the last completed stage.

## How to setup

### Prerequisites
- **Go 1.26+** (No CGO required)
- An **Anthropic API Key** (for the LLM agents)
- An optional **Tavily API Key** (highly recommended for optimized web search)
- An **SMTP account** (like Gmail with an App Password) to send the email digest

### Installation

1. **Clone and build:**
   ```bash
   git clone <your-repo>/DailyRead
   cd DailyRead
   go build -o dailyread.exe ./cmd/dailyread
   ```

2. **Database Initialization:**
   Initialize the SQLite database.
   ```bash
   ./dailyread.exe migrate
   ```

3. **Configuration:**
   Create and populate your configuration file (see `configs/config.example.yaml` for a reference):
   ```bash
   cp configs/config.example.yaml configs/config.yaml
   ```
   *Edit `configs/config.yaml` to configure your interests, intensity, and schedule.*

4. **Set Environment Variables:**
   Create a `.env` file in the root directory (or set these in your environment) to hold your secrets:
   ```env
   ANTHROPIC_API_KEY=your_anthropic_key
   TAVILY_API_KEY=your_tavily_key
   SMTP_HOST=smtp.gmail.com
   SMTP_PORT=587
   SMTP_USER=your_email@gmail.com
   SMTP_PASS=your_app_password
   ```

5. **Run the Daemon:**
   Start the dailyread daemon. It will wait in the background and execute the pipeline at your configured cron schedule.
   ```bash
   ./dailyread.exe run
   ```

*(You can also force a run immediately for testing purposes by running `./dailyread.exe run-now`)*
