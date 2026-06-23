package triage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"dailyread/internal/domain"
	"dailyread/internal/llm"
)

type Agent struct {
	llmClient llm.Client
	model     string
	maxItems  int
}

func New(client llm.Client, model string, maxItems int) *Agent {
	return &Agent{
		llmClient: client,
		model:     model,
		maxItems:  maxItems,
	}
}

type TriageResponse struct {
	SelectedIndices []int `json:"selected_indices"`
}

func (a *Agent) Run(ctx context.Context, pool []domain.Candidate) ([]domain.Candidate, error) {
	if len(pool) == 0 {
		return nil, nil
	}

	if len(pool) <= a.maxItems {
		slog.Info("Candidate pool is smaller than max items, returning all", "pool_size", len(pool), "max", a.maxItems)
		return pool, nil
	}

	slog.Info("Triaging candidates", "pool_size", len(pool), "target", a.maxItems)

	// Format the pool into a numbered list
	poolJSON, _ := json.MarshalIndent(pool, "", "  ")

	systemPrompt := fmt.Sprintf(`You are a highly selective editor for a weekly technical digest.
Your job is to select the absolute best %d articles from the provided candidate pool.
You must ensure that at least %d items are selected from 'primary' interests (if enough exist).

You will be given a JSON array of candidate articles. Each article has an implicit index (0 to N-1).
Evaluate the candidates based on relevance, technical depth, and diversity of topics.

Return your answer strictly according to the schema: an object with a 'selected_indices' array containing exactly %d integers.
`, a.maxItems, 0, a.maxItems)

	req := llm.StructuredRequest{
		Model:  a.model,
		System: systemPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: string(poolJSON)},
		},
		MaxTokens: 2000,
		Schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"selected_indices": map[string]interface{}{
					"type":        "array",
					"description": "Array of integers representing the index of the selected candidates in the original pool.",
					"items": map[string]interface{}{
						"type": "integer",
					},
				},
			},
			"required":             []string{"selected_indices"},
			"additionalProperties": false,
		},
	}

	var resp TriageResponse
	err := a.llmClient.Structured(ctx, req, &resp)
	if err != nil {
		return nil, fmt.Errorf("structured LLM call failed: %w", err)
	}

	var final []domain.Candidate
	for _, idx := range resp.SelectedIndices {
		if idx >= 0 && idx < len(pool) {
			final = append(final, pool[idx])
		}
	}

	slog.Info("Triage complete", "selected", len(final))
	return final, nil
}
