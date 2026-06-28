// Package curator provides the Curator agent, which takes a triaged candidate list and
// enriches each item with a personalized "why it matters", "how to read it", and
// "when to read it" framing — tuned to the user's long-term profile and interests.
package curator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"dailyread/internal/domain"
	"dailyread/internal/llm"
)

// Agent holds the LLM client wired for the curate role.
type Agent struct {
	client llm.Client
	model  string
}

func New(client llm.Client, model string) *Agent {
	return &Agent{client: client, model: model}
}

// curatedItem is the structured schema the LLM must return per candidate.
type curatedItem struct {
	URL  string `json:"url"`
	Why  string `json:"why"`  // 1–2 sentences, personalized to the user's profile/interests
	How  string `json:"how"`  // e.g. "Deep read — focus on the replication section (~30 min)"
	Slot string `json:"slot"` // morning | evening | weekend
}

type curatedResponse struct {
	Items []curatedItem `json:"items"`
}

// Run enriches each candidate with personalized Why/How/Slot framing drawn from the
// user's profile and their configured interests. Returns the same slice with those
// fields filled in; candidates whose URL isn't matched keep their original values.
func (a *Agent) Run(ctx context.Context, profile string, interests []domain.UserInterest, candidates []domain.Candidate) ([]domain.Candidate, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}

	slog.Info("curator running", "model", a.model, "candidates", len(candidates))

	// Serialize the candidate list for the prompt.
	type candidateView struct {
		URL         string `json:"url"`
		Title       string `json:"title"`
		Summary     string `json:"summary"`
		InterestTag string `json:"interest_tag"`
		Relevance   int    `json:"relevance"`
		WordCount   int    `json:"word_count"`
	}
	views := make([]candidateView, len(candidates))
	for i, c := range candidates {
		views[i] = candidateView{
			URL: c.URL, Title: c.Title, Summary: c.Summary,
			InterestTag: c.InterestTag, Relevance: c.Relevance, WordCount: c.WordCount,
		}
	}
	candidatesJSON, _ := json.MarshalIndent(views, "", "  ")

	// Build interest summary for prompt context.
	var interestLines []string
	for _, i := range interests {
		prim := ""
		if i.IsPrimary {
			prim = " (primary)"
		}
		interestLines = append(interestLines, fmt.Sprintf("- %s [intensity: %s]%s", i.Tag, i.Intensity, prim))
	}
	interestCtx := strings.Join(interestLines, "\n")

	profileCtx := profile
	if profileCtx == "" {
		profileCtx = "No prior reading history yet — this is the user's first briefing."
	}

	systemPrompt := fmt.Sprintf(`You are the personal AI news anchor for one reader.
Your job is to write the "why, how, and when" for each article in their briefing —
in the voice of an anchor who knows them well.

User's long-term profile:
%s

User's configured interests:
%s

Rules:
- "why": 1–2 sentences. Personal and specific — tie the article to their profile/interests.
  Reference prior knowledge if the profile mentions it ("following the Raft discussion last week…").
  Never generic ("this is interesting"). Max 60 words.
- "how": One sentence on HOW to read it — depth (skim vs deep), time budget, where to focus.
  Example: "Deep read — go straight to the failure analysis section (~25 min)."
- "slot": One of exactly: morning | evening | weekend
  Heavy/dense material → weekend. Quick updates → morning or evening.
- Match each item by its URL — you must return exactly one entry per candidate.`, profileCtx, interestCtx)

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"items": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url":  map[string]interface{}{"type": "string"},
						"why":  map[string]interface{}{"type": "string"},
						"how":  map[string]interface{}{"type": "string"},
						"slot": map[string]interface{}{"type": "string", "enum": []string{"morning", "evening", "weekend"}},
					},
					"required":             []string{"url", "why", "how", "slot"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"items"},
		"additionalProperties": false,
	}

	req := llm.StructuredRequest{
		Model:  a.model,
		System: systemPrompt,
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: "Here are the articles for this briefing:\n\n" + string(candidatesJSON),
		}},
		MaxTokens: 2000,
		Schema:    schema,
	}

	var resp curatedResponse
	if err := a.client.Structured(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("curator LLM call: %w", err)
	}

	// Index the enrichments by URL for O(1) merge.
	enriched := make(map[string]curatedItem, len(resp.Items))
	for _, item := range resp.Items {
		enriched[item.URL] = item
	}

	// Merge enrichments back into the candidate slice, preserving order.
	out := make([]domain.Candidate, len(candidates))
	for i, c := range candidates {
		out[i] = c
		if e, ok := enriched[c.URL]; ok {
			out[i].Why = e.Why
			out[i].How = e.How
			out[i].Slot = e.Slot
		}
	}

	slog.Info("curator complete", "enriched", len(resp.Items))
	return out, nil
}
