package research

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"dailyread/internal/domain"
	"dailyread/internal/fetch"
	"dailyread/internal/llm"
)

type Researcher struct {
	llmClient llm.Client
	searcher  SearchProvider
	fetcher   fetch.Fetcher
	mem       Memory
	model     string
	maxRounds int
}

// New builds a Researcher. mem is the global research memory (corpus + caches);
// pass nil to run without persistence (e.g. in isolated tests).
func New(client llm.Client, searcher SearchProvider, fetcher fetch.Fetcher, mem Memory, model string, maxRounds int) *Researcher {
	return &Researcher{
		llmClient: client,
		searcher:  searcher,
		fetcher:   fetcher,
		mem:       mem,
		model:     model,
		maxRounds: maxRounds,
	}
}

// Run executes the agentic loop to find articles matching the given interest.
func (r *Researcher) Run(ctx context.Context, interest domain.Interest) ([]domain.Candidate, error) {
	slog.Info("Starting research loop", "interest", interest.Tag)

	systemPrompt := fmt.Sprintf(`You are an expert technical researcher. Your goal is to find high-quality, deeply technical, and novel articles, blog posts, or case studies about "%s".
The user has specified these preferred formats: %v.
The intensity of this interest is "%s" (higher intensity means they want more in-depth/advanced material).

You have two tools:
1. web_search: Search the internet for recent and highly relevant articles.
2. fetch_url: Fetch the full text of an article to verify its quality, depth, and relevance.

Instructions:
1. Formulate a highly specific search query to find deep technical content.
2. Search the web.
3. Review the snippets. If a snippet looks extremely promising, use fetch_url to read its content.
4. If it's a good fit, add it to your internal list of candidates.
5. If you haven't found enough good candidates (aim for 2-3 excellent ones), search again with different keywords.
6. Once you are satisfied with your candidates, return your final answer as a JSON array of objects.

Your final output MUST be a valid JSON array where each object has the following keys:
- "interest_tag": "%s"
- "url": the article URL
- "title": the article title
- "summary": a 2-3 sentence summary
- "relevance": an integer 1-10 scoring how well it matches the interest
- "word_count": approximate word count (guess based on fetched length)
- "why": a short explanation of why this is a high-quality pick

Do not output any markdown formatting around the final array (no json tags), just output the raw JSON array. You have a strict limit of %d turns, so stop and output the array once you have found 2-3 great candidates.`, interest.Tag, interest.Types, interest.Intensity, interest.Tag, r.maxRounds)

	tools := []llm.Tool{
		{
			Name:        "web_search",
			Description: "Search the web for articles",
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string"},
				},
				"required": []string{"query"},
				"additionalProperties": false,
			},
		},
		{
			Name:        "fetch_url",
			Description: "Fetch the text content of a webpage by URL to read the article",
			Schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{"type": "string"},
				},
				"required": []string{"url"},
				"additionalProperties": false,
			},
		},
	}

	beginMsg := "Begin your research now. Remember to output ONLY the JSON array when you are finished." + r.warmStartContext(interest.Tag)

	req := llm.LoopRequest{
		Model:  r.model,
		System: systemPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: beginMsg},
		},
		MaxTokens: 8000,
	}

	executor := func(ctx context.Context, name, args string) (string, error) {
		switch name {
		case "web_search":
			var parsed struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal([]byte(args), &parsed); err != nil {
				return "", err
			}
			// Routes through global research memory: cache hit, or live search +
			// distill into the corpus.
			return r.memSearch(ctx, interest.Tag, parsed.Query)

		case "fetch_url":
			var parsed struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal([]byte(args), &parsed); err != nil {
				return "", err
			}
			// Corpus hit, or live fetch + distilled write-back.
			return r.memFetch(ctx, parsed.URL)
		}
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	finalJSON, err := r.llmClient.ResearchLoop(ctx, req, tools, r.maxRounds, executor)
	if err != nil {
		return nil, fmt.Errorf("research loop failed: %w", err)
	}

	if finalJSON == "" {
		return nil, fmt.Errorf("research loop hit max rounds (%d) without returning a final answer", r.maxRounds)
	}

	// Clean up any accidental markdown blocks
	finalJSON = strings.TrimPrefix(finalJSON, "```json\n")
	finalJSON = strings.TrimPrefix(finalJSON, "```\n")
	finalJSON = strings.TrimSuffix(finalJSON, "\n```")

	var candidates []domain.Candidate
	if err := json.Unmarshal([]byte(finalJSON), &candidates); err != nil {
		slog.Error("Failed to parse agent JSON output", "output", finalJSON, "error", err)
		return nil, fmt.Errorf("failed to parse JSON from agent: %w", err)
	}

	// Promote the final picks into the global corpus so future runs (and other
	// users) start warm on this topic.
	r.recordCandidates(interest.Tag, candidates)

	return candidates, nil
}
