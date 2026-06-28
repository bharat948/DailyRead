// Package compact provides the Compactor agent, which folds a completed run's
// digest items into the user's long-term profile summary. This is what makes the
// anchor's memory compound: the profile evolves after every run.
package compact

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"dailyread/internal/domain"
	"dailyread/internal/llm"
)

// Agent holds the LLM client wired for the compact role (Haiku-class: cheap, summary task).
type Agent struct {
	client llm.Client
	model  string
}

func New(client llm.Client, model string) *Agent {
	return &Agent{client: client, model: model}
}

// Run takes the current compacted profile and the items just delivered in this run,
// and returns an updated profile summary. The summary is bounded to stay concise
// (the prompt enforces a max length so it never grows unbounded).
func (a *Agent) Run(ctx context.Context, currentProfile string, items []domain.DigestItem) (string, error) {
	if len(items) == 0 {
		slog.Info("compactor: no items to fold, skipping")
		return currentProfile, nil
	}

	slog.Info("compactor running", "model", a.model, "items", len(items))

	// Summarise this run's delivered items for the prompt.
	var runLines []string
	for _, it := range items {
		line := fmt.Sprintf("- [%s] %s", it.InterestTag, it.Title)
		if it.Why != "" {
			line += " — " + it.Why
		}
		runLines = append(runLines, line)
	}
	runSummary := strings.Join(runLines, "\n")

	priorProfile := currentProfile
	if priorProfile == "" {
		priorProfile = "(no prior profile — this is the first run)"
	}

	systemPrompt := `You are maintaining a long-term reading profile for one person.
Your job is to fold this run's delivered items into their existing profile,
then return a compact, updated summary.

Rules:
- Keep the profile SHORT: 150–250 words max.
- Preserve important older patterns; don't wipe them on every update.
- Note any topic drift (e.g. user is moving from X toward Y).
- Note any latent themes that keep appearing across runs.
- Note depth/format preferences revealed by the selected items.
- Write in third person, present tense ("The user favours…").
- Never mention specific article titles; capture themes and patterns only.
- Return ONLY the updated profile text — no preamble, no commentary.`

	userMsg := fmt.Sprintf("Current profile:\n%s\n\nThis run's delivered items:\n%s\n\nWrite the updated profile:", priorProfile, runSummary)

	resp, err := a.client.Chat(ctx, llm.ChatRequest{
		Model:     a.model,
		System:    systemPrompt,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: userMsg}},
		MaxTokens: 400,
	})
	if err != nil {
		return "", fmt.Errorf("compactor LLM call: %w", err)
	}

	updated := strings.TrimSpace(resp.Message.Content)
	if updated == "" {
		return currentProfile, fmt.Errorf("compactor returned empty profile")
	}

	slog.Info("compactor complete", "profile_words", len(strings.Fields(updated)))
	return updated, nil
}
