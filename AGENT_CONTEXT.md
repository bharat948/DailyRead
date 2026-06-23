# Agent Context & Project Status

This file provides context for any coding agents working on the `DailyRead` project in future developments. We are keeping it minimal for now and will improve it as development progresses.

## Project Overview
DailyRead is a Go service designed to run on a weekly schedule. It performs deep web research and curates a personalized reading list using small multi-agent LLM workflows. 

## Source of Truth
The master architectural blueprint and roadmap is maintained in `IMPLEMENTATION_PLAN.md`. **Always refer to it before making design decisions.**

## Current Status
As of the current implementation:
- **Phase 0 (Scaffold)** is **COMPLETE**. We have the base Go module, configuration loading/validation, SQLite with embedded migrations, structured logging (slog), and the base Cobra CLI.
- **Phase 1 (Search)** is **COMPLETE**. The `Searcher` interface, multi-provider router with circuit breakers and quotas, and adapters for Tavily and DuckDuckGo are implemented.
- **Phase 2 (LLM Core)** is **PENDING**. This is the immediate next step.

## Guidelines for Coding Agents
1. **No CGO:** The project relies on `modernc.org/sqlite` to avoid CGO dependencies. Ensure all new dependencies are pure Go.
2. **Architecture:** Respect the single-responsibility principles outlined in the implementation plan. New phases should be contained within their respective `internal/` packages (e.g., `internal/llm`, `internal/agents`).
3. **Dependencies:** Rely on standard library packages where possible, and only use dependencies specified in the implementation plan (e.g., Anthropic SDK, `cenkalti/backoff/v4`, `sony/gobreaker`).
4. **Update Progress:** As phases are completed, update this document and the `README.md` to reflect the latest state.
